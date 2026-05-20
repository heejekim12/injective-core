package spot

import (
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"

	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/keeper/events"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/types"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/types/v2"
)

// MatchingOrderbook is the minimal interface for orderbooks that can participate in spot matching.
// Both SpotMarketOrderbook and SpotLimitOrderbook implement it.
type MatchingOrderbook interface {
	Peek(sdk.Context) *v2.PriceLevel
	Fill(sdk.Context, math.LegacyDec)
}

// MatchSpotOrderbooks performs the matching loop between buy and sell orderbooks.
// It iterates until either side is exhausted or prices no longer cross (spread becomes positive).
// Callers pass the buy-side and sell-side orderbooks (each may be market or limit).
func MatchSpotOrderbooks(
	ctx sdk.Context,
	buyOrderbook MatchingOrderbook,
	sellOrderbook MatchingOrderbook,
) {
	for {
		buyOrder := buyOrderbook.Peek(ctx)
		sellOrder := sellOrderbook.Peek(ctx)

		if buyOrder == nil || sellOrder == nil {
			break
		}

		unitSpread := sellOrder.Price.Sub(buyOrder.Price)
		matchQuantityIncrement := math.LegacyMinDec(buyOrder.Quantity, sellOrder.Quantity)

		if unitSpread.IsPositive() || matchQuantityIncrement.IsZero() {
			break
		}

		buyOrderbook.Fill(ctx, matchQuantityIncrement)
		sellOrderbook.Fill(ctx, matchQuantityIncrement)
	}
}

func (k SpotKeeper) ExecuteAtomicSpotMarketOrder(
	ctx sdk.Context,
	market *v2.SpotMarket,
	marketOrder *v2.SpotMarketOrder,
	feeRate math.LegacyDec,
) *v2.SpotMarketOrderResults {
	defer k.Meter(ctx).FuncTiming(&ctx, "ExecuteAtomicSpotMarketOrder")()

	marketID := market.MarketID()

	stakingInfo, feeDiscountConfig := k.feeDiscounts.GetFeeDiscountConfigAndStakingInfoForMarket(ctx, marketID)
	tradingRewards := types.NewTradingRewardPoints()
	spotVwapInfo := &v2.SpotVwapInfo{}
	tradeRewardsMultiplierConfig := k.GetEffectiveTradingRewardsMarketPointsMultiplierConfig(ctx, market.MarketID())

	isMarketBuy := marketOrder.IsBuy()

	spotLimitOrderStateExpansions, spotMarketOrderStateExpansions, clearingPrice, clearingQuantity :=
		k.getMarketOrderStateExpansionsAndClearingPrice(
			ctx, market, isMarketBuy, []*v2.SpotMarketOrder{marketOrder}, tradeRewardsMultiplierConfig, feeDiscountConfig, feeRate,
		)
	batchExecutionData := GetSpotMarketOrderBatchExecutionData(
		isMarketBuy, market, spotLimitOrderStateExpansions, spotMarketOrderStateExpansions, clearingPrice, clearingQuantity,
	)

	modifiedPositionCache := v2.NewModifiedPositionCache()

	tradingRewards = k.PersistSingleSpotMarketOrderExecution(ctx, marketID, batchExecutionData, *spotVwapInfo, tradingRewards)

	sortedSubaccountIDs := modifiedPositionCache.GetSortedSubaccountIDsByMarket(marketID)
	k.AppendModifiedSubaccountsByMarket(ctx, marketID, sortedSubaccountIDs)

	k.tradingRewards.PersistTradingRewardPoints(ctx, tradingRewards)
	k.feeDiscounts.PersistFeeDiscountStakingInfoUpdates(ctx, stakingInfo)
	k.tradingRewards.PersistVwapInfo(ctx, spotVwapInfo, nil)

	// a trade will always occur since there must exist at least one spot limit order that will cross
	marketOrderTrade := batchExecutionData.MarketOrderExecutionEvent.Trades[0]

	return &v2.SpotMarketOrderResults{
		Quantity: marketOrderTrade.Quantity,
		Price:    marketOrderTrade.Price,
		Fee:      marketOrderTrade.Fee,
	}
}

func GetSpotMarketOrderBatchExecutionData(
	isMarketBuy bool,
	market *v2.SpotMarket,
	spotLimitOrderStateExpansions, spotMarketOrderStateExpansions []*v2.SpotOrderStateExpansion,
	clearingPrice, clearingQuantity math.LegacyDec,
) *v2.SpotBatchExecutionData {
	baseDenomDepositDeltas := types.NewDepositDeltas()
	quoteDenomDepositDeltas := types.NewDepositDeltas()

	// Step 3a: Process market order events
	marketOrderBatchEvent := &v2.EventBatchSpotExecution{
		MarketId:      market.MarketID().Hex(),
		IsBuy:         isMarketBuy,
		ExecutionType: v2.ExecutionType_Market,
	}

	trades := make([]*v2.TradeLog, len(spotMarketOrderStateExpansions))

	marketOrderTradingRewardPoints := types.NewTradingRewardPoints()

	for idx := range spotMarketOrderStateExpansions {
		expansion := spotMarketOrderStateExpansions[idx]
		expansion.UpdateFromDepositDeltas(market, baseDenomDepositDeltas, quoteDenomDepositDeltas)

		realizedTradeFee := expansion.AuctionFeeReward

		isSelfRelayedTrade := expansion.FeeRecipient == types.SubaccountIDToEthAddress(expansion.SubaccountID)
		if !isSelfRelayedTrade {
			realizedTradeFee = realizedTradeFee.Add(expansion.FeeRecipientReward)
		}

		trades[idx] = &v2.TradeLog{
			Quantity:            expansion.BaseChangeAmount.Abs(),
			Price:               expansion.TradePrice,
			SubaccountId:        expansion.SubaccountID.Bytes(),
			Fee:                 realizedTradeFee,
			OrderHash:           expansion.OrderHash.Bytes(),
			FeeRecipientAddress: expansion.FeeRecipient.Bytes(),
			Cid:                 expansion.Cid,
		}
		marketOrderTradingRewardPoints.AddPointsForAddress(expansion.TraderAddress, expansion.TradingRewardPoints)
	}
	marketOrderBatchEvent.Trades = trades

	if len(trades) == 0 {
		marketOrderBatchEvent = nil
	}

	// Stage 3b: Process limit order events
	limitOrderBatchEvent, filledDeltas, limitOrderTradingRewardPoints := v2.GetBatchExecutionEventsFromSpotLimitOrderStateExpansions(
		!isMarketBuy,
		market,
		v2.ExecutionType_LimitFill,
		spotLimitOrderStateExpansions,
		baseDenomDepositDeltas, quoteDenomDepositDeltas,
	)

	limitOrderExecutionEvent := make([]*v2.EventBatchSpotExecution, 0)
	if limitOrderBatchEvent != nil {
		limitOrderExecutionEvent = append(limitOrderExecutionEvent, limitOrderBatchEvent)
	}

	vwapData := v2.NewSpotVwapData()
	vwapData = vwapData.ApplyExecution(clearingPrice, clearingQuantity)

	tradingRewardPoints := types.MergeTradingRewardPoints(marketOrderTradingRewardPoints, limitOrderTradingRewardPoints)

	// Final Step: Store the SpotBatchExecutionData for future reduction/processing
	batch := &v2.SpotBatchExecutionData{
		Market:                         market,
		BaseDenomDepositDeltas:         baseDenomDepositDeltas,
		QuoteDenomDepositDeltas:        quoteDenomDepositDeltas,
		BaseDenomDepositSubaccountIDs:  baseDenomDepositDeltas.GetSortedSubaccountKeys(),
		QuoteDenomDepositSubaccountIDs: quoteDenomDepositDeltas.GetSortedSubaccountKeys(),
		LimitOrderFilledDeltas:         filledDeltas,
		MarketOrderExecutionEvent:      marketOrderBatchEvent,
		LimitOrderExecutionEvent:       limitOrderExecutionEvent,
		TradingRewardPoints:            tradingRewardPoints,
		VwapData:                       vwapData,
	}
	return batch
}

//nolint:revive // ok
func (k SpotKeeper) getMarketOrderStateExpansionsAndClearingPrice(
	ctx sdk.Context,
	market *v2.SpotMarket,
	isMarketBuy bool,
	marketOrders []*v2.SpotMarketOrder,
	pointsMultiplier v2.PointsMultiplier,
	feeDiscountConfig *v2.FeeDiscountConfig,
	takerFeeRate math.LegacyDec,
) (spotLimitOrderStateExpansions, spotMarketOrderStateExpansions []*v2.SpotOrderStateExpansion, clearingPrice, clearingQuantity math.LegacyDec) {
	defer k.Meter(ctx).FuncTiming(&ctx, "getMarketOrderStateExpansionsAndClearingPrice")()

	isLimitBuy := !isMarketBuy
	limitOrdersIterator := k.SpotLimitOrderbookIterator(ctx, market.MarketID(), isLimitBuy)
	limitOrderbook := NewSpotLimitOrderbook(k, limitOrdersIterator, nil, isLimitBuy)

	if limitOrderbook != nil {
		defer limitOrderbook.Close()
	} else {
		spotMarketOrderStateExpansions = k.ProcessSpotMarketOrderStateExpansions(
			ctx,
			market.MarketID(),
			isMarketBuy,
			marketOrders,
			make([]math.LegacyDec, len(marketOrders)),
			math.LegacyDec{},
			takerFeeRate,
			market.RelayerFeeShareRate,
			pointsMultiplier,
			feeDiscountConfig,
		)

		return
	}

	marketOrderbook := NewSpotMarketOrderbook(marketOrders)

	var buyOrderbook, sellOrderbook MatchingOrderbook
	if isMarketBuy {
		buyOrderbook, sellOrderbook = marketOrderbook, limitOrderbook
	} else {
		buyOrderbook, sellOrderbook = limitOrderbook, marketOrderbook
	}
	MatchSpotOrderbooks(ctx, buyOrderbook, sellOrderbook)

	clearingQuantity = limitOrderbook.GetTotalQuantityFilled()

	if clearingQuantity.IsPositive() {
		// Clearing Price equals limit orderbook side average weighted price
		clearingPrice = limitOrderbook.GetNotional().Quo(clearingQuantity)
	}

	spotLimitOrderStateExpansions = k.ProcessRestingSpotLimitOrderExpansions(
		ctx,
		market.MarketID(),
		limitOrderbook.GetRestingOrderbookFills(),
		!isMarketBuy,
		math.LegacyDec{},
		market.MakerFeeRate,
		market.RelayerFeeShareRate,
		pointsMultiplier,
		feeDiscountConfig,
	)

	spotMarketOrderStateExpansions = k.ProcessSpotMarketOrderStateExpansions(
		ctx,
		market.MarketID(),
		isMarketBuy,
		marketOrders,
		marketOrderbook.GetOrderbookFillQuantities(),
		clearingPrice,
		takerFeeRate,
		market.RelayerFeeShareRate,
		pointsMultiplier,
		feeDiscountConfig,
	)

	return
}

// ProcessSpotMarketOrderStateExpansions processes the spot market order state expansions.
// NOTE: clearingPrice may be Nil
//
//nolint:revive // ok
func (k SpotKeeper) ProcessSpotMarketOrderStateExpansions(
	ctx sdk.Context,
	marketID common.Hash,
	isMarketBuy bool,
	marketOrders []*v2.SpotMarketOrder,
	marketFillQuantities []math.LegacyDec,
	clearingPrice math.LegacyDec,
	tradeFeeRate, relayerFeeShareRate math.LegacyDec,
	pointsMultiplier v2.PointsMultiplier,
	feeDiscountConfig *v2.FeeDiscountConfig,
) []*v2.SpotOrderStateExpansion {
	defer k.Meter(ctx).FuncTiming(&ctx, "ProcessSpotMarketOrderStateExpansions")()

	stateExpansions := make([]*v2.SpotOrderStateExpansion, len(marketOrders))

	for idx := range marketOrders {
		stateExpansions[idx] = k.getSpotMarketOrderStateExpansion(
			ctx,
			marketID,
			marketOrders[idx],
			isMarketBuy,
			marketFillQuantities[idx],
			clearingPrice,
			tradeFeeRate,
			relayerFeeShareRate,
			pointsMultiplier,
			feeDiscountConfig,
		)
	}
	return stateExpansions
}

//nolint:revive // ok
func (k SpotKeeper) getSpotMarketOrderStateExpansion(
	ctx sdk.Context,
	marketID common.Hash,
	order *v2.SpotMarketOrder,
	isMarketBuy bool,
	fillQuantity, clearingPrice math.LegacyDec,
	takerFeeRate, relayerFeeShareRate math.LegacyDec,
	pointsMultiplier v2.PointsMultiplier,
	feeDiscountConfig *v2.FeeDiscountConfig,
) *v2.SpotOrderStateExpansion {
	defer k.Meter(ctx).FuncTiming(&ctx, "getSpotMarketOrderStateExpansion")()

	var baseChangeAmount, quoteChangeAmount math.LegacyDec

	if fillQuantity.IsNil() {
		fillQuantity = math.LegacyZeroDec()
	}
	orderNotional := math.LegacyZeroDec()
	if !clearingPrice.IsNil() {
		orderNotional = fillQuantity.Mul(clearingPrice)
	}

	isMaker := false

	feeData := k.tradingRewards.GetTradeDataAndIncrementVolumeContribution(
		ctx,
		order.SubaccountID(),
		marketID,
		fillQuantity,
		clearingPrice,
		takerFeeRate,
		relayerFeeShareRate,
		pointsMultiplier.TakerPointsMultiplier,
		feeDiscountConfig,
		isMaker,
	)

	baseRefundAmount, quoteRefundAmount, quoteChangeAmount := math.LegacyZeroDec(), math.LegacyZeroDec(), math.LegacyZeroDec()

	if isMarketBuy {
		// market buys are credited with the order fill quantity in base denom
		baseChangeAmount = fillQuantity
		// market buys are debited with (fillQuantity * clearingPrice) * (1 + takerFee) in quote denom
		if !clearingPrice.IsNil() {
			quoteChangeAmount = fillQuantity.Mul(clearingPrice).Add(feeData.TotalTradeFee).Neg()
		}
		quoteRefundAmount = order.BalanceHold.Add(quoteChangeAmount)
	} else {
		// market sells are debited by fillQuantity in base denom
		baseChangeAmount = fillQuantity.Neg()
		// market sells are credited with the (fillQuantity * clearingPrice) * (1 - TakerFee) in quote denom
		if !clearingPrice.IsNil() {
			quoteChangeAmount = orderNotional.Sub(feeData.TotalTradeFee)
		}
		// base denom refund unfilled market order quantity
		if fillQuantity.LT(order.OrderInfo.Quantity) {
			baseRefundAmount = order.OrderInfo.Quantity.Sub(fillQuantity)
		}
	}

	tradePrice := clearingPrice
	if tradePrice.IsNil() {
		tradePrice = math.LegacyZeroDec()
	}

	stateExpansion := v2.SpotOrderStateExpansion{
		BaseChangeAmount:        baseChangeAmount,
		BaseRefundAmount:        baseRefundAmount,
		QuoteChangeAmount:       quoteChangeAmount,
		QuoteRefundAmount:       quoteRefundAmount,
		TradePrice:              tradePrice,
		FeeRecipient:            order.FeeRecipient(),
		FeeRecipientReward:      feeData.FeeRecipientReward,
		AuctionFeeReward:        feeData.AuctionFeeReward,
		TraderFeeReward:         math.LegacyZeroDec(),
		TradingRewardPoints:     feeData.TradingRewardPoints,
		MarketOrder:             order,
		MarketOrderFillQuantity: fillQuantity,
		OrderHash:               common.BytesToHash(order.OrderHash),
		OrderPrice:              order.OrderInfo.Price,
		SubaccountID:            order.SubaccountID(),
		TraderAddress:           order.SdkAccAddress().String(),
		Cid:                     order.Cid(),
	}
	return &stateExpansion
}

//nolint:revive // ok
func (k SpotKeeper) ProcessRestingSpotLimitOrderExpansions(
	ctx sdk.Context,
	marketID common.Hash,
	fills *v2.OrderbookFills,
	isLimitBuy bool,
	clearingPrice math.LegacyDec,
	makerFeeRate, relayerFeeShareRate math.LegacyDec,
	pointsMultiplier v2.PointsMultiplier,
	feeDiscountConfig *v2.FeeDiscountConfig,
) []*v2.SpotOrderStateExpansion {
	defer k.Meter(ctx).FuncTiming(&ctx, "ProcessRestingSpotLimitOrderExpansions")()

	stateExpansions := make([]*v2.SpotOrderStateExpansion, len(fills.Orders))
	for idx, order := range fills.Orders {
		fillQuantity, fillPrice := fills.FillQuantities[idx], order.OrderInfo.Price
		if !clearingPrice.IsNil() {
			fillPrice = clearingPrice
		}

		if isLimitBuy {
			stateExpansions[idx] = k.getRestingSpotLimitBuyStateExpansion(
				ctx,
				marketID,
				order,
				order.Hash(),
				fillQuantity,
				fillPrice,
				makerFeeRate,
				relayerFeeShareRate,
				pointsMultiplier,
				feeDiscountConfig,
			)
		} else {
			stateExpansions[idx] = k.getSpotLimitSellStateExpansion(
				ctx,
				marketID,
				order,
				true,
				fillQuantity,
				fillPrice,
				makerFeeRate,
				relayerFeeShareRate,
				pointsMultiplier,
				feeDiscountConfig,
			)
		}
	}
	return stateExpansions
}

//nolint:revive // ok
func (k SpotKeeper) getRestingSpotLimitBuyStateExpansion(
	ctx sdk.Context,
	marketID common.Hash,
	order *v2.SpotLimitOrder,
	orderHash common.Hash,
	fillQuantity, fillPrice, makerFeeRate, relayerFeeShareRate math.LegacyDec,
	pointsMultiplier v2.PointsMultiplier,
	feeDiscountConfig *v2.FeeDiscountConfig,
) *v2.SpotOrderStateExpansion {
	defer k.Meter(ctx).FuncTiming(&ctx, "getRestingSpotLimitBuyStateExpansion")()

	var baseChangeAmount, quoteChangeAmount math.LegacyDec

	isMaker := true
	feeData := k.tradingRewards.GetTradeDataAndIncrementVolumeContribution(
		ctx,
		order.SubaccountID(),
		marketID,
		fillQuantity,
		fillPrice,
		makerFeeRate,
		relayerFeeShareRate,
		pointsMultiplier.MakerPointsMultiplier,
		feeDiscountConfig,
		isMaker,
	)

	orderNotional := fillQuantity.Mul(fillPrice)

	// limit buys are credited with the order fill quantity in base denom
	baseChangeAmount = fillQuantity
	quoteRefund := math.LegacyZeroDec()

	// limit buys are debited with (fillQuantity * Price) * (1 + makerFee) in quote denom
	if feeData.TotalTradeFee.IsNegative() {
		quoteChangeAmount = orderNotional.Neg().Add(feeData.TraderFee.Abs())
		quoteRefund = feeData.TraderFee.Abs()
	} else {
		quoteChangeAmount = orderNotional.Add(feeData.TotalTradeFee).Neg()
	}

	positiveDiscountedFeeRatePart := math.LegacyMaxDec(math.LegacyZeroDec(), feeData.DiscountedTradeFeeRate)

	if !fillPrice.Equal(order.OrderInfo.Price) {
		// nolint:all
		// priceDelta = price - fill price
		priceDelta := order.OrderInfo.Price.Sub(fillPrice)
		// nolint:all
		// clearingRefund = fillQuantity * priceDelta
		clearingRefund := fillQuantity.Mul(priceDelta)

		// nolint:all
		// matchedFeeRefund = max(discountedMakerFeeRate, 0) * fillQuantity * priceDelta
		matchedFeeRefund := positiveDiscountedFeeRatePart.Mul(fillQuantity.Mul(priceDelta))

		// nolint:all
		// quoteRefund += (1 + max(makerFeeRate, 0)) * fillQuantity * priceDelta
		quoteRefund = quoteRefund.Add(clearingRefund.Add(matchedFeeRefund))
	}

	if feeData.TotalTradeFee.IsPositive() {
		positiveMakerFeeRatePart := math.LegacyMaxDec(makerFeeRate, math.LegacyZeroDec())
		makerFeeRateDelta := positiveMakerFeeRatePart.Sub(feeData.DiscountedTradeFeeRate)
		matchedFeeDiscountRefund := fillQuantity.Mul(order.OrderInfo.Price).Mul(makerFeeRateDelta)
		quoteRefund = quoteRefund.Add(matchedFeeDiscountRefund)
	}

	order.Fillable = order.Fillable.Sub(fillQuantity)

	stateExpansion := v2.SpotOrderStateExpansion{
		BaseChangeAmount:       baseChangeAmount,
		BaseRefundAmount:       math.LegacyZeroDec(),
		QuoteChangeAmount:      quoteChangeAmount,
		QuoteRefundAmount:      quoteRefund,
		TradePrice:             fillPrice,
		FeeRecipient:           order.FeeRecipient(),
		FeeRecipientReward:     feeData.FeeRecipientReward,
		AuctionFeeReward:       feeData.AuctionFeeReward,
		TraderFeeReward:        feeData.TraderFee,
		TradingRewardPoints:    feeData.TradingRewardPoints,
		LimitOrder:             order,
		LimitOrderFillQuantity: fillQuantity,
		OrderPrice:             order.OrderInfo.Price,
		OrderHash:              orderHash,
		SubaccountID:           order.SubaccountID(),
		TraderAddress:          order.SdkAccAddress().String(),
		Cid:                    order.Cid(),
	}
	return &stateExpansion
}

//nolint:revive // ok
func (k SpotKeeper) getSpotLimitSellStateExpansion(
	ctx sdk.Context,
	marketID common.Hash,
	order *v2.SpotLimitOrder,
	isMaker bool,
	fillQuantity, fillPrice, tradeFeeRate, relayerFeeShareRate math.LegacyDec,
	pointsMultiplier v2.PointsMultiplier,
	feeDiscountConfig *v2.FeeDiscountConfig,
) *v2.SpotOrderStateExpansion {
	defer k.Meter(ctx).FuncTiming(&ctx, "getSpotLimitSellStateExpansion")()

	orderNotional := fillQuantity.Mul(fillPrice)

	var tradeRewardMultiplier math.LegacyDec
	if isMaker {
		tradeRewardMultiplier = pointsMultiplier.MakerPointsMultiplier
	} else {
		tradeRewardMultiplier = pointsMultiplier.TakerPointsMultiplier
	}
	feeData := k.tradingRewards.GetTradeDataAndIncrementVolumeContribution(
		ctx,
		order.SubaccountID(),
		marketID,
		fillQuantity,
		fillPrice,
		tradeFeeRate,
		relayerFeeShareRate,
		tradeRewardMultiplier,
		feeDiscountConfig,
		isMaker,
	)

	// limit sells are credited with the (fillQuantity * price) * traderFee in quote denom
	// traderFee can be positive or negative
	quoteChangeAmount := orderNotional.Sub(feeData.TraderFee)
	order.Fillable = order.Fillable.Sub(fillQuantity)

	stateExpansion := v2.SpotOrderStateExpansion{
		// limit sells are debited by fillQuantity in base denom
		BaseChangeAmount:       fillQuantity.Neg(),
		BaseRefundAmount:       math.LegacyZeroDec(),
		QuoteChangeAmount:      quoteChangeAmount,
		QuoteRefundAmount:      math.LegacyZeroDec(),
		TradePrice:             fillPrice,
		FeeRecipient:           order.FeeRecipient(),
		FeeRecipientReward:     feeData.FeeRecipientReward,
		AuctionFeeReward:       feeData.AuctionFeeReward,
		TraderFeeReward:        feeData.TraderFee,
		TradingRewardPoints:    feeData.TradingRewardPoints,
		LimitOrder:             order,
		LimitOrderFillQuantity: fillQuantity,
		OrderPrice:             order.OrderInfo.Price,
		OrderHash:              order.Hash(),
		SubaccountID:           order.SubaccountID(),
		TraderAddress:          order.SdkAccAddress().String(),
		Cid:                    order.Cid(),
	}
	return &stateExpansion
}

//nolint:revive // ok
func (k SpotKeeper) PersistSingleSpotMarketOrderExecution(
	ctx sdk.Context,
	marketID common.Hash,
	execution *v2.SpotBatchExecutionData,
	spotVwapData v2.SpotVwapInfo,
	tradingRewardPoints types.TradingRewardPoints,
) types.TradingRewardPoints {
	defer k.Meter(ctx).FuncTiming(&ctx, "PersistSingleSpotMarketOrderExecution")()

	if execution == nil {
		return tradingRewardPoints
	}

	if execution.VwapData != nil && !execution.VwapData.Price.IsZero() && !execution.VwapData.Quantity.IsZero() {
		spotVwapData.ApplyVwap(marketID, execution.VwapData)
	}
	baseDenom, quoteDenom := execution.Market.BaseDenom, execution.Market.QuoteDenom

	for _, subaccountID := range execution.BaseDenomDepositSubaccountIDs {
		k.subaccount.UpdateDepositWithDelta(
			ctx,
			subaccountID,
			baseDenom,
			execution.BaseDenomDepositDeltas[subaccountID],
		)
		// Evict stale cross-pool snapshot: base-denom balance change can affect cross equity.
		k.RiskEngine().EvictCrossPoolSnapshotCache(ctx, subaccountID)
	}
	for _, subaccountID := range execution.QuoteDenomDepositSubaccountIDs {
		k.subaccount.UpdateDepositWithDelta(
			ctx,
			subaccountID,
			quoteDenom,
			execution.QuoteDenomDepositDeltas[subaccountID],
		)
		// Evict stale cross-pool snapshot: quote-denom balance feeds into cross equity.
		k.RiskEngine().EvictCrossPoolSnapshotCache(ctx, subaccountID)
	}

	for _, limitOrderDelta := range execution.LimitOrderFilledDeltas {
		k.UpdateSpotLimitOrder(ctx, marketID, limitOrderDelta)
	}

	// only get first index since only one limit order side that gets filled
	if execution.MarketOrderExecutionEvent != nil {
		events.Emit(ctx, k.BaseKeeper, execution.MarketOrderExecutionEvent)
	}

	if len(execution.LimitOrderExecutionEvent) > 0 {
		events.Emit(ctx, k.BaseKeeper, execution.LimitOrderExecutionEvent[0])
	}

	if len(execution.TradingRewardPoints) > 0 {
		tradingRewardPoints = types.MergeTradingRewardPoints(tradingRewardPoints, execution.TradingRewardPoints)
	}

	return tradingRewardPoints
}

func (k SpotKeeper) PersistSpotMarketOrderExecution(
	ctx sdk.Context,
	batchSpotExecutionData []*v2.SpotBatchExecutionData,
	spotVwapData v2.SpotVwapInfo,
) types.TradingRewardPoints {
	defer k.Meter(ctx).FuncTiming(&ctx, "PersistSpotMarketOrderExecution")()

	tradingRewardPoints := types.NewTradingRewardPoints()
	for batchIdx := range batchSpotExecutionData {
		execution := batchSpotExecutionData[batchIdx]
		if execution == nil {
			continue
		}
		marketID := execution.Market.MarketID()

		tradingRewardPoints = k.PersistSingleSpotMarketOrderExecution(ctx, marketID, execution, spotVwapData, tradingRewardPoints)
	}
	return tradingRewardPoints
}

// TODO: refactor to merge ProcessTransientSpotLimitBuyOrderbookMatchingResults and ProcessTransientSpotLimitSellOrderbookMatchingResults
//
//nolint:revive // ok
func (k SpotKeeper) ProcessTransientSpotLimitBuyOrderbookMatchingResults(
	ctx sdk.Context,
	marketID common.Hash,
	o *v2.SpotOrderbookMatchingResults,
	clearingPrice math.LegacyDec,
	makerFeeRate, takerFeeRate, relayerFeeShare math.LegacyDec,
	pointsMultiplier v2.PointsMultiplier,
	feeDiscountConfig *v2.FeeDiscountConfig,
) ([]*v2.SpotOrderStateExpansion, []*v2.SpotLimitOrder) {
	defer k.Meter(ctx).FuncTiming(&ctx, "ProcessTransientSpotLimitBuyOrderbookMatchingResults")()

	orderbookFills := o.TransientBuyOrderbookFills
	stateExpansions := make([]*v2.SpotOrderStateExpansion, len(orderbookFills.Orders))
	newRestingOrders := make([]*v2.SpotLimitOrder, 0, len(orderbookFills.Orders))

	for idx, order := range orderbookFills.Orders {
		fillQuantity := math.LegacyZeroDec()
		if orderbookFills.FillQuantities != nil {
			fillQuantity = orderbookFills.FillQuantities[idx]
		}
		stateExpansions[idx] = k.getTransientSpotLimitBuyStateExpansion(
			ctx,
			marketID,
			order,
			common.BytesToHash(order.OrderHash),
			clearingPrice, fillQuantity,
			makerFeeRate, takerFeeRate, relayerFeeShare,
			pointsMultiplier,
			feeDiscountConfig,
		)

		if order.Fillable.IsPositive() {
			newRestingOrders = append(newRestingOrders, order)
		}
	}
	return stateExpansions, newRestingOrders
}

func (k SpotKeeper) getTransientSpotLimitBuyStateExpansion( //nolint:revive // ok
	ctx sdk.Context,
	marketID common.Hash,
	order *v2.SpotLimitOrder,
	orderHash common.Hash,
	clearingPrice, fillQuantity,
	makerFeeRate, takerFeeRate, relayerFeeShareRate math.LegacyDec,
	pointsMultiplier v2.PointsMultiplier,
	feeDiscountConfig *v2.FeeDiscountConfig,
) *v2.SpotOrderStateExpansion {
	defer k.Meter(ctx).FuncTiming(&ctx, "getTransientSpotLimitBuyStateExpansion")()

	orderNotional, clearingChargeOrRefund, matchedFeeRefund := math.LegacyZeroDec(), math.LegacyZeroDec(), math.LegacyZeroDec()

	isMaker := false
	feeData := k.tradingRewards.GetTradeDataAndIncrementVolumeContribution(
		ctx,
		order.SubaccountID(),
		marketID,
		fillQuantity,
		clearingPrice,
		takerFeeRate,
		relayerFeeShareRate,
		pointsMultiplier.TakerPointsMultiplier,
		feeDiscountConfig,
		isMaker,
	)

	if !fillQuantity.IsZero() {
		orderNotional = fillQuantity.Mul(clearingPrice)
		priceDelta := order.OrderInfo.Price.Sub(clearingPrice)
		// Clearing Refund = FillQuantity * (Price - ClearingPrice)
		clearingChargeOrRefund = fillQuantity.Mul(priceDelta)
		// Matched Fee Refund = FillQuantity * TakerFeeRate * (Price - ClearingPrice)
		matchedFeeRefund = fillQuantity.Mul(feeData.DiscountedTradeFeeRate).Mul(priceDelta)
	}

	// limit buys are credited with the order fill quantity in base denom
	baseChangeAmount := fillQuantity
	// limit buys are debited with (fillQuantity * Price) * (1 + makerFee) in quote denom
	quoteChangeAmount := orderNotional.Add(feeData.TotalTradeFee).Neg()
	// Unmatched Fee Refund = (Quantity - FillQuantity) * Price * (TakerFeeRate - MakerFeeRate)
	positiveMakerFeePart := math.LegacyMaxDec(math.LegacyZeroDec(), makerFeeRate)

	unfilledQuantity := order.OrderInfo.Quantity.Sub(fillQuantity)
	unmatchedFeeRefund := unfilledQuantity.Mul(order.OrderInfo.Price).Mul(takerFeeRate.Sub(positiveMakerFeePart))
	// Fee Refund = Matched Fee Refund + Unmatched Fee Refund
	feeRefund := matchedFeeRefund.Add(unmatchedFeeRefund)
	// refund amount = clearing charge or refund + matched fee refund + unmatched fee refund
	quoteRefundAmount := clearingChargeOrRefund.Add(feeRefund)
	order.Fillable = order.Fillable.Sub(fillQuantity)

	takerFeeRateDelta := takerFeeRate.Sub(feeData.DiscountedTradeFeeRate)
	matchedFeeDiscountRefund := fillQuantity.Mul(order.OrderInfo.Price).Mul(takerFeeRateDelta)
	quoteRefundAmount = quoteRefundAmount.Add(matchedFeeDiscountRefund)

	stateExpansion := v2.SpotOrderStateExpansion{
		BaseChangeAmount:       baseChangeAmount,
		BaseRefundAmount:       math.LegacyZeroDec(),
		QuoteChangeAmount:      quoteChangeAmount,
		QuoteRefundAmount:      quoteRefundAmount,
		TradePrice:             clearingPrice,
		FeeRecipient:           order.FeeRecipient(),
		FeeRecipientReward:     feeData.FeeRecipientReward,
		AuctionFeeReward:       feeData.AuctionFeeReward,
		TraderFeeReward:        math.LegacyZeroDec(),
		TradingRewardPoints:    feeData.TradingRewardPoints,
		LimitOrder:             order,
		LimitOrderFillQuantity: fillQuantity,
		OrderPrice:             order.OrderInfo.Price,
		OrderHash:              orderHash,
		SubaccountID:           order.SubaccountID(),
		TraderAddress:          order.SdkAccAddress().String(),
		Cid:                    order.Cid(),
	}
	return &stateExpansion
}

// ProcessTransientSpotLimitSellOrderbookMatchingResults processes.
// Note: clearingPrice should be set to math.LegacyDec{} for normal fills
func (k SpotKeeper) ProcessTransientSpotLimitSellOrderbookMatchingResults(
	ctx sdk.Context,
	marketID common.Hash,
	o *v2.SpotOrderbookMatchingResults,
	clearingPrice math.LegacyDec,
	takerFeeRate, relayerFeeShare math.LegacyDec,
	pointsMultiplier v2.PointsMultiplier,
	feeDiscountConfig *v2.FeeDiscountConfig,
) ([]*v2.SpotOrderStateExpansion, []*v2.SpotLimitOrder) {
	defer k.Meter(ctx).FuncTiming(&ctx, "ProcessTransientSpotLimitSellOrderbookMatchingResults")()

	orderbookFills := o.TransientSellOrderbookFills

	stateExpansions := make([]*v2.SpotOrderStateExpansion, len(orderbookFills.Orders))
	newRestingOrders := make([]*v2.SpotLimitOrder, 0, len(orderbookFills.Orders))

	for idx, order := range orderbookFills.Orders {
		fillQuantity, fillPrice := orderbookFills.FillQuantities[idx], order.OrderInfo.Price
		if !clearingPrice.IsNil() {
			fillPrice = clearingPrice
		}
		stateExpansions[idx] = k.getSpotLimitSellStateExpansion(
			ctx,
			marketID,
			order,
			false,
			fillQuantity,
			fillPrice,
			takerFeeRate,
			relayerFeeShare,
			pointsMultiplier,
			feeDiscountConfig,
		)
		if order.Fillable.IsPositive() {
			newRestingOrders = append(newRestingOrders, order)
		}
	}
	return stateExpansions, newRestingOrders
}

func (k SpotKeeper) PersistSpotMatchingExecution( //nolint:revive // ok
	ctx sdk.Context,
	batchSpotMatchingExecutionData []*v2.SpotBatchExecutionData,
	spotVwapData v2.SpotVwapInfo,
	tradingRewardPoints types.TradingRewardPoints,
) types.TradingRewardPoints {
	defer k.Meter(ctx).FuncTiming(&ctx, "PersistSpotMatchingExecution")()

	// Persist Spot Matching execution data
	for batchIdx := range batchSpotMatchingExecutionData {
		execution := batchSpotMatchingExecutionData[batchIdx]
		if execution == nil {
			continue
		}

		marketID := execution.Market.MarketID()
		baseDenom, quoteDenom := execution.Market.BaseDenom, execution.Market.QuoteDenom

		if execution.VwapData != nil && !execution.VwapData.Price.IsZero() && !execution.VwapData.Quantity.IsZero() {
			spotVwapData.ApplyVwap(marketID, execution.VwapData)
		}

		for _, subaccountID := range execution.BaseDenomDepositSubaccountIDs {
			k.subaccount.UpdateDepositWithDelta(ctx, subaccountID, baseDenom, execution.BaseDenomDepositDeltas[subaccountID])
			k.RiskEngine().EvictCrossPoolSnapshotCache(ctx, subaccountID)
		}

		for _, subaccountID := range execution.QuoteDenomDepositSubaccountIDs {
			k.subaccount.UpdateDepositWithDelta(ctx, subaccountID, quoteDenom, execution.QuoteDenomDepositDeltas[subaccountID])
			k.RiskEngine().EvictCrossPoolSnapshotCache(ctx, subaccountID)
		}

		if execution.NewOrdersEvent != nil {
			for idx := range execution.NewOrdersEvent.BuyOrders {
				k.SaveNewSpotLimitOrder(ctx,
					execution.NewOrdersEvent.BuyOrders[idx],
					marketID, true,
					execution.NewOrdersEvent.BuyOrders[idx].Hash(),
				)
			}

			for idx := range execution.NewOrdersEvent.SellOrders {
				k.SaveNewSpotLimitOrder(ctx,
					execution.NewOrdersEvent.SellOrders[idx],
					marketID, false,
					execution.NewOrdersEvent.SellOrders[idx].Hash(),
				)
			}

			events.Emit(ctx, k.BaseKeeper, execution.NewOrdersEvent)
		}

		for _, limitOrderDelta := range execution.LimitOrderFilledDeltas {
			k.UpdateSpotLimitOrder(ctx, marketID, limitOrderDelta)
		}

		for idx := range execution.LimitOrderExecutionEvent {
			if execution.LimitOrderExecutionEvent[idx] != nil {
				events.Emit(ctx, k.BaseKeeper, execution.LimitOrderExecutionEvent[idx])
			}
		}

		if len(execution.TradingRewardPoints) > 0 {
			tradingRewardPoints = types.MergeTradingRewardPoints(tradingRewardPoints, execution.TradingRewardPoints)
		}
	}
	return tradingRewardPoints
}
