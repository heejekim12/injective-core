package fba

import (
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"

	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/keeper/derivative"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/types/v2"
	"github.com/InjectiveLabs/metrics/v2"
)

type DerivativeBatchAuctionExecutor struct {
	BatchAuctionExecutor

	keeper                derivative.DerivativeKeeper
	market                v2.DerivativeMarketI
	markPrice             math.LegacyDec
	funding               *v2.PerpetualMarketFunding
	positionStates        map[common.Hash]*v2.PositionState
	positionCache         map[common.Hash]*v2.Position
	openNotionalCap       v2.OpenNotionalCap
	currentOpenNotional   math.LegacyDec
	modifiedPositionCache v2.ModifiedPositionCache

	// Reduce-only orders filtered due to position modifications
	ordersToCancel *derivative.FilteredTransientOrderResults

	// Concrete orderbooks - owned by this executor
	buyOrderbook  *derivative.LimitOrderbook
	sellOrderbook *derivative.LimitOrderbook
}

func NewDerivativeBatchAuctionExecutor(
	ctx sdk.Context,
	keeper derivative.DerivativeKeeper,
	market v2.DerivativeMarketI,
	markPrice math.LegacyDec,
	funding *v2.PerpetualMarketFunding,
	openNotionalCap v2.OpenNotionalCap,
	currentOpenNotional math.LegacyDec,
	modifiedPositionCache v2.ModifiedPositionCache,
) *DerivativeBatchAuctionExecutor {
	return &DerivativeBatchAuctionExecutor{
		BatchAuctionExecutor: *NewBatchAuctionExecutor(
			ctx.Logger().With("executor", "DerivativeBatchAuctionExecutor", "marketID", market.MarketID().Hex()),
			keeper.Meter(ctx).SubMeter("DerivativeBatchAuctionExecutor", metrics.Tag("market_id", market.MarketID().Hex())),
		),
		keeper:                keeper,
		market:                market,
		markPrice:             markPrice,
		funding:               funding,
		positionStates:        v2.NewPositionStates(),
		positionCache:         make(map[common.Hash]*v2.Position),
		openNotionalCap:       openNotionalCap,
		currentOpenNotional:   currentOpenNotional,
		modifiedPositionCache: modifiedPositionCache,
	}
}

func (e *DerivativeBatchAuctionExecutor) Execute(ctx sdk.Context, stakingInfo *v2.FeeDiscountStakingInfo) *v2.DerivativeBatchExecutionData {
	defer e.meter.FuncTiming(&ctx, "DerivativeBatchAuctionExecutor.Execute")()

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

func (e *DerivativeBatchAuctionExecutor) buildOrderbooks(ctx sdk.Context) {
	defer e.meter.FuncTiming(&ctx, "DerivativeBatchAuctionExecutor.buildOrderbooks")()

	e.ensureFilteredOrders(ctx)

	e.buyOrderbook = derivative.NewLimitOrderbook(
		e.keeper, ctx, true,
		false, // not a liquidation
		e.ordersToCancel.TransientLimitBuyOrders,
		e.market, e.markPrice, e.funding,
		e.currentOpenNotional, e.openNotionalCap,
		e.positionStates, e.positionCache,
	)

	e.sellOrderbook = derivative.NewLimitOrderbook(
		e.keeper, ctx, false,
		false, // not a liquidation
		e.ordersToCancel.TransientLimitSellOrders,
		e.market, e.markPrice, e.funding,
		e.currentOpenNotional, e.openNotionalCap,
		e.positionStates, e.positionCache,
	)

	// Link opposite sides for open notional cap validation
	if e.buyOrderbook != nil && e.sellOrderbook != nil {
		e.buyOrderbook.SetOppositeSideDerivativeOrderbook(e.sellOrderbook)
		e.sellOrderbook.SetOppositeSideDerivativeOrderbook(e.buyOrderbook)
	}
}

func (e *DerivativeBatchAuctionExecutor) ensureFilteredOrders(ctx sdk.Context) {
	defer e.meter.FuncTiming(&ctx, "DerivativeBatchAuctionExecutor.ensureFilteredOrders")()

	if e.ordersToCancel != nil {
		return
	}
	e.ordersToCancel = e.keeper.GetFilteredTransientOrdersAndOrdersToCancel(ctx, e.market.MarketID(), e.modifiedPositionCache)
}

func (e *DerivativeBatchAuctionExecutor) closeOrderbooks() {
	if e.buyOrderbook != nil {
		e.buyOrderbook.Close()
	}
	if e.sellOrderbook != nil {
		e.sellOrderbook.Close()
	}
}

// newClearingPriceStrategy returns a derivative-specific strategy with mark price.
func (e *DerivativeBatchAuctionExecutor) newClearingPriceStrategy(ctx sdk.Context) ClearingPriceStrategy {
	defer e.meter.FuncTiming(&ctx, "DerivativeBatchAuctionExecutor.newClearingPriceStrategy")()

	midMarketPrice := e.keeper.GetDerivativeMidPriceOrBestPrice(ctx, e.market.MarketID())
	return NewDerivativeClearingPriceStrategy(e.markPrice, midMarketPrice)
}

func (e *DerivativeBatchAuctionExecutor) processMatchResult(ctx sdk.Context, matchResult *MatchResult, stakingInfo *v2.FeeDiscountStakingInfo) *v2.DerivativeBatchExecutionData {
	defer e.meter.FuncTiming(&ctx, "DerivativeBatchAuctionExecutor.processMatchResult")()

	feeDiscountConfig := e.keeper.GetFeeDiscountConfigForMarket(ctx, e.market.MarketID(), stakingInfo)
	isCrossSubaccount := makeIsCrossSubaccountFn(ctx, e.keeper)

	// Process orderbook fills and get expansion data
	expansionData := e.processOrderbookFills(ctx, matchResult.ClearingPrice, matchResult.ClearingQuantity, feeDiscountConfig)

	// Add cancelled reduce-only orders (filtered due to position modifications)
	if e.ordersToCancel != nil {
		expansionData.TransientLimitBuyOrderCancels = append(
			expansionData.TransientLimitBuyOrderCancels,
			e.ordersToCancel.TransientLimitBuyOrdersToCancel...,
		)
		expansionData.TransientLimitSellOrderCancels = append(
			expansionData.TransientLimitSellOrderCancels,
			e.ordersToCancel.TransientLimitSellOrdersToCancel...,
		)
	}

	// Convert expansion data to batch execution data
	return expansionData.GetLimitMatchingDerivativeBatchExecutionData(
		e.market,
		e.markPrice,
		e.funding,
		e.positionStates,
		isCrossSubaccount,
	)
}

func (e *DerivativeBatchAuctionExecutor) processOrderbookFills(
	ctx sdk.Context,
	clearingPrice, clearingQuantity math.LegacyDec,
	feeDiscountConfig *v2.FeeDiscountConfig,
) *v2.DerivativeMatchingExpansionData {
	defer e.meter.FuncTiming(&ctx, "DerivativeBatchAuctionExecutor.processOrderbookFills")()

	tradeRewardsMultiplierConfig := e.keeper.GetEffectiveTradingRewardsMarketPointsMultiplierConfig(ctx, e.market.MarketID())
	expansionData := v2.NewDerivativeMatchingExpansionData(clearingPrice, clearingQuantity)

	if e.buyOrderbook != nil {
		e.processSideOrderbookFills(ctx, expansionData, clearingPrice, tradeRewardsMultiplierConfig, feeDiscountConfig, true, e.buyOrderbook)
	}

	if e.sellOrderbook != nil {
		e.processSideOrderbookFills(ctx, expansionData, clearingPrice, tradeRewardsMultiplierConfig, feeDiscountConfig, false, e.sellOrderbook)
	}

	return expansionData
}

func (e *DerivativeBatchAuctionExecutor) processSideOrderbookFills(
	ctx sdk.Context,
	expansionData *v2.DerivativeMatchingExpansionData,
	clearingPrice math.LegacyDec,
	tradeRewardsMultiplierConfig v2.PointsMultiplier,
	feeDiscountConfig *v2.FeeDiscountConfig,
	isBuy bool, // revive:disable:flag-parameter // this was introduced to remove duplicated code
	orderbook *derivative.LimitOrderbook,
) {
	defer e.meter.FuncTiming(&ctx, "DerivativeBatchAuctionExecutor.processSideOrderbookFills")()
	// Collect partial cancel orders from orderbook
	for hash := range orderbook.GetPartialCancelOrders() {
		expansionData.PartialCancelOrders[hash] = struct{}{}
	}

	mergedOrderbookFills := derivative.NewMergedDerivativeOrderbookFills(
		isBuy,
		orderbook.GetTransientOrderbookFills(),
		orderbook.GetRestingOrderbookFills(),
	)

	for {
		fill := mergedOrderbookFills.Next()
		if fill == nil {
			break
		}

		expansion := e.keeper.ApplyPositionDeltaAndGetDerivativeLimitOrderStateExpansion(
			ctx,
			e.market,
			e.funding,
			isBuy,
			fill.IsTransient,
			fill.Order,
			orderbook.GetPositionStates(),
			fill.FillQuantity,
			clearingPrice,
			tradeRewardsMultiplierConfig,
			feeDiscountConfig,
			false,
		)

		expansionData.AddExpansion(isBuy, fill.IsTransient, expansion)

		// Add partially filled transient order to the soon-to-be new resting orders
		// but NOT if it's marked for partial cancellation
		_, isPartialCancel := expansionData.PartialCancelOrders[fill.Order.Hash()]
		if fill.IsTransient && expansion.LimitOrderFilledDelta.FillableQuantity().IsPositive() && !isPartialCancel {
			if isBuy {
				expansionData.AddNewBuyRestingLimitOrder(fill.Order)
			} else {
				expansionData.AddNewSellRestingLimitOrder(fill.Order)
			}
		}
	}

	// Assign cancels and open interest delta based on side
	if isBuy {
		expansionData.RestingLimitBuyOrderCancels = orderbook.GetRestingOrderbookCancels()
		expansionData.TransientLimitBuyOrderCancels = orderbook.GetTransientOrderbookCancels()
	} else {
		expansionData.RestingLimitSellOrderCancels = orderbook.GetRestingOrderbookCancels()
		expansionData.TransientLimitSellOrderCancels = orderbook.GetTransientOrderbookCancels()
	}

	expansionData.OpenInterestDelta = expansionData.OpenInterestDelta.Add(orderbook.GetOpenInterestDelta())
}

func makeIsCrossSubaccountFn(ctx sdk.Context, keeper derivative.DerivativeKeeper) func(common.Hash) bool {
	return keeper.RiskEngine().MakeIsCrossSubaccountFn(ctx)
}
