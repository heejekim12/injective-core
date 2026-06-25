package exchange

import (
	"runtime/debug"
	"sync"

	"github.com/cosmos/cosmos-sdk/baseapp"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"

	downtimetypes "github.com/InjectiveLabs/injective-core/injective-chain/modules/downtime-detector/types"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/fba"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/keeper"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/types"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/types/v2"
	chaintypes "github.com/InjectiveLabs/injective-core/injective-chain/types"
)

type BlockHandler struct {
	k *keeper.Keeper
}

func NewBlockHandler(k *keeper.Keeper) *BlockHandler {
	return &BlockHandler{
		k: k,
	}
}

func (h *BlockHandler) BeginBlocker(ctx sdk.Context) {
	defer h.k.Meter(ctx).FuncTiming(&ctx, "BlockHandler.BeginBlocker")()

	// swap the gas meter with a threadsafe version

	// Check for downtime-based post-only mode activation (execute first to ensure immediate response to downtime)
	params := h.k.GetParams(ctx)

	h.processDowntimePostOnlyMode(ctx, params)

	// Check for post-only mode cancellation flag and disable post-only mode if set
	h.processPostOnlyModeCancellation(ctx)

	h.k.ProcessExpiredOrders(ctx)
	h.k.ProcessHourlyFundings(ctx)
	h.k.ProcessForceClosedSpotMarkets(ctx)
	h.k.ProcessMarketsScheduledToSettle(ctx) // ensure this runs before ProcessMatureExpiryFutureMarkets
	h.k.ProcessMatureExpiryFutureMarkets(ctx)
	h.k.ProcessBinaryOptionsMarketsToExpireAndSettle(ctx)
	h.k.ProcessTradingRewards(ctx)
	h.k.ProcessFeeDiscountBuckets(ctx)

	if ctx.BlockHeight()%100000 == 0 {
		h.k.CleanupHistoricalTradeRecords(ctx)
	}

	// update cached fixed_gas enabled flag based on params (gov proposal might have changed it)
	if params.FixedGasEnabled != h.k.IsFixedGasEnabled() {
		h.k.SetFixedGasEnabled(params.FixedGasEnabled)
	}

}

func (h *BlockHandler) EndBlocker(ctx sdk.Context) {
	defer h.k.Meter(ctx).FuncTiming(&ctx, "BlockHandler.EndBlocker")()

	// swap the gas meter with a threadsafe version
	ctx = ctx.WithGasMeter(chaintypes.NewThreadsafeInfiniteGasMeter()).
		WithBlockGasMeter(chaintypes.NewThreadsafeInfiniteGasMeter())

	// =========== Pre-matching: Trigger conditional market orders ===========

	// Process Conditional Market orders first
	triggeredMarketsAndOrders, marketCache := h.k.GetAllTriggeredConditionalOrders(ctx)
	h.handleConditionalMarketOrderCancels(ctx, triggeredMarketsAndOrders)
	h.handleTriggeringConditionalMarketOrders(ctx, triggeredMarketsAndOrders)

	// =========== Pre-FBA: Cancel paused transient orders ===========
	// Must happen before FBA stages to avoid store mutations during parallel execution
	// that would stale derivative cross-margin risk snapshots, and to prevent paused
	// transient derivative orders from being promoted to resting during post-match processing.
	h.k.CancelPausedTransientSpotOrders(ctx)
	h.k.CancelPausedTransientDerivativeOrders(ctx)

	// =========== Stage 1: Process market orders in parallel ===========

	stakingInfo := h.k.InitialFetchAndUpdateActiveAccountFeeDiscountStakingInfo(ctx)
	spotVwapData := v2.NewSpotVwapInfo()

	// Create FBA batch auction for this block execution
	batchAuction := fba.NewBatchAuction(*h.k.SpotKeeper, *h.k.DerivativeKeeper, h.k.BuildDerivativeStageRiskPrepass)

	// Get market order indicators
	spotMarketOrderIndicators := h.k.GetAllTransientSpotMarketOrderIndicators(ctx)
	derivativeMarketOrderDirections := h.k.GetAllTransientDerivativeMarketDirections(ctx, false)

	// Build modified position cache in parallel with market order execution
	derivativeLimitOrderMarketDirections := h.k.GetAllTransientDerivativeMarketDirections(ctx, true)
	modifiedPositionCache := v2.NewModifiedPositionCache()

	wg := new(sync.WaitGroup)
	if len(derivativeLimitOrderMarketDirections) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for _, m := range derivativeLimitOrderMarketDirections {
				subaccountIDs := h.k.GetModifiedSubaccountsByMarket(ctx, m.MarketId)
				if subaccountIDs == nil || len(subaccountIDs.SubaccountIds) == 0 {
					continue
				}

				for _, subaccountID := range subaccountIDs.SubaccountIds {
					modifiedPositionCache.SetPositionIndicator(m.MarketId, common.BytesToHash(subaccountID))
				}
			}
		}()
	}

	// Execute market orders via FBA BatchAuction (handles parallel execution internally)
	batchSpotExecutionData, batchDerivativeExecutionData := batchAuction.ExecuteMarketOrders(
		ctx, spotMarketOrderIndicators, derivativeMarketOrderDirections, stakingInfo,
	)

	// Wait for modified position cache building to complete
	wg.Wait()

	/* =========== Stage 2: Persist market order execution to store =========== */
	// Persist Spot market order execution data
	tradingRewards := h.k.PersistSpotMarketOrderExecution(ctx, batchSpotExecutionData, spotVwapData)

	// Cancel/refund spot market orders for markets where the FBA goroutine panicked (nil result).
	h.cleanupTransientSpotMarketOrders(ctx, batchSpotExecutionData, spotMarketOrderIndicators)

	// Process triggering conditional limit orders
	h.handleConditionalLimitOrderCancels(ctx, triggeredMarketsAndOrders)
	h.handleTriggeringConditionalLimitOrders(ctx, triggeredMarketsAndOrders)

	// Initialize derivative market funding info
	derivativeVwapData := v2.NewDerivativeVwapInfo()

	// Persist Derivative market order execution data
	var insolventMarkets map[common.Hash]struct{}
	tradingRewards, insolventMarkets = h.k.PersistDerivativeMarketOrderExecution(
		ctx, batchDerivativeExecutionData, derivativeVwapData, tradingRewards, modifiedPositionCache,
	)

	// Remove consumed market orders from transient storage so subsequent stages (e.g.
	// stage 3 limit-order last-look) don't double-count their exposure.
	h.cleanupTransientDerivativeMarketOrders(ctx, batchDerivativeExecutionData, derivativeMarketOrderDirections, insolventMarkets)

	// Re-fetch after market-order execution and conditional limit-order triggering,
	// since new market directions may have been added.
	derivativeLimitOrderMarketDirections = h.k.GetAllTransientDerivativeMarketDirections(ctx, true)

	// Refresh the modified-position cache for any markets that were added after
	// conditional-limit triggering. Without this, reduce-only conflict pruning in
	// GetFilteredTransientOrdersAndOrdersToCancel would be skipped for these
	// markets (HasAnyModifiedPositionsInMarket would return false).
	for _, m := range derivativeLimitOrderMarketDirections {
		if modifiedPositionCache.HasAnyModifiedPositionsInMarket(m.MarketId) {
			continue
		}

		subaccountIDs := h.k.GetModifiedSubaccountsByMarket(ctx, m.MarketId)
		if subaccountIDs == nil || len(subaccountIDs.SubaccountIds) == 0 {
			continue
		}

		for _, subaccountID := range subaccountIDs.SubaccountIds {
			modifiedPositionCache.SetPositionIndicator(m.MarketId, common.BytesToHash(subaccountID))
		}
	}

	/* =========== Stage 3: Process all limit orders in parallel =========== */

	spotLimitOrderMarketDirections := h.k.GetAllTransientMatchedSpotLimitOrderMarkets(ctx)

	// Execute FBA (Frequent Batch Auction) for limit orders
	batchSpotMatchingExecutionData, batchDerivativeMatchingExecutionData := batchAuction.ExecuteLimitOrders(
		ctx,
		spotLimitOrderMarketDirections,
		derivativeLimitOrderMarketDirections,
		stakingInfo,
		modifiedPositionCache,
	)

	/* =========== Stage 4: Persist limit order matching execution + new limit orders to store =========== */
	// Persist Spot Matching execution data
	tradingRewards = h.k.PersistSpotMatchingExecution(ctx, batchSpotMatchingExecutionData, spotVwapData, tradingRewards)

	// Persist Derivative Limit order matching execution data
	tradingRewards = h.k.PersistDerivativeMatchingExecution(ctx, batchDerivativeMatchingExecutionData, derivativeVwapData, tradingRewards)

	// Clean up transient limit orders for markets that were skipped due to panic (nil execution
	// data). Unlike stage-1 market orders which have cleanupTransientDerivativeMarketOrders,
	// stage-3 limit orders previously had no fail-closed cleanup — a panic would silently strand
	// locked margin and stale orderbook metadata. The insolvency case is handled inside
	// PersistDerivativeMatchingExecution itself.
	h.cleanupTransientLimitOrders(ctx,
		batchSpotMatchingExecutionData, spotLimitOrderMarketDirections,
		batchDerivativeMatchingExecutionData, derivativeLimitOrderMarketDirections,
	)

	/* =========== Stage 5: Update perpetual market funding info =========== */

	atomicVwapData := h.k.GetAllAtomicPerpetualVwap(ctx)
	derivativeVwapData.MergePerpetualVwap(atomicVwapData)

	h.k.PersistVwapInfo(ctx, &spotVwapData, &derivativeVwapData)

	syntheticFundingVwapData := h.k.GetAllSyntheticPerpetualFundingVwap(ctx)
	derivativeVwapData.MergePerpetualVwap(syntheticFundingVwapData)

	h.k.PersistPerpetualFundingInfo(ctx, derivativeVwapData)
	h.k.PersistTradingRewardPoints(ctx, tradingRewards)
	h.k.PersistFeeDiscountStakingInfoUpdates(ctx, stakingInfo)

	/* =========== Stage 6: Process Spot Market Param Updates if any =========== */
	h.k.IterateSpotMarketParamUpdates(ctx, func(p *v2.SpotMarketParamUpdateProposal) (stop bool) {
		err := h.k.ExecuteSpotMarketParamUpdateProposal(ctx, p)
		if err != nil {
			ctx.Logger().Error(err.Error())
		}
		return false
	})

	/* =========== Stage 7: Process Derivative Market Param Updates if any =========== */
	h.k.IterateDerivativeMarketParamUpdates(ctx, func(p *v2.DerivativeMarketParamUpdateProposal) (stop bool) {
		err := h.k.ExecuteDerivativeMarketParamUpdateProposal(ctx, p)
		if err != nil {
			ctx.Logger().Error(err.Error())
		}
		return false
	})

	/* =========== Stage 8: Process Derivative Market Param Updates if any =========== */
	h.k.IterateBinaryOptionsMarketParamUpdates(ctx, func(p *v2.BinaryOptionsMarketParamUpdateProposal) (stop bool) {
		err := h.k.ExecuteBinaryOptionsMarketParamUpdateProposal(ctx, p)
		if err != nil {
			ctx.Logger().Error(err.Error())
		}
		return false
	})

	/* =========== Stage 9: Invalidate conditional RO orders if no locked margin left =========== */
	h.k.IterateInvalidConditionalOrderFlags(ctx, func(marketID, subaccountID common.Hash, isBuy bool) (stop bool) {
		h.invalidateConditionalOrdersIfNoMarginLocked(ctx, marketID, subaccountID, isBuy, marketCache)
		return false
	})

	/* =========== Stage 10: Emit Deposit, Position and Orderbook Update Events =========== */
	h.k.EmitAllTransientDepositUpdates(ctx)
	h.k.EmitAllTransientPositionUpdates(ctx)
	h.k.IncrementSequenceAndEmitAllTransientOrderbookUpdates(ctx)
}

// cleanupTransientSpotMarketOrders cancels and refunds transient spot market orders for markets
// where the FBA goroutine panicked (nil execution data). Without this, a recovered panic would
// drop the orders at block end but their balance holds would never be refunded.
func (h *BlockHandler) cleanupTransientSpotMarketOrders(
	ctx sdk.Context,
	batchData []*v2.SpotBatchExecutionData,
	indicators []*v2.MarketOrderIndicator,
) {
	defer h.k.Meter(ctx).FuncTiming(&ctx, "cleanupTransientSpotMarketOrders")()

	for i, execData := range batchData {
		if execData != nil {
			continue // successfully processed — PersistSpotMarketOrderExecution handles cleanup
		}

		if i >= len(indicators) || indicators[i] == nil {
			continue
		}

		marketID := common.HexToHash(indicators[i].MarketId)
		market := h.k.GetSpotMarketByID(ctx, marketID)
		if market == nil {
			continue
		}

		// Cancel all transient spot market orders in this market (both directions).
		for _, isBuy := range []bool{true, false} {
			orders := h.k.GetAllTransientSpotMarketOrders(ctx, marketID, isBuy)
			for _, order := range orders {
				h.k.CancelTransientSpotMarketOrder(ctx, market, marketID, order)
			}
		}
	}
}

// cleanupTransientDerivativeMarketOrders removes transient market orders after stage-1.
// This is intentionally in the FBA batch path (not inside the persistence function) because
// PersistSingleDerivativeMarketOrderExecution is also used by immediate execution paths
// (atomic orders, liquidations) which must not delete unrelated queued market orders.
//
// When a market is disabled or has no mark price, executeDerivativeMarketOrder returns nil.
// We must still delete the transient market orders for that market, otherwise stage-3
// cross-margin risk calculations will see unmatchable leftovers that inflate OLR or trigger
// oracle-failure paths. The directions slice provides the marketID for nil entries.
func (h *BlockHandler) cleanupTransientDerivativeMarketOrders(
	ctx sdk.Context,
	batchData []*v2.DerivativeBatchExecutionData,
	directions []*types.MatchedMarketDirection,
	insolventMarkets map[common.Hash]struct{},
) {
	defer h.k.Meter(ctx).FuncTiming(&ctx, "cleanupTransientDerivativeMarketOrders")()

	for i, execData := range batchData {
		var marketID common.Hash
		switch {
		case execData != nil:
			marketID = execData.Market.MarketID()
		case i < len(directions) && directions[i] != nil:
			marketID = directions[i].MarketId
		default:
			continue
		}

		if execData == nil {
			// Market was disabled or had no mark price — orders were never matched.
			// Cancel with refund instead of bare deletion to avoid losing margin holds.
			h.k.CancelUnprocessedTransientDerivativeMarketOrders(ctx, marketID)
		} else if _, insolvent := insolventMarkets[marketID]; insolvent {
			// Market was matched but found insolvent during persistence. PersistSingleDerivativeMarketOrderExecution
			// returned early without applying deposit deltas (including refunds for unfilled orders),
			// so individual order margins are still locked. Cancel with refund to release them.
			h.k.CancelUnprocessedTransientDerivativeMarketOrders(ctx, marketID)
		} else {
			// Orders were processed during matching — just clean up the transient store.
			h.k.DeleteConsumedTransientDerivativeMarketOrders(ctx, marketID)
		}
	}
}

// cleanupTransientLimitOrders cancels transient limit orders for markets where stage-3 FBA
// matching returned nil (panic recovery). Without this, a goroutine panic would silently strand
// locked margin (derivative) and balance holds (spot), and leave orderbook metadata stale.
// The insolvency case for derivatives is handled inside PersistDerivativeMatchingExecution.
func (h *BlockHandler) cleanupTransientLimitOrders(
	ctx sdk.Context,
	batchSpotData []*v2.SpotBatchExecutionData,
	spotDirections []*types.MatchedMarketDirection,
	batchDerivativeData []*v2.DerivativeBatchExecutionData,
	derivativeDirections []*types.MatchedMarketDirection,
) {
	defer h.k.Meter(ctx).FuncTiming(&ctx, "cleanupTransientLimitOrders")()

	h.cleanupPanickedDerivativeLimitOrders(ctx, batchDerivativeData, derivativeDirections)
	h.cleanupPanickedSpotLimitOrders(ctx, batchSpotData, spotDirections)
}

func (h *BlockHandler) cleanupPanickedDerivativeLimitOrders(
	ctx sdk.Context,
	batchData []*v2.DerivativeBatchExecutionData,
	directions []*types.MatchedMarketDirection,
) {
	for i, execData := range batchData {
		if execData != nil || i >= len(directions) || directions[i] == nil {
			continue
		}

		marketID := directions[i].MarketId
		// Use GetDerivativeMarketByID to find both enabled and disabled markets.
		// The mark-price-requiring lookup would skip disabled/no-oracle markets,
		// stranding transient orders without refund.
		var market v2.DerivativeMarketI
		if m := h.k.GetDerivativeMarketByID(ctx, marketID); m != nil {
			market = m
		} else if m := h.k.GetBinaryOptionsMarketByID(ctx, marketID); m != nil {
			market = m
		}
		if market == nil {
			continue
		}

		ctx.Logger().Error("stage-3 derivative limit matching returned nil — cancelling transient orders with refund",
			"marketID", marketID.Hex(),
		)
		h.k.CancelAllTransientDerivativeLimitOrders(ctx, market)
	}
}

func (h *BlockHandler) cleanupPanickedSpotLimitOrders(
	ctx sdk.Context,
	batchData []*v2.SpotBatchExecutionData,
	directions []*types.MatchedMarketDirection,
) {
	for i, execData := range batchData {
		if execData != nil || i >= len(directions) || directions[i] == nil {
			continue
		}

		marketID := directions[i].MarketId
		market := h.k.GetSpotMarket(ctx, marketID, true)
		if market == nil {
			market = h.k.GetSpotMarket(ctx, marketID, false) // try disabled markets
		}
		if market == nil {
			continue
		}

		ctx.Logger().Error("stage-3 spot limit matching returned nil — cancelling transient orders with refund",
			"marketID", marketID.Hex(),
		)
		h.k.CancelAllTransientSpotLimitOrdersForMarket(ctx, market)
	}
}

func (h *BlockHandler) handleConditionalMarketOrderCancels(ctx sdk.Context, triggeredMarketsAndOrders []*v2.TriggeredOrdersInMarket) {
	defer h.k.Meter(ctx).FuncTiming(&ctx, "BlockHandler.handleConditionalMarketOrderCancels")()
	// cancel conditional orders first on ctx so we can trigger them on separate cacheCtx
	for _, triggeredMarket := range triggeredMarketsAndOrders {
		if triggeredMarket == nil {
			continue
		}

		h.cancelTriggeredMarketOrdersForMarket(ctx, triggeredMarket)
	}
}

func (h *BlockHandler) cancelTriggeredMarketOrdersForMarket(ctx sdk.Context, triggeredMarket *v2.TriggeredOrdersInMarket) {
	defer h.k.Meter(ctx).FuncTiming(&ctx, "BlockHandler.cancelTriggeredMarketOrdersForMarket")()

	for i, marketOrder := range triggeredMarket.MarketOrders {
		if marketOrder == nil {
			continue
		}

		subaccID := marketOrder.OrderInfo.SubaccountID()

		// Skip CM-paused subaccounts: the order stays as a conditional in state and will
		// re-trigger naturally when the pause is lifted. Deleting it here would permanently
		// lose the order since the subsequent creation attempt would also fail.
		if h.k.RiskEngine().CheckCrossMarginEmergencyPause(ctx, subaccID) != nil {
			triggeredMarket.MarketOrders[i] = nil
			continue
		}

		if panicked, err := h.k.CancelConditionalDerivativeMarketOrderWithCache(
			ctx, triggeredMarket.Market, subaccID, nil, marketOrder.Hash(),
		); panicked {
			triggeredMarket.MarketOrders[i] = nil
			ctx.Logger().Error("Cancelling of conditional market order panicked")
		} else if err != nil {
			// should never happen
			// remove the order from the array of orders to trigger since we couldn't cancel it
			triggeredMarket.MarketOrders[i] = nil
			ctx.Logger().Debug("Cancelling of conditional market order failed: ", err.Error())
		}
	}
}

func (h *BlockHandler) handleTriggeringConditionalMarketOrders(ctx sdk.Context, triggeredMarketsAndOrders []*v2.TriggeredOrdersInMarket) {
	defer h.k.Meter(ctx).FuncTiming(&ctx, "BlockHandler.handleTriggeringConditionalMarketOrders")()

	// try with one big cacheCtx first for performance reasons, fall back on individual cacheCtx if panicked
	cacheCtx, writeCache := ctx.CacheContext()
	// bank charge should fail if the account no longer has permissions to send the tokens
	cacheCtx = cacheCtx.WithValue(baseapp.DoNotFailFastSendContextKey, nil)

	if isPanicked := h.executeTriggeredMarketOrders(
		cacheCtx, triggeredMarketsAndOrders, triggerMarketOrdersForMarketWithoutCache,
	); !isPanicked {
		writeCache()
	} else {
		h.executeTriggeredMarketOrders(ctx, triggeredMarketsAndOrders, triggerMarketOrdersForMarketWithCache)
	}
}

func (h *BlockHandler) executeTriggeredMarketOrders(
	ctx sdk.Context,
	triggeredMarketsAndOrders []*v2.TriggeredOrdersInMarket,
	triggerFn func(sdk.Context, *keeper.Keeper, *v2.TriggeredOrdersInMarket),
) (isPanicked bool) {
	defer h.k.Meter(ctx).FuncTiming(&ctx, "BlockHandler.executeTriggeredMarketOrders")()

	defer RecoverEndBlocker(ctx, &isPanicked)

	for _, triggeredMarket := range triggeredMarketsAndOrders {
		if triggeredMarket == nil {
			continue
		}

		triggerFn(ctx, h.k, triggeredMarket)
		h.updateTransientOrderIndicators(ctx, triggeredMarket)
	}
	return false // will be overwritten by deferred call
}

func (h *BlockHandler) updateTransientOrderIndicators(ctx sdk.Context, triggeredMarket *v2.TriggeredOrdersInMarket) {
	defer h.k.Meter(ctx).FuncTiming(&ctx, "BlockHandler.updateTransientOrderIndicators")()

	if triggeredMarket.HasLimitBuyOrders {
		h.k.SetTransientDerivativeLimitOrderIndicator(ctx, triggeredMarket.Market.MarketID(), true)
	}
	if triggeredMarket.HasLimitSellOrders {
		h.k.SetTransientDerivativeLimitOrderIndicator(ctx, triggeredMarket.Market.MarketID(), false)
	}
}

func triggerMarketOrdersForMarketWithCache(ctx sdk.Context, k *keeper.Keeper, triggeredMarket *v2.TriggeredOrdersInMarket) {
	for _, marketOrder := range triggeredMarket.MarketOrders {
		if marketOrder == nil {
			continue
		}
		triggerMarketOrderWithCache(ctx, k, triggeredMarket, marketOrder)
	}
}

func triggerMarketOrdersForMarketWithoutCache(ctx sdk.Context, k *keeper.Keeper, triggeredMarket *v2.TriggeredOrdersInMarket) {
	for _, marketOrder := range triggeredMarket.MarketOrders {
		if marketOrder == nil {
			continue
		}
		triggerMarketOrderWithoutCache(ctx, k, triggeredMarket, marketOrder)
	}
}

func triggerMarketOrderWithCache(
	ctx sdk.Context,
	k *keeper.Keeper,
	triggeredMarket *v2.TriggeredOrdersInMarket,
	marketOrder *v2.DerivativeMarketOrder,
) {
	var unused bool
	defer RecoverEndBlocker(ctx, &unused)

	cacheCtx, writeCache := ctx.CacheContext()
	// bank charge should fail if the account no longer has permissions to send the tokens
	cacheCtx = cacheCtx.WithValue(baseapp.DoNotFailFastSendContextKey, nil)

	if err := k.TriggerConditionalDerivativeMarketOrder(
		cacheCtx, triggeredMarket.Market, triggeredMarket.MarkPrice, marketOrder,
	); err != nil {
		ctx.Logger().Debug("Trigger of market order failed: ", err.Error())
		k.EmitEvent(
			ctx, &v2.EventTriggerConditionalMarketOrderFailed{
				MarketId:     triggeredMarket.Market.MarketId,
				SubaccountId: marketOrder.OrderInfo.SubaccountId,
				MarkPrice:    triggeredMarket.MarkPrice,
				OrderHash:    marketOrder.OrderHash,
				TriggerErr:   err.Error(),
				Cid:          marketOrder.OrderInfo.Cid,
			},
		)
		return // don't commit partial/failed creation state
	}
	writeCache()
}

func triggerMarketOrderWithoutCache(
	ctx sdk.Context,
	k *keeper.Keeper,
	triggeredMarket *v2.TriggeredOrdersInMarket,
	marketOrder *v2.DerivativeMarketOrder,
) {
	if err := k.TriggerConditionalDerivativeMarketOrder(
		ctx, triggeredMarket.Market, triggeredMarket.MarkPrice, marketOrder,
	); err != nil {
		ctx.Logger().Debug("Trigger of market order failed: ", err.Error())
		k.EmitEvent(
			ctx, &v2.EventTriggerConditionalMarketOrderFailed{
				MarketId:     triggeredMarket.Market.MarketId,
				SubaccountId: marketOrder.OrderInfo.SubaccountId,
				MarkPrice:    triggeredMarket.MarkPrice,
				OrderHash:    marketOrder.OrderHash,
				TriggerErr:   err.Error(),
				Cid:          marketOrder.OrderInfo.Cid,
			},
		)
	}
}

func (h *BlockHandler) handleConditionalLimitOrderCancels(ctx sdk.Context, triggeredMarketsAndOrders []*v2.TriggeredOrdersInMarket) {
	defer h.k.Meter(ctx).FuncTiming(&ctx, "BlockHandler.handleConditionalLimitOrderCancels")()
	// Trigger Conditional Limit Orders (after market orders matching is done, so we won't hit the limitation of one market order per block)
	for _, triggeredMarket := range triggeredMarketsAndOrders {
		if triggeredMarket == nil {
			continue
		}

		h.cancelConditionalOrdersForMarket(ctx, triggeredMarket)
	}
}

// cancelConditionalOrdersForMarket handles cancellation of limit orders for a specific market
func (h *BlockHandler) cancelConditionalOrdersForMarket(ctx sdk.Context, triggeredMarket *v2.TriggeredOrdersInMarket) {
	defer h.k.Meter(ctx).FuncTiming(&ctx, "BlockHandler.cancelConditionalOrdersForMarket")()

	for i, limitOrder := range triggeredMarket.LimitOrders {
		if limitOrder == nil {
			continue
		}

		subaccID := limitOrder.OrderInfo.SubaccountID()

		// Skip CM-paused subaccounts: the order stays as a conditional in state and will
		// re-trigger naturally when the pause is lifted. Deleting it here would permanently
		// lose the order since the subsequent creation attempt would also fail.
		if h.k.RiskEngine().CheckCrossMarginEmergencyPause(ctx, subaccID) != nil {
			triggeredMarket.LimitOrders[i] = nil
			continue
		}

		if panicked, err := h.cancelConditionalLimitOrderWithRecover(ctx, triggeredMarket, limitOrder); panicked {
			triggeredMarket.LimitOrders[i] = nil
			ctx.Logger().Error("Cancelling of conditional limit order panicked")
		} else if err != nil {
			// should never happen
			// remove the order from the array of orders to trigger since we couldn't cancel it
			triggeredMarket.LimitOrders[i] = nil
			ctx.Logger().Debug("Cancelling of conditional limit order failed: ", err.Error())
		}
	}

	// NOTE: we intentionally do NOT delete the transient limit-order indicators here.
	// updateTransientOrderIndicators already set them based on HasLimit*Orders flags.
	// If all triggered conditionals were paused, the stale indicator causes an unnecessary
	// but harmless stage-3 FBA run that correctly matches any crossing resting orders.
	// Deleting the indicator would be unsafe: it's keyed by (marketID, side) and would
	// also suppress stage-3 processing for unrelated transient limit orders placed earlier
	// in the block on the same market/side.
}

func (h *BlockHandler) cancelConditionalLimitOrderWithRecover(
	ctx sdk.Context,
	triggeredMarket *v2.TriggeredOrdersInMarket,
	limitOrder *v2.DerivativeLimitOrder,
) (panicked bool, err error) {
	cacheCtx, writeCache := ctx.CacheContext()
	subaccID := limitOrder.OrderInfo.SubaccountID()

	func() {
		defer RecoverEndBlocker(ctx, &panicked)
		err = h.k.CancelConditionalDerivativeLimitOrder(
			cacheCtx, triggeredMarket.Market, subaccID, nil, limitOrder.Hash(),
		)
	}()

	if panicked || err != nil {
		return panicked, err
	}

	writeCache()
	return false, nil
}

func (h *BlockHandler) invalidateConditionalOrdersIfNoMarginLocked(
	ctx sdk.Context,
	marketID, subaccountID common.Hash,
	isBuy bool,
	marketCache map[common.Hash]*v2.DerivativeMarket,
) {
	panicked := false

	func() {
		defer RecoverEndBlocker(ctx, &panicked)
		h.k.InvalidateConditionalOrdersIfNoMarginLocked(ctx, marketID, subaccountID, false, &isBuy, marketCache)
	}()

	if panicked {
		ctx.Logger().Error("Invalidating conditional orders panicked")
	}
}

func (h *BlockHandler) handleTriggeringConditionalLimitOrders(ctx sdk.Context, triggeredMarketsAndOrders []*v2.TriggeredOrdersInMarket) {
	defer h.k.Meter(ctx).FuncTiming(&ctx, "BlockHandler.handleTriggeringConditionalLimitOrders")()

	triggerLimitOrders := func(
		ctx sdk.Context,
		triggerFn func(sdk.Context, *keeper.Keeper, *v2.TriggeredOrdersInMarket, *v2.DerivativeLimitOrder),
	) (isPanicked bool) {
		defer RecoverEndBlocker(ctx, &isPanicked)

		for _, triggeredMarket := range triggeredMarketsAndOrders {
			if triggeredMarket == nil {
				continue
			}

			triggerLimitOrdersForMarket(ctx, h.k, triggeredMarket, triggerFn)
		}
		return false // will be overwritten by deferred call
	}
	// first try to trigger all orders in one big cacheCtx and in case of a panic abandon all changes and start again executing
	// each order in it's own separate cacheCtx so only bad orders won't be triggered
	cacheCtx, writeCache := ctx.CacheContext()
	// bank charge should fail if the account no longer has permissions to send the tokens
	cacheCtx = cacheCtx.WithValue(baseapp.DoNotFailFastSendContextKey, nil)

	if isPanicked := triggerLimitOrders(cacheCtx, triggerLimitOrderWithoutCache); !isPanicked {
		writeCache()
	} else {
		triggerLimitOrders(ctx, triggerLimitOrderWithCache)
	}
}

func triggerLimitOrdersForMarket(
	ctx sdk.Context,
	k *keeper.Keeper,
	triggeredMarket *v2.TriggeredOrdersInMarket,
	triggerFn func(sdk.Context, *keeper.Keeper, *v2.TriggeredOrdersInMarket, *v2.DerivativeLimitOrder),
) {
	for _, limitOrder := range triggeredMarket.LimitOrders {
		if limitOrder == nil {
			continue
		}
		triggerFn(ctx, k, triggeredMarket, limitOrder)
	}
}

func triggerLimitOrderWithCache(
	ctx sdk.Context, k *keeper.Keeper, triggeredMarket *v2.TriggeredOrdersInMarket, limitOrder *v2.DerivativeLimitOrder,
) {
	var unused bool
	defer RecoverEndBlocker(ctx, &unused)

	cacheCtx, writeCache := ctx.CacheContext()
	// bank charge should fail if the account no longer has permissions to send the tokens
	cacheCtx = cacheCtx.WithValue(baseapp.DoNotFailFastSendContextKey, nil)

	if err := k.TriggerConditionalDerivativeLimitOrder(
		cacheCtx, triggeredMarket.Market, triggeredMarket.MarkPrice, limitOrder, true,
	); err != nil {
		ctx.Logger().Debug("Trigger of limit order failed: ", err.Error())
		k.EmitEvent(
			ctx, &v2.EventTriggerConditionalLimitOrderFailed{
				MarketId:     triggeredMarket.Market.MarketId,
				SubaccountId: limitOrder.OrderInfo.SubaccountId,
				MarkPrice:    triggeredMarket.MarkPrice,
				OrderHash:    limitOrder.OrderHash,
				TriggerErr:   err.Error(),
				Cid:          limitOrder.OrderInfo.Cid,
			},
		)
		return // don't commit partial/failed creation state
	}
	writeCache()
}

func triggerLimitOrderWithoutCache(
	ctx sdk.Context, k *keeper.Keeper, triggeredMarket *v2.TriggeredOrdersInMarket, limitOrder *v2.DerivativeLimitOrder,
) {
	if err := k.TriggerConditionalDerivativeLimitOrder(
		ctx, triggeredMarket.Market, triggeredMarket.MarkPrice, limitOrder, true,
	); err != nil {
		ctx.Logger().Debug("Trigger of limit order failed: ", err.Error())
		k.EmitEvent(
			ctx, &v2.EventTriggerConditionalLimitOrderFailed{
				MarketId:     triggeredMarket.Market.MarketId,
				SubaccountId: limitOrder.OrderInfo.SubaccountId,
				MarkPrice:    triggeredMarket.MarkPrice,
				OrderHash:    limitOrder.OrderHash,
				TriggerErr:   err.Error(),
				Cid:          limitOrder.OrderInfo.Cid,
			},
		)
	}
}

// processDowntimePostOnlyMode checks if the current block is the first block after a detected downtime
// and activates post-only mode if the downtime exceeds the configured MinPostOnlyModeDowntimeDuration
func (h *BlockHandler) processDowntimePostOnlyMode(ctx sdk.Context, params v2.Params) {
	defer h.k.Meter(ctx).FuncTiming(&ctx, "BlockHandler.processDowntimePostOnlyMode")()
	// Skip if MinPostOnlyModeDowntimeDuration is empty or if exchange is already in post-only mode
	if params.MinPostOnlyModeDowntimeDuration == "" || h.k.IsPostOnlyMode(ctx) {
		return
	}

	// Get the Downtime enum value from the string parameter
	downtimeValue, exists := downtimetypes.Downtime_value[params.MinPostOnlyModeDowntimeDuration]
	if !exists {
		ctx.Logger().Error("Invalid MinPostOnlyModeDowntimeDuration", "value", params.MinPostOnlyModeDowntimeDuration)
		return
	}
	downtimeEnum := downtimetypes.Downtime(downtimeValue)

	// Get the last downtime of the specified duration from the downtime detector
	lastDowntimeBlockTime, err := h.k.DowntimeKeeper.GetLastDowntimeOfLength(ctx, downtimeEnum)
	if err != nil {
		// No downtime recorded for this duration, nothing to do
		return
	}

	// Check if the current block time matches the last recorded downtime block time
	// This means this is the first block after the detected downtime
	if ctx.BlockTime().Equal(lastDowntimeBlockTime) {
		// Activate post-only mode by setting PostOnlyModeHeightThreshold
		newThreshold := ctx.BlockHeight() + int64(params.PostOnlyModeBlocksAmountAfterDowntime)

		// Update the params with the new threshold
		updatedParams := params
		updatedParams.PostOnlyModeHeightThreshold = newThreshold
		h.k.SetParams(ctx, updatedParams)

		ctx.Logger().Info(
			"Post-only mode activated due to downtime detection",
			"downtime_duration", params.MinPostOnlyModeDowntimeDuration,
			"current_height", ctx.BlockHeight(),
			"post_only_until_height", newThreshold,
			"downtime_block_time", lastDowntimeBlockTime,
		)
	}
}

// processPostOnlyModeCancellation checks if the post-only mode cancellation flag is set
// and disables post-only mode if requested by governance or exchange admins
func (h *BlockHandler) processPostOnlyModeCancellation(ctx sdk.Context) {
	defer h.k.Meter(ctx).FuncTiming(&ctx, "BlockHandler.processPostOnlyModeCancellation")()
	// Check if the cancellation flag is set
	if !h.k.HasPostOnlyModeCancellationFlag(ctx) {
		return
	}

	// Disable post-only mode by setting threshold to current height - 1
	params := h.k.GetParams(ctx)
	params.PostOnlyModeHeightThreshold = ctx.BlockHeight() - 1
	h.k.SetParams(ctx, params)

	// Remove the cancellation flag
	h.k.DeletePostOnlyModeCancellationFlag(ctx)

	ctx.Logger().Info(
		"Post-only mode cancelled via governance/admin action",
		"current_height", ctx.BlockHeight(),
		"new_post_only_mode_threshold", ctx.BlockHeight()-1,
	)
}

func RecoverEndBlocker(ctx sdk.Context, isPanicked *bool) {
	if r := recover(); r != nil {
		if e, ok := r.(error); ok {
			ctx.Logger().Error("EndBlocker panicked with an error: ", e)
			ctx.Logger().Error(string(debug.Stack()))
		} else {
			ctx.Logger().Error("EndBlocker panicked with a msg: ", r)
		}
		*isPanicked = true
	} else {
		*isPanicked = false
	}
}
