package fba

import (
	"cosmossdk.io/math"
	"github.com/InjectiveLabs/metrics/v2"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/keeper/spot"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/types"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/types/v2"
)

type SpotBatchAuctionExecutor struct {
	BatchAuctionExecutor // Embedded - provides Match() method

	keeper spot.SpotKeeper
	market *v2.SpotMarket

	// Concrete orderbooks - owned by this executor
	buyOrderbook  *spot.SpotLimitOrderbook
	sellOrderbook *spot.SpotLimitOrderbook
}

func NewSpotBatchAuctionExecutor(ctx sdk.Context, keeper spot.SpotKeeper, market *v2.SpotMarket) *SpotBatchAuctionExecutor {
	return &SpotBatchAuctionExecutor{
		BatchAuctionExecutor: *NewBatchAuctionExecutor(
			ctx.Logger().With("executor", "SpotBatchAuctionExecutor", "marketID", market.MarketID().Hex()),
			keeper.Meter(ctx).SubMeter("SpotBatchAuctionExecutor", metrics.Tag("market_id", market.MarketID().Hex())),
		),
		keeper: keeper,
		market: market,
	}
}

func (e *SpotBatchAuctionExecutor) Execute(ctx sdk.Context, stakingInfo *v2.FeeDiscountStakingInfo) *v2.SpotBatchExecutionData {
	defer e.meter.FuncTiming(&ctx, "SpotBatchAuctionExecutor.Execute")()

	e.buildOrderbooks(ctx)
	defer e.closeOrderbooks()

	// Skip matching if either orderbook is nil (no orders on that side)
	if e.buyOrderbook == nil || e.sellOrderbook == nil {
		return e.processMatchResult(ctx, EmptyMatchResult(), stakingInfo)
	}

	clearingStrategy := e.newClearingPriceStrategy(ctx)
	matchResult := e.Match(ctx, e.buyOrderbook, e.sellOrderbook, clearingStrategy)

	return e.processMatchResult(ctx, matchResult, stakingInfo)
}

func (e *SpotBatchAuctionExecutor) buildOrderbooks(ctx sdk.Context) {
	defer e.meter.FuncTiming(&ctx, "SpotBatchAuctionExecutor.buildOrderbooks")()

	marketID := e.market.MarketID()
	buyIterator := e.keeper.SpotLimitOrderbookIterator(ctx, marketID, true)
	buyTransientOrders := e.keeper.GetAllTransientSpotLimitOrdersByMarketDirection(ctx, marketID, true)
	buyTransientOrders = filterPausedSpotLimitOrders(ctx, e.keeper, buyTransientOrders)
	e.buyOrderbook = spot.NewSpotLimitOrderbook(e.keeper, buyIterator, buyTransientOrders, true)

	sellIterator := e.keeper.SpotLimitOrderbookIterator(ctx, marketID, false)
	sellTransientOrders := e.keeper.GetAllTransientSpotLimitOrdersByMarketDirection(ctx, marketID, false)
	sellTransientOrders = filterPausedSpotLimitOrders(ctx, e.keeper, sellTransientOrders)
	e.sellOrderbook = spot.NewSpotLimitOrderbook(e.keeper, sellIterator, sellTransientOrders, false)
}

// closeOrderbooks ensures orderbooks are properly closed.
func (e *SpotBatchAuctionExecutor) closeOrderbooks() {
	if e.buyOrderbook != nil {
		e.buyOrderbook.Close()
	}
	if e.sellOrderbook != nil {
		e.sellOrderbook.Close()
	}
}

func (e *SpotBatchAuctionExecutor) newClearingPriceStrategy(ctx sdk.Context) ClearingPriceStrategy {
	defer e.meter.FuncTiming(&ctx, "SpotBatchAuctionExecutor.newClearingPriceStrategy")()

	midMarketPrice := e.keeper.GetSpotMidPriceOrBestPrice(ctx, e.market.MarketID())
	return NewSpotClearingPriceStrategy(midMarketPrice)
}

func (e *SpotBatchAuctionExecutor) processMatchResult(ctx sdk.Context, matchResult *MatchResult, stakingInfo *v2.FeeDiscountStakingInfo) *v2.SpotBatchExecutionData {
	defer e.meter.FuncTiming(&ctx, "SpotBatchAuctionExecutor.processMatchResult")()

	var transientBuyFills, restingBuyFills, transientSellFills, restingSellFills *v2.OrderbookFills

	if e.buyOrderbook != nil {
		transientBuyFills = e.buyOrderbook.GetTransientOrderbookFills()
		restingBuyFills = e.buyOrderbook.GetRestingOrderbookFills()
	}
	if e.sellOrderbook != nil {
		transientSellFills = e.sellOrderbook.GetTransientOrderbookFills()
		restingSellFills = e.sellOrderbook.GetRestingOrderbookFills()
	}

	// Convert to SpotOrderbookMatchingResults format
	orderbookResults := &v2.SpotOrderbookMatchingResults{
		ClearingPrice:               matchResult.ClearingPrice,
		ClearingQuantity:            matchResult.ClearingQuantity,
		TransientBuyOrderbookFills:  transientBuyFills,
		RestingBuyOrderbookFills:    restingBuyFills,
		TransientSellOrderbookFills: transientSellFills,
		RestingSellOrderbookFills:   restingSellFills,
	}

	// Get trading rewards and fee discount config
	marketID := e.market.MarketID()
	pointsMultiplier := e.keeper.GetEffectiveTradingRewardsMarketPointsMultiplierConfig(ctx, marketID)
	feeDiscountConfig := e.keeper.GetFeeDiscountConfigForMarket(ctx, marketID, stakingInfo)

	// Initialize deposit deltas
	baseDenomDepositDeltas := types.NewDepositDeltas()
	quoteDenomDepositDeltas := types.NewDepositDeltas()

	// Process resting limit order fills (inlined from ProcessBothRestingSpotLimitOrderbookMatchingResults)
	restingEvents, filledDeltas, restingTradingRewards := e.processRestingOrderbookFills(
		ctx, orderbookResults, pointsMultiplier, feeDiscountConfig, baseDenomDepositDeltas, quoteDenomDepositDeltas, matchResult.ClearingPrice,
	)
	transientEvents, newRestingBuySpotLimitOrders, newRestingSellSpotLimitOrders, transientTradingRewards := e.processTransientOrderbookFills(
		ctx, orderbookResults, pointsMultiplier, feeDiscountConfig, baseDenomDepositDeltas, quoteDenomDepositDeltas, matchResult.ClearingPrice,
	)

	// Collect execution events
	eventBatchSpotExecution := make([]*v2.EventBatchSpotExecution, 0, len(restingEvents)+len(transientEvents))
	eventBatchSpotExecution = append(eventBatchSpotExecution, restingEvents...)
	eventBatchSpotExecution = append(eventBatchSpotExecution, transientEvents...)

	// Calculate VWAP data
	vwapData := v2.NewSpotVwapData()
	vwapData = vwapData.ApplyExecution(orderbookResults.ClearingPrice, orderbookResults.ClearingQuantity)

	// Merge trading rewards
	tradingRewards := types.MergeTradingRewardPoints(restingTradingRewards, transientTradingRewards)

	// Build batch execution data
	batch := &v2.SpotBatchExecutionData{
		Market:                         e.market,
		BaseDenomDepositDeltas:         baseDenomDepositDeltas,
		QuoteDenomDepositDeltas:        quoteDenomDepositDeltas,
		BaseDenomDepositSubaccountIDs:  baseDenomDepositDeltas.GetSortedSubaccountKeys(),
		QuoteDenomDepositSubaccountIDs: quoteDenomDepositDeltas.GetSortedSubaccountKeys(),
		LimitOrderFilledDeltas:         filledDeltas,
		LimitOrderExecutionEvent:       eventBatchSpotExecution,
		TradingRewardPoints:            tradingRewards,
		VwapData:                       vwapData,
	}

	if len(newRestingBuySpotLimitOrders) > 0 || len(newRestingSellSpotLimitOrders) > 0 {
		batch.NewOrdersEvent = &v2.EventNewSpotOrders{
			MarketId:   e.market.MarketId,
			BuyOrders:  newRestingBuySpotLimitOrders,
			SellOrders: newRestingSellSpotLimitOrders,
		}
	}

	return batch
}

// processRestingOrderbookFills processes resting limit order fills.
func (e *SpotBatchAuctionExecutor) processRestingOrderbookFills(
	ctx sdk.Context,
	o *v2.SpotOrderbookMatchingResults,
	pointsMultiplier v2.PointsMultiplier,
	feeDiscountConfig *v2.FeeDiscountConfig,
	baseDenomDepositDeltas, quoteDenomDepositDeltas types.DepositDeltas,
	clearingPrice math.LegacyDec,
) (
	events []*v2.EventBatchSpotExecution,
	filledDeltas []*v2.SpotLimitOrderDelta,
	tradingRewardPoints types.TradingRewardPoints,
) {
	defer e.meter.FuncTiming(&ctx, "SpotBatchAuctionExecutor.processRestingOrderbookFills")()

	var spotLimitBuyOrderStateExpansions, spotLimitSellOrderStateExpansions []*v2.SpotOrderStateExpansion
	var buyTradingRewards, sellTradingRewards types.TradingRewardPoints

	events = make([]*v2.EventBatchSpotExecution, 0, 2)
	filledDeltas = make([]*v2.SpotLimitOrderDelta, 0)

	if o.RestingBuyOrderbookFills != nil {
		orderbookFills := o.GetOrderbookFills(v2.RestingLimitBuy)
		spotLimitBuyOrderStateExpansions = e.keeper.ProcessRestingSpotLimitOrderExpansions(
			ctx,
			e.market.MarketID(),
			orderbookFills,
			true,
			clearingPrice,
			e.market.MakerFeeRate,
			e.market.RelayerFeeShareRate,
			pointsMultiplier,
			feeDiscountConfig,
		)

		var buyEvent *v2.EventBatchSpotExecution
		var currFilledDeltas []*v2.SpotLimitOrderDelta
		buyEvent, currFilledDeltas, buyTradingRewards = v2.GetBatchExecutionEventsFromSpotLimitOrderStateExpansions(
			true,
			e.market,
			v2.ExecutionType_LimitMatchRestingOrder,
			spotLimitBuyOrderStateExpansions,
			baseDenomDepositDeltas, quoteDenomDepositDeltas,
		)
		if buyEvent != nil {
			events = append(events, buyEvent)
		}
		filledDeltas = append(filledDeltas, currFilledDeltas...)
	}

	if o.RestingSellOrderbookFills != nil {
		orderbookFills := o.GetOrderbookFills(v2.RestingLimitSell)
		spotLimitSellOrderStateExpansions = e.keeper.ProcessRestingSpotLimitOrderExpansions(
			ctx,
			e.market.MarketID(),
			orderbookFills,
			false,
			clearingPrice,
			e.market.MakerFeeRate,
			e.market.RelayerFeeShareRate,
			pointsMultiplier,
			feeDiscountConfig,
		)

		var sellEvent *v2.EventBatchSpotExecution
		var currFilledDeltas []*v2.SpotLimitOrderDelta
		sellEvent, currFilledDeltas, sellTradingRewards = v2.GetBatchExecutionEventsFromSpotLimitOrderStateExpansions(
			false,
			e.market,
			v2.ExecutionType_LimitMatchRestingOrder,
			spotLimitSellOrderStateExpansions,
			baseDenomDepositDeltas, quoteDenomDepositDeltas,
		)
		if sellEvent != nil {
			events = append(events, sellEvent)
		}
		filledDeltas = append(filledDeltas, currFilledDeltas...)
	}

	tradingRewardPoints = types.MergeTradingRewardPoints(buyTradingRewards, sellTradingRewards)
	return events, filledDeltas, tradingRewardPoints
}

//nolint:revive // function-result-limit: returns related values that belong together
func (e *SpotBatchAuctionExecutor) processTransientOrderbookFills(
	ctx sdk.Context,
	o *v2.SpotOrderbookMatchingResults,
	pointsMultiplier v2.PointsMultiplier,
	feeDiscountConfig *v2.FeeDiscountConfig,
	baseDenomDepositDeltas, quoteDenomDepositDeltas types.DepositDeltas,
	clearingPrice math.LegacyDec,
) (
	events []*v2.EventBatchSpotExecution,
	newRestingBuySpotLimitOrders []*v2.SpotLimitOrder,
	newRestingSellSpotLimitOrders []*v2.SpotLimitOrder,
	tradingRewardPoints types.TradingRewardPoints,
) {
	defer e.meter.FuncTiming(&ctx, "SpotBatchAuctionExecutor.processTransientOrderbookFills")()

	var expansions []*v2.SpotOrderStateExpansion
	var buyTradingRewards, sellTradingRewards types.TradingRewardPoints

	events = make([]*v2.EventBatchSpotExecution, 0, 2)

	if o.TransientBuyOrderbookFills != nil {
		expansions, newRestingBuySpotLimitOrders = e.keeper.ProcessTransientSpotLimitBuyOrderbookMatchingResults(
			ctx,
			e.market.MarketID(),
			o,
			clearingPrice,
			e.market.MakerFeeRate,
			e.market.TakerFeeRate,
			e.market.RelayerFeeShareRate,
			pointsMultiplier,
			feeDiscountConfig,
		)

		var buyEvent *v2.EventBatchSpotExecution
		buyEvent, _, buyTradingRewards = v2.GetBatchExecutionEventsFromSpotLimitOrderStateExpansions(
			true,
			e.market,
			v2.ExecutionType_LimitMatchNewOrder,
			expansions,
			baseDenomDepositDeltas, quoteDenomDepositDeltas,
		)
		if buyEvent != nil {
			events = append(events, buyEvent)
		}
	}

	if o.TransientSellOrderbookFills != nil {
		expansions, newRestingSellSpotLimitOrders = e.keeper.ProcessTransientSpotLimitSellOrderbookMatchingResults(
			ctx,
			e.market.MarketID(),
			o,
			clearingPrice,
			e.market.TakerFeeRate,
			e.market.RelayerFeeShareRate,
			pointsMultiplier,
			feeDiscountConfig,
		)

		var sellEvent *v2.EventBatchSpotExecution
		sellEvent, _, sellTradingRewards = v2.GetBatchExecutionEventsFromSpotLimitOrderStateExpansions(
			false,
			e.market,
			v2.ExecutionType_LimitMatchNewOrder,
			expansions,
			baseDenomDepositDeltas, quoteDenomDepositDeltas,
		)
		if sellEvent != nil {
			events = append(events, sellEvent)
		}
	}

	tradingRewardPoints = types.MergeTradingRewardPoints(buyTradingRewards, sellTradingRewards)
	return events, newRestingBuySpotLimitOrders, newRestingSellSpotLimitOrders, tradingRewardPoints
}

// filterPausedSpotLimitOrders excludes transient spot limit orders belonging to cross-margin
// subaccounts under emergency pause. No store writes — safe for parallel FBA execution.
// Filtered orders remain in the transient store and are promoted to resting at end-of-block;
// subsequent blocks skip them via the resting-order pause filter in getRestingOrder.
func filterPausedSpotLimitOrders(ctx sdk.Context, k spot.SpotKeeper, orders []*v2.SpotLimitOrder) []*v2.SpotLimitOrder {
	n := 0
	for _, order := range orders {
		if err := k.RiskEngine().CheckCrossMarginEmergencyPause(ctx, order.SubaccountID()); err != nil {
			continue
		}
		orders[n] = order
		n++
	}
	return orders[:n]
}

// filterPausedSpotMarketOrders excludes transient spot market orders belonging to cross-margin
// subaccounts under emergency pause. No store writes — safe for parallel FBA execution.
// Actual cancellation/refund happens in the pre-FBA cleanup step (CancelPausedTransientSpotOrders).
func filterPausedSpotMarketOrders(ctx sdk.Context, k spot.SpotKeeper, orders []*v2.SpotMarketOrder) []*v2.SpotMarketOrder {
	n := 0
	for _, order := range orders {
		if err := k.RiskEngine().CheckCrossMarginEmergencyPause(ctx, order.SubaccountID()); err != nil {
			continue
		}
		orders[n] = order
		n++
	}
	return orders[:n]
}
