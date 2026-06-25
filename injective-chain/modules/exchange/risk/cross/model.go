package cross

import (
	"cosmossdk.io/errors"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"

	exchangetypes "github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/types"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/types/v2"

	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/risk"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/risk/isolated"
)

// Model implements cross-margin behaviour (per quote-denom pool, derivatives-only, FULL_HOLD / order locking).
//
// NOTE: Spot reservation/refunds keep isolated behaviour even for cross-mode subaccounts; spot-as-collateral is not
// supported by this model.
type Model struct {
	engine *risk.Engine
}

// NewModel creates a new cross-margin model backed by the given engine.
func NewModel(engine *risk.Engine) *Model {
	return &Model{engine: engine}
}

// checkCrossMarginEligibility validates that a market is eligible for cross-margin trading.
func (m *Model) checkCrossMarginEligibility(ctx sdk.Context, market v2.DerivativeMarketI) error {
	return m.engine.CheckCrossMarginMarketEligibility(ctx, market)
}

func (m *Model) ReserveSpotLimitOrder(
	ctx sdk.Context,
	funds risk.Funds,
	subaccountID common.Hash,
	order *v2.SpotOrder,
	market *v2.SpotMarket,
) error {
	defer m.engine.Meter(ctx).FuncTiming(&ctx, "cross.Model.ReserveSpotLimitOrder")()

	// Spot holds affect AvailableBalance → QuoteBalance → cross-pool equity, so they must
	// also be blocked during emergency pause to freeze all risk-changing activity.
	if err := m.engine.CheckCrossMarginEmergencyPause(ctx, subaccountID); err != nil {
		return err
	}
	err := isolated.NewModel().ReserveSpotLimitOrder(ctx, funds, subaccountID, order, market)
	if err == nil {
		// Spot holds mutate AvailableBalance which feeds cross QuoteBalance.
		m.engine.EvictCrossPoolSnapshotCache(ctx, subaccountID)
	}
	return err
}

func (m *Model) ReserveSpotMarketOrder(
	ctx sdk.Context,
	funds risk.Funds,
	subaccountID common.Hash,
	order *v2.SpotOrder,
	market *v2.SpotMarket,
	feeRate, bestPrice math.LegacyDec,
) (balanceHold math.LegacyDec, err error) {
	defer m.engine.Meter(ctx).FuncTiming(&ctx, "cross.Model.ReserveSpotMarketOrder")(&err)

	// Spot holds affect AvailableBalance → QuoteBalance → cross-pool equity, so they must
	// also be blocked during emergency pause to freeze all risk-changing activity.
	if err := m.engine.CheckCrossMarginEmergencyPause(ctx, subaccountID); err != nil {
		return math.LegacyZeroDec(), err
	}
	balanceHold, err = isolated.NewModel().ReserveSpotMarketOrder(ctx, funds, subaccountID, order, market, feeRate, bestPrice)
	if err == nil {
		// Spot holds mutate AvailableBalance which feeds cross QuoteBalance.
		m.engine.EvictCrossPoolSnapshotCache(ctx, subaccountID)
	}
	return balanceHold, err
}

func (m *Model) RefundSpotLimitOrderCancel(
	ctx sdk.Context,
	funds risk.Funds,
	subaccountID common.Hash,
	order *v2.SpotLimitOrder,
	market *v2.SpotMarket,
	isTransient bool,
) error {
	defer m.engine.Meter(ctx).FuncTiming(&ctx, "cross.Model.RefundSpotLimitOrderCancel")()

	err := isolated.NewModel().RefundSpotLimitOrderCancel(ctx, funds, subaccountID, order, market, isTransient)
	if err == nil {
		// Spot refunds mutate AvailableBalance which feeds cross QuoteBalance.
		m.engine.EvictCrossPoolSnapshotCache(ctx, subaccountID)
	}
	return err
}

//nolint:revive // cyclomatic: cross-margin admission requires multiple validation steps
func (m *Model) ReserveDerivativeOrderMargin(
	ctx sdk.Context,
	funds risk.Funds,
	subaccountID common.Hash,
	order *v2.DerivativeOrder,
	market v2.DerivativeMarketI,
	markPriceToCheck, tradeFeeRate math.LegacyDec,
) (marginHold math.LegacyDec, err error) {
	defer m.engine.Meter(ctx).FuncTiming(&ctx, "cross.Model.ReserveDerivativeOrderMargin")(&err)

	if !order.IsVanilla() {
		return math.LegacyZeroDec(), nil
	}

	if order.IsReduceOnly() {
		_ = funds
		return math.LegacyZeroDec(), nil
	}

	if err := m.checkCrossMarginEligibility(ctx, market); err != nil {
		return math.LegacyZeroDec(), err
	}

	maxActiveMarkets := m.engine.CrossMarginMaxActiveDerivativeMarketsPerPool(ctx)
	if err := m.validateCrossMarginActiveMarketLimit(ctx, subaccountID, market.GetQuoteDenom(), market.MarketID(), maxActiveMarkets); err != nil {
		return math.LegacyZeroDec(), err
	}

	if markPriceToCheck.IsNil() || !markPriceToCheck.IsPositive() {
		return math.LegacyZeroDec(), errors.Wrap(exchangetypes.ErrInvalidOracle, "missing/invalid mark price for cross-margin admission")
	}

	if _, err := order.CheckMarginAndGetMarginHold(
		market.GetInitialMarginRatio(),
		markPriceToCheck,
		tradeFeeRate,
		market.GetMarketType(),
		market.GetOracleScaleFactor(),
	); err != nil {
		return math.LegacyZeroDec(), err
	}

	if order.IsConditional() {
		_ = funds
		return math.LegacyZeroDec(), nil
	}

	snapshot, snapErr := m.engine.GetOrBuildCrossPoolSnapshot(ctx, subaccountID, market.GetQuoteDenom(), market.GetQuoteDecimals())
	if snapErr != nil {
		return math.LegacyZeroDec(), errors.Wrap(snapErr, "failed to build cross-margin snapshot")
	}

	if snapshot.EquityLiquidation.LT(snapshot.MaintenanceMarginTotal) {
		return math.LegacyZeroDec(), errors.Wrapf(
			exchangetypes.ErrInsufficientMargin,
			"cross-margin maintenance check failed for pool %s: equity %s < maintenance %s",
			market.GetQuoteDenom(),
			snapshot.EquityLiquidation.String(),
			snapshot.MaintenanceMarginTotal.String(),
		)
	}

	qty := order.OrderInfo.Quantity
	px := order.OrderInfo.Price
	atomicMul := math.LegacyOneDec()
	if order.OrderType.IsAtomic() {
		atomicMul = m.engine.CrossDeps().AtomicMarketOrderFeeMultiplier(ctx, market.MarketID(), market.GetMarketType())
	}
	feeRateWorst := risk.WorstCaseFeeRate(market, atomicMul)

	entryLoss := risk.ComputeEntryLoss(order.IsBuy(), px, markPriceToCheck, qty)
	feeReserve := feeRateWorst.Mul(px).Mul(qty)

	mls := risk.BuildMarketLockState(ctx, m.engine.CrossDeps(), market.MarketID(), subaccountID)
	absWorstBefore := risk.ComputeAbsWorstExposure(mls.SignedPosQty, mls.BuyQty, mls.SellQty)

	if order.IsBuy() {
		mls.BuyQty = mls.BuyQty.Add(qty)
	} else {
		mls.SellQty = mls.SellQty.Add(qty)
	}
	absWorstAfter := risk.ComputeAbsWorstExposure(mls.SignedPosQty, mls.BuyQty, mls.SellQty)

	deltaAbsWorst := absWorstAfter.Sub(absWorstBefore)
	if deltaAbsWorst.IsNegative() {
		deltaAbsWorst = math.LegacyZeroDec()
	}

	deltaIMWithOrders := market.GetInitialMarginRatio().Mul(markPriceToCheck).Mul(deltaAbsWorst)
	orderLockAfter := snapshot.OrderLockRequirement.Add(deltaIMWithOrders).Add(entryLoss).Add(feeReserve)

	if snapshot.EquityAdmission.LT(orderLockAfter) {
		return math.LegacyZeroDec(), errors.Wrapf(
			exchangetypes.ErrInsufficientMargin,
			"cross-margin admission check failed for pool %s: equity_admission %s < order_lock_after %s (delta_im_with_orders %s, entry_loss %s, fee_reserve %s, uPnL_haircut %s)",
			market.GetQuoteDenom(),
			snapshot.EquityAdmission.String(),
			orderLockAfter.String(),
			deltaIMWithOrders.String(),
			entryLoss.String(),
			feeReserve.String(),
			snapshot.PositiveUPnLHaircutRate.String(),
		)
	}

	updated := snapshot.Copy()
	updated.OrderLockRequirement = orderLockAfter
	updated.EntryLossTotal = updated.EntryLossTotal.Add(entryLoss)
	updated.FeeReserveTotal = updated.FeeReserveTotal.Add(feeReserve)
	updated.InitialMarginWithOrdersTotal = updated.InitialMarginWithOrdersTotal.Add(deltaIMWithOrders)

	callerMarkPrices := map[common.Hash]math.LegacyDec{market.MarketID(): markPriceToCheck}

	callerMarketParams := make(map[common.Hash]risk.CachedMarketParams, 1)
	if concreteMarket := m.engine.CrossDeps().DerivativeMarket(ctx, market.MarketID()); concreteMarket != nil {
		callerMarketParams[market.MarketID()] = risk.CachedMarketParams{
			InitialMarginRatio:     concreteMarket.InitialMarginRatio,
			MaintenanceMarginRatio: concreteMarket.MaintenanceMarginRatio,
			MakerFeeRate:           concreteMarket.GetMakerFeeRate(),
			TakerFeeRate:           concreteMarket.GetTakerFeeRate(),
		}
	}
	risk.CacheSnapshot(m.engine.CrossDeps().ObjectStore(ctx), subaccountID, market.GetQuoteDenom(), &updated, callerMarkPrices, callerMarketParams)

	_ = funds
	return math.LegacyZeroDec(), nil
}

// liveTransientDerivativeOrderMarkets returns market IDs where the subaccount has live (not yet
// cancelled/filled) transient derivative orders in this block.
func (m *Model) liveTransientDerivativeOrderMarkets(ctx sdk.Context, subaccountID common.Hash) []common.Hash {
	candidates := m.engine.CrossDeps().TransientDerivativeOrderIndicatorMarketsBySubaccount(ctx, subaccountID)
	if len(candidates) == 0 {
		return nil
	}

	live := make([]common.Hash, 0, len(candidates))
	for _, marketID := range candidates {
		if m.hasLiveTransientDerivativeOrder(ctx, marketID, subaccountID) {
			live = append(live, marketID)
		}
	}
	return live
}

func (m *Model) hasLiveTransientDerivativeOrder(ctx sdk.Context, marketID, subaccountID common.Hash) bool {
	found := false

	for _, isBuy := range []bool{true, false} {
		if found {
			break
		}
		m.engine.CrossDeps().IterateTransientDerivativeLimitOrdersBySubaccount(ctx, marketID, isBuy, subaccountID, func(_ *v2.DerivativeLimitOrder) (stop bool) {
			found = true
			return true
		})
	}

	for _, isBuy := range []bool{true, false} {
		if found {
			break
		}
		found = m.engine.CrossDeps().HasTransientDerivativeMarketOrderForSubaccount(ctx, marketID, subaccountID, isBuy)
	}

	return found
}

func (m *Model) validateCrossMarginActiveMarketLimit(
	ctx sdk.Context,
	subaccountID common.Hash,
	quoteDenom string,
	newMarketID common.Hash,
	maxActiveMarkets uint32,
) error {
	if maxActiveMarkets == 0 {
		maxActiveMarkets = risk.DefaultCrossMarginMaxActiveDerivativeMarketsPerPool
	}

	marketIDs := risk.MergeAndSortMarketIDs(
		m.engine.CrossDeps().ActiveDerivativeMarketsBySubaccount(ctx, subaccountID),
		m.engine.CrossDeps().ActiveDerivativeOrderMarketsBySubaccount(ctx, subaccountID),
		m.liveTransientDerivativeOrderMarkets(ctx, subaccountID),
	)

	hasMarketAlready := false
	activeCount := uint32(0)

	for _, marketID := range marketIDs {
		if marketID == newMarketID {
			hasMarketAlready = true
		}

		market, _, _ := m.engine.CrossDeps().DerivativeMarketInfo(ctx, marketID)
		if market == nil {
			market = m.engine.CrossDeps().DerivativeMarket(ctx, marketID)
		}
		if market == nil {
			continue
		}
		if market.GetMarketType().IsBinaryOptions() {
			continue
		}
		if market.QuoteDenom != quoteDenom {
			continue
		}
		activeCount++
	}

	if !hasMarketAlready && activeCount >= maxActiveMarkets {
		return errors.Wrapf(
			exchangetypes.ErrFeatureDisabled,
			"cross margin exceeds max active derivative markets per pool: %d",
			maxActiveMarkets,
		)
	}

	return nil
}

func (*Model) RefundDerivativeLimitOrderCancel(
	_ sdk.Context,
	_ risk.Funds,
	_ *v2.DerivativeLimitOrder,
	_ v2.MarketI,
	_ bool,
) error {
	return nil
}

func (*Model) RefundDerivativeMarketOrderCancel(
	_ sdk.Context, _ risk.Funds, _ common.Hash, _ v2.MarketI, _ math.LegacyDec,
) {
}

//nolint:revive // cyclomatic: last-look pruning has multiple conditional paths for cache management
func (m *Model) ShouldSkipDerivativeOrderForMarginRequirement(
	ctx sdk.Context,
	subaccountID common.Hash,
	order risk.DerivativeInitialMarginChecker,
	market v2.DerivativeMarketI,
	markPrice math.LegacyDec,
	remainingQty math.LegacyDec,
) (bool, error) {
	defer m.engine.Meter(ctx).FuncTiming(&ctx, "cross.Model.ShouldSkipDerivativeOrderForMarginRequirement")()

	if !order.IsVanilla() || market.GetMarketType() == exchangetypes.MarketType_BinaryOption {
		return false, nil
	}

	if err := m.checkCrossMarginEligibility(ctx, market); err != nil {
		m.engine.DecrementLastLookOLR(ctx, subaccountID, order, market, markPrice, remainingQty)
		return true, nil
	}

	if markPrice.IsNil() || !markPrice.IsPositive() {
		m.engine.DecrementLastLookOLR(ctx, subaccountID, order, market, markPrice, remainingQty)
		return true, errors.Wrap(exchangetypes.ErrInvalidOracle, "missing/invalid mark price for cross-margin last-look")
	}

	quoteDenom := market.GetQuoteDenom()
	quoteDecimals := market.GetQuoteDecimals()

	// Fast path: reuse existing pool state.
	if pool, hasPool := risk.GetLastLookPoolState(ctx, subaccountID, quoteDenom); hasPool && pool != nil {
		if !pool.EquityAdmission.LT(pool.OrderLockRequirement) {
			return false, nil
		}

		m.decrementOLRFromPool(ctx, pool, subaccountID, order, market, markPrice, remainingQty)
		return true, nil
	}

	// Slow path: build snapshot.
	var snapshot *risk.CrossPoolSnapshot
	if prepass, ok := risk.GetPrepassResult(ctx); ok && prepass != nil && prepass.CrossPoolSnapshots != nil {
		if byDenom, found := prepass.CrossPoolSnapshots[subaccountID]; found {
			snapshot = byDenom[quoteDenom]
		}
	}

	if snapshot == nil {
		s, _, err := m.engine.BuildCrossPoolSnapshotRaw(ctx, subaccountID, quoteDenom, quoteDecimals)
		if err != nil {
			m.engine.DecrementLastLookOLR(ctx, subaccountID, order, market, markPrice, remainingQty)
			return true, err
		}
		snapshot = s
	}

	pool, hasCache := risk.GetOrCreateLastLookPoolState(ctx, subaccountID, quoteDenom, snapshot)
	if hasCache {
		if !pool.EquityAdmission.LT(pool.OrderLockRequirement) {
			return false, nil
		}

		m.decrementOLRFromPool(ctx, pool, subaccountID, order, market, markPrice, remainingQty)
		return true, nil
	}

	return snapshot.EquityAdmission.LT(snapshot.OrderLockRequirement), nil
}

// decrementOLRFromPool extracts order params and decrements OLR on the given pool.
// If params cannot be extracted (unrecognised order type) or quantity is non-positive, this is a no-op.
func (m *Model) decrementOLRFromPool(
	ctx sdk.Context,
	pool *risk.CrossMarginLastLookPoolState,
	subaccountID common.Hash,
	order risk.DerivativeInitialMarginChecker,
	market v2.DerivativeMarketI,
	markPrice, remainingQty math.LegacyDec,
) {
	isBuy, _, px, ok := risk.ExtractOrderParams(order)
	if !ok || !remainingQty.IsPositive() {
		return
	}

	atomicMul := math.LegacyOneDec()
	if order.IsAtomic() {
		atomicMul = m.engine.CrossDeps().AtomicMarketOrderFeeMultiplier(ctx, market.MarketID(), market.GetMarketType())
	}
	feeRateWorst := risk.WorstCaseFeeRate(market, atomicMul)
	risk.DecrementLastLookOLROnPool(ctx, m.engine.CrossDeps(), pool, subaccountID, market, markPrice, isBuy, remainingQty, px, feeRateWorst)
}

//nolint:revive // argument-limit: signature matches Model interface
func (*Model) CheckValidPositionToReduce(
	ctx sdk.Context,
	subaccountID common.Hash,
	position *v2.Position,
	marketType exchangetypes.MarketType,
	orderPrice math.LegacyDec,
	isBuy bool,
	tradeFeeRate math.LegacyDec,
	funding *v2.PerpetualMarketFunding,
	closeExecutionMargin math.LegacyDec,
) error {
	// The per-position bankruptcy check is intentionally kept for cross-margin: allowing a close below
	// bankruptcy price would drain pool equity to cover an individual position's loss. Relaxing this
	// would only apply to a future portfolio-margin model with cross-asset offsetting.
	return isolated.NewModel().CheckValidPositionToReduce(ctx, subaccountID, position, marketType, orderPrice, isBuy, tradeFeeRate, funding, closeExecutionMargin)
}

func (*Model) ValidateDerivativePositionMarginDecrease(
	ctx sdk.Context,
	subaccountID common.Hash,
	position *v2.Position,
	market *v2.DerivativeMarket,
	markPrice math.LegacyDec,
) error {
	return isolated.NewModel().ValidateDerivativePositionMarginDecrease(ctx, subaccountID, position, market, markPrice)
}

func (m *Model) DerivativePositionLiquidationCheck(
	ctx sdk.Context,
	subaccountID common.Hash,
	position *v2.Position,
	market *v2.DerivativeMarket,
	markPrice math.LegacyDec,
	funding *v2.PerpetualMarketFunding,
) (liquidationPrice math.LegacyDec, shouldLiquidate bool, err error) {
	defer m.engine.Meter(ctx).FuncTiming(&ctx, "cross.Model.DerivativePositionLiquidationCheck")(&err)

	_ = markPrice
	snapshot, _, snapErr := m.engine.BuildCrossPoolSnapshotRaw(ctx, subaccountID, market.QuoteDenom, market.QuoteDecimals)
	if snapErr != nil {
		return math.LegacyZeroDec(), false, errors.Wrap(snapErr, "failed to build cross-margin snapshot")
	}

	shouldLiquidate = snapshot.EquityLiquidation.LT(snapshot.MaintenanceMarginTotal)
	liquidationPrice = position.GetLiquidationPrice(market.MaintenanceMarginRatio, funding)
	return liquidationPrice, shouldLiquidate, nil
}
