package risk

import (
	"bytes"
	"maps"
	"slices"

	"cosmossdk.io/errors"
	"cosmossdk.io/math"
	storetypes "cosmossdk.io/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"

	exchangetypes "github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/types"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/types/v2"
)

// MergeAndSortMarketIDs deduplicates and lexicographically sorts market IDs from multiple sources.
// This is used to build a canonical, deterministic list of markets for cross-margin snapshot computation.
func MergeAndSortMarketIDs(sources ...[]common.Hash) []common.Hash {
	totalCap := 0
	for _, s := range sources {
		totalCap += len(s)
	}

	seen := make(map[common.Hash]struct{}, totalCap)
	result := make([]common.Hash, 0, totalCap)

	for _, source := range sources {
		for _, marketID := range source {
			if _, ok := seen[marketID]; ok {
				continue
			}
			seen[marketID] = struct{}{}
			result = append(result, marketID)
		}
	}

	slices.SortFunc(result, func(a, b common.Hash) int {
		return bytes.Compare(a.Bytes(), b.Bytes())
	})

	return result
}

// CrossMarginDeps provides the read-only state required to compute cross-margin risk.
//
// Implementations must be deterministic and must not mutate state.
type CrossMarginDeps interface {
	// QuoteBalance returns the subaccount's available quote balance in *chain format* (exchange deposits only).
	// Bank balance is excluded because it can be moved via MsgSend without exchange-module risk checks.
	QuoteBalance(ctx sdk.Context, subaccountID common.Hash, quoteDenom string) math.LegacyDec

	// ActiveDerivativeMarketsBySubaccount returns all derivative market IDs where the subaccount has an active position.
	ActiveDerivativeMarketsBySubaccount(ctx sdk.Context, subaccountID common.Hash) []common.Hash

	// ActiveDerivativeOrderMarketsBySubaccount returns all derivative market IDs where the subaccount has active
	// derivative orders (resting/conditional across blocks).
	ActiveDerivativeOrderMarketsBySubaccount(ctx sdk.Context, subaccountID common.Hash) []common.Hash

	// TransientDerivativeOrderIndicatorMarketsBySubaccount returns all derivative market IDs where the subaccount
	// has placed transient derivative orders in this block.
	TransientDerivativeOrderIndicatorMarketsBySubaccount(ctx sdk.Context, subaccountID common.Hash) []common.Hash

	// DerivativeMarketInfo returns market + mark price + funding (for perpetuals).
	//
	// For non-perpetual markets, funding may be nil.
	DerivativeMarketInfo(ctx sdk.Context, marketID common.Hash) (market *v2.DerivativeMarket, markPrice math.LegacyDec, funding *v2.PerpetualMarketFunding)

	// DerivativeMarket returns market metadata without requiring mark-price availability.
	// This is used in paths that need market identity/quote-denom/type but should not depend on oracle health.
	DerivativeMarket(ctx sdk.Context, marketID common.Hash) *v2.DerivativeMarket

	// Position returns the stored position for (market, subaccount) or nil.
	Position(ctx sdk.Context, marketID, subaccountID common.Hash) *v2.Position

	// SubaccountOrderbookMetadata returns the stored orderbook metadata for (market, subaccount, direction).
	SubaccountOrderbookMetadata(ctx sdk.Context, marketID, subaccountID common.Hash, isBuy bool) *v2.SubaccountOrderbookMetadata

	// IterateSubaccountOrders iterates over the subaccount's derivative limit orders for (market, direction),
	// using the canonical (price-time) ordering.
	IterateSubaccountOrders(
		ctx sdk.Context,
		marketID common.Hash,
		subaccountID common.Hash,
		isBuy bool,
		process func(order *v2.SubaccountOrder) (stop bool),
	)

	// GetTransientDerivativeMarketOrderForSubaccount returns the single transient derivative market order
	// for (market, subaccount, direction), or nil if none exists.
	GetTransientDerivativeMarketOrderForSubaccount(ctx sdk.Context, marketID, subaccountID common.Hash, isBuy bool) *v2.DerivativeMarketOrder

	// HasTransientDerivativeMarketOrderForSubaccount returns true if the subaccount has at least one
	// transient derivative market order in the given market and direction.
	HasTransientDerivativeMarketOrderForSubaccount(ctx sdk.Context, marketID, subaccountID common.Hash, isBuy bool) bool

	// IterateDerivativeMarketOrdersBySubaccount iterates over the subaccount's transient derivative market orders
	// for (market, direction).
	IterateDerivativeMarketOrdersBySubaccount(
		ctx sdk.Context,
		marketID common.Hash,
		subaccountID common.Hash,
		isBuy bool,
		process func(order *v2.DerivativeMarketOrder) (stop bool),
	)

	// IterateTransientDerivativeLimitOrdersBySubaccount iterates over the subaccount's transient derivative
	// limit orders for (market, direction) placed in this block.
	IterateTransientDerivativeLimitOrdersBySubaccount(
		ctx sdk.Context,
		marketID common.Hash,
		isBuy bool,
		subaccountID common.Hash,
		process func(order *v2.DerivativeLimitOrder) (stop bool),
	)

	// AtomicMarketOrderFeeMultiplier returns the fee multiplier for atomic orders in the given market.
	// This multiplier is applied on top of the taker fee rate for atomic order execution.
	AtomicMarketOrderFeeMultiplier(ctx sdk.Context, marketID common.Hash, marketType exchangetypes.MarketType) math.LegacyDec

	// ObjectStore returns the block-scoped object store for caching cross-margin snapshots.
	ObjectStore(ctx sdk.Context) storetypes.ObjKVStore
}

// CrossPoolSnapshot is the canonical risk snapshot for a (subaccount, quote-denom) cross pool.
//
// All values are in *human* units (i.e. notional units), consistent with existing derivative matching math.
type CrossPoolSnapshot struct {
	QuoteDenom string

	QuoteBalance        math.LegacyDec
	PositionMarginTotal math.LegacyDec
	UnrealizedPnl       math.LegacyDec
	UnrealizedPnlEff    math.LegacyDec

	EquityAdmission   math.LegacyDec
	EquityLiquidation math.LegacyDec

	InitialMarginTotal     math.LegacyDec
	MaintenanceMarginTotal math.LegacyDec

	// InitialMarginWithOrdersTotal is the summed initial margin requirement across markets
	// using worst-case net exposure per market (q + B, q - S).
	InitialMarginWithOrdersTotal math.LegacyDec
	EntryLossTotal               math.LegacyDec
	FeeReserveTotal              math.LegacyDec
	OrderLockRequirement         math.LegacyDec

	PositiveUPnLHaircutRate math.LegacyDec

	// HealthFactor is the ratio of EquityLiquidation to MaintenanceMarginTotal.
	// When < 1, the account is liquidatable. When < 1.5, the account is in a warning state.
	// Returns nil if MaintenanceMarginTotal is zero (no positions).
	HealthFactor *math.LegacyDec
}

// StripPositiveUPnL returns adjusted equity values with positive UPnL removed.
// During emergency pause, withdrawals are allowed only up to isolated-margin maintenance level,
// meaning positive UPnL from one position cannot subsidise another's maintenance shortfall.
func (s *CrossPoolSnapshot) StripPositiveUPnL(equityLiq, equityAdm math.LegacyDec) (adjLiq, adjAdm math.LegacyDec) {
	if s.UnrealizedPnl.IsPositive() {
		equityLiq = equityLiq.Sub(s.UnrealizedPnl)
	}
	if s.UnrealizedPnlEff.IsPositive() {
		equityAdm = equityAdm.Sub(s.UnrealizedPnlEff)
	}
	return equityLiq, equityAdm
}

// Copy returns a deep copy of the snapshot, safe for mutation without affecting the original.
// This must be kept in sync with the struct layout — adding pointer fields requires updating this method.
func (s *CrossPoolSnapshot) Copy() CrossPoolSnapshot {
	c := CrossPoolSnapshot{
		QuoteDenom:                   s.QuoteDenom,
		QuoteBalance:                 s.QuoteBalance.Clone(),
		PositionMarginTotal:          s.PositionMarginTotal.Clone(),
		UnrealizedPnl:                s.UnrealizedPnl.Clone(),
		UnrealizedPnlEff:             s.UnrealizedPnlEff.Clone(),
		EquityAdmission:              s.EquityAdmission.Clone(),
		EquityLiquidation:            s.EquityLiquidation.Clone(),
		InitialMarginTotal:           s.InitialMarginTotal.Clone(),
		MaintenanceMarginTotal:       s.MaintenanceMarginTotal.Clone(),
		InitialMarginWithOrdersTotal: s.InitialMarginWithOrdersTotal.Clone(),
		EntryLossTotal:               s.EntryLossTotal.Clone(),
		FeeReserveTotal:              s.FeeReserveTotal.Clone(),
		OrderLockRequirement:         s.OrderLockRequirement.Clone(),
		PositiveUPnLHaircutRate:      s.PositiveUPnLHaircutRate.Clone(),
	}
	if s.HealthFactor != nil {
		hf := s.HealthFactor.Clone()
		c.HealthFactor = &hf
	}
	return c
}

func DefaultPositiveUPnLHaircut() math.LegacyDec {
	return math.LegacyNewDecWithPrec(5, 1) // 50%
}

const DefaultCrossMarginMaxActiveDerivativeMarketsPerPool uint32 = 100

func EffectiveUPnL(uPnL, positiveHaircut math.LegacyDec) math.LegacyDec {
	if !uPnL.IsPositive() {
		return uPnL
	}
	return uPnL.Mul(math.LegacyOneDec().Sub(positiveHaircut))
}

// crossMarginParams returns the haircut and fees buffer from module params, or defaults if not wired.
// NOTE: Defensive nil checks provide graceful degradation during tests or partial initialisation.
// For production, call EnsureWired() during keeper startup to fail fast.
func (e *Engine) CrossMarginParams(ctx sdk.Context) (haircut, feesBuffer math.LegacyDec) {
	haircut = DefaultPositiveUPnLHaircut()
	feesBuffer = math.LegacyZeroDec()

	if e == nil {
		return haircut, feesBuffer
	}
	defer e.Meter(ctx).FuncTiming(&ctx, "Engine.CrossMarginParams")()

	if e.params == nil {
		return haircut, feesBuffer
	}

	params := e.params.GetParams(ctx)
	if !params.CrossMarginParams.PositiveUpnlHaircutRate.IsNil() {
		haircut = params.CrossMarginParams.PositiveUpnlHaircutRate
	}
	if !params.CrossMarginParams.FeesBuffer.IsNil() {
		feesBuffer = params.CrossMarginParams.FeesBuffer
	}

	return haircut, feesBuffer
}

func (e *Engine) CrossMarginMaxActiveDerivativeMarketsPerPool(ctx sdk.Context) uint32 {
	maxMarkets := DefaultCrossMarginMaxActiveDerivativeMarketsPerPool

	if e == nil {
		return maxMarkets
	}
	defer e.Meter(ctx).FuncTiming(&ctx, "Engine.crossMarginMaxActiveDerivativeMarketsPerPool")()

	if e.params == nil {
		return maxMarkets
	}

	params := e.params.GetParams(ctx)
	if params.CrossMarginParams.MaxActiveDerivativeMarketsPerPool != 0 {
		maxMarkets = params.CrossMarginParams.MaxActiveDerivativeMarketsPerPool
	}

	return maxMarkets
}

func (e *Engine) CrossMarginEligibility(ctx sdk.Context) (enabledQuoteDenoms map[string]struct{}, allowPerpetual, allowExpiry bool) {
	enabledQuoteDenoms = make(map[string]struct{})
	allowPerpetual = true
	allowExpiry = false

	if e == nil {
		return enabledQuoteDenoms, allowPerpetual, allowExpiry
	}
	defer e.Meter(ctx).FuncTiming(&ctx, "Engine.crossMarginEligibility")()

	if e.params == nil {
		return enabledQuoteDenoms, allowPerpetual, allowExpiry
	}

	params := e.params.GetParams(ctx)
	for _, denom := range params.CrossMarginParams.EnabledQuoteDenoms {
		enabledQuoteDenoms[denom] = struct{}{}
	}

	allowPerpetual = params.CrossMarginParams.PerpetualEnabled
	allowExpiry = params.CrossMarginParams.ExpiryEnabled

	return enabledQuoteDenoms, allowPerpetual, allowExpiry
}

// subaccountHasMarketExposure checks whether the subaccount has live exposure in a given market:
// a non-zero position, persistent vanilla orders, or transient orders (limit or market).
// Markets with no exposure can be safely skipped when oracle is unavailable,
// since they contribute nothing to the snapshot's risk calculations.
//
//nolint:revive // flag-parameter: marketID and subaccountID are natural identifiers, not flags
func (e *Engine) subaccountHasMarketExposure(ctx sdk.Context, marketID, subaccountID common.Hash) bool {
	defer e.Meter(ctx).FuncTiming(&ctx, "Engine.subaccountHasMarketExposure")()

	// Position check.
	if pos := e.cross.Position(ctx, marketID, subaccountID); pos != nil && !pos.Quantity.IsZero() {
		return true
	}

	// Persistent vanilla order check (orderbook metadata aggregates).
	for _, isBuy := range []bool{true, false} {
		if meta := e.cross.SubaccountOrderbookMetadata(ctx, marketID, subaccountID, isBuy); meta != nil {
			if !meta.AggregateVanillaQuantity.IsNil() && meta.AggregateVanillaQuantity.IsPositive() {
				return true
			}
		}
	}

	// Transient market order check (vanilla only — reduce-only orders contribute no OLR/maintenance risk).
	for _, isBuy := range []bool{true, false} {
		if o := e.cross.GetTransientDerivativeMarketOrderForSubaccount(ctx, marketID, subaccountID, isBuy); o != nil && o.IsVanilla() && !o.OrderInfo.Quantity.IsZero() {
			return true
		}
	}

	// Transient limit order check (vanilla only).
	for _, isBuy := range []bool{true, false} {
		found := false
		e.cross.IterateTransientDerivativeLimitOrdersBySubaccount(ctx, marketID, isBuy, subaccountID, func(order *v2.DerivativeLimitOrder) (stop bool) {
			if order != nil && order.IsVanilla() && !order.Fillable.IsZero() {
				found = true
				return true
			}
			return false
		})
		if found {
			return true
		}
	}

	return false
}

func (e *Engine) buildCrossPoolSnapshot(
	ctx sdk.Context,
	subaccountID common.Hash,
	quoteDenom string,
	quoteDecimals uint32,
) (*CrossPoolSnapshot, map[common.Hash]math.LegacyDec, error) {
	if e == nil || e.cross == nil {
		return nil, nil, errors.Wrap(exchangetypes.ErrInvalidState, "cross-margin deps not wired")
	}
	defer e.Meter(ctx).FuncTiming(&ctx, "Engine.buildCrossPoolSnapshot")()

	if quoteDecimals == 0 {
		return nil, nil, errors.Wrapf(exchangetypes.ErrInvalidDenomDecimal, "invalid quoteDecimals=0 for denom %s", quoteDenom)
	}

	chainQuoteBalance := e.cross.QuoteBalance(ctx, subaccountID, quoteDenom)
	quoteBalance := exchangetypes.NotionalFromChainFormat(chainQuoteBalance, quoteDecimals)

	marketIDs := MergeAndSortMarketIDs(
		e.cross.ActiveDerivativeMarketsBySubaccount(ctx, subaccountID),
		e.cross.ActiveDerivativeOrderMarketsBySubaccount(ctx, subaccountID),
		e.cross.TransientDerivativeOrderIndicatorMarketsBySubaccount(ctx, subaccountID),
	)

	var agg snapshotAggregator
	agg.init(len(marketIDs))

	for _, marketID := range marketIDs {
		rm, err := e.resolveMarketForSnapshot(ctx, marketID, subaccountID, quoteDenom, quoteDecimals)
		if err != nil {
			return nil, nil, err
		}
		if rm.market == nil {
			continue // filtered out (wrong denom, binary options, no exposure, etc.)
		}

		agg.buildMarkPrices[marketID] = rm.markPrice
		signedPosQty := agg.addPosition(e.cross, ctx, marketID, subaccountID, rm.market, rm.markPrice, rm.funding)
		agg.addOrders(e.cross, ctx, marketID, subaccountID, rm.market, rm.markPrice, signedPosQty)
	}

	snap, markPrices := agg.finalise(e, ctx, quoteDenom, quoteBalance)
	return snap, markPrices, nil
}

// resolvedMarket bundles the data returned by resolveMarketForSnapshot.
type resolvedMarket struct {
	market    *v2.DerivativeMarket
	markPrice math.LegacyDec
	funding   *v2.PerpetualMarketFunding
}

// resolveMarketForSnapshot validates and returns market data for a single market within a snapshot build.
// Returns a zero resolvedMarket (market==nil) when the market should be skipped, or an error for hard failures.
func (e *Engine) resolveMarketForSnapshot(
	ctx sdk.Context,
	marketID, subaccountID common.Hash,
	quoteDenom string,
	quoteDecimals uint32,
) (resolvedMarket, error) {
	market, markPrice, funding := e.cross.DerivativeMarketInfo(ctx, marketID)

	if market == nil {
		return e.resolveNilMarket(ctx, marketID, subaccountID, quoteDenom)
	}

	if market.QuoteDenom != quoteDenom || market.GetMarketType().IsBinaryOptions() {
		return resolvedMarket{}, nil
	}

	if market.QuoteDecimals != quoteDecimals {
		return resolvedMarket{}, errors.Wrapf(exchangetypes.ErrInvalidDenomDecimal,
			"inconsistent quote decimals in cross-pool: market %s has %d, expected %d (quote denom %s)",
			marketID.Hex(), market.QuoteDecimals, quoteDecimals, quoteDenom,
		)
	}

	if markPrice.IsNil() || !markPrice.IsPositive() {
		// Eligibility does NOT override fail-closed here: a grandfathered position on an
		// ineligible-but-exposed market still owes MM to the pool. Skipping by eligibility
		// alone would silently drop that MM liability, letting a subaccount withdraw
		// collateral or evade liquidation while live exposure remains. requireExposureOrSkip
		// already short-circuits harmlessly for subaccounts without exposure on this market.
		return e.requireExposureOrSkip(ctx, marketID, subaccountID, exchangetypes.ErrInvalidOracle, "missing/invalid mark price for market %s", marketID.Hex())
	}

	return resolvedMarket{market: market, markPrice: markPrice, funding: funding}, nil
}

// resolveNilMarket handles the case where DerivativeMarketInfo returned a nil market.
// This happens both when the market doesn't exist and when the oracle lookup fails.
func (e *Engine) resolveNilMarket(
	ctx sdk.Context,
	marketID, subaccountID common.Hash,
	quoteDenom string,
) (resolvedMarket, error) {
	m := e.cross.DerivativeMarket(ctx, marketID)
	if m == nil || m.QuoteDenom != quoteDenom || m.GetMarketType().IsBinaryOptions() {
		return resolvedMarket{}, nil
	}
	// Eligibility does NOT skip here when exposure is live: see the matching rationale in
	// resolveMarketForSnapshot. requireExposureOrSkip fails closed only for subaccounts with
	// actual exposure on this market, preserving MM accounting for grandfathered positions.
	return e.requireExposureOrSkip(ctx, marketID, subaccountID, exchangetypes.ErrInvalidOracle, "oracle unavailable for market %s", marketID.Hex())
}

// requireExposureOrSkip returns an error if the subaccount has live exposure in the market, otherwise skips.
func (e *Engine) requireExposureOrSkip(
	ctx sdk.Context,
	marketID, subaccountID common.Hash,
	baseErr *errors.Error, msgFmt string, args ...any,
) (resolvedMarket, error) {
	if !e.subaccountHasMarketExposure(ctx, marketID, subaccountID) {
		return resolvedMarket{}, nil
	}
	return resolvedMarket{}, errors.Wrapf(baseErr, msgFmt, args...)
}

// snapshotAggregator accumulates per-market contributions during snapshot building.
type snapshotAggregator struct {
	buildMarkPrices map[common.Hash]math.LegacyDec
	positionMargin  math.LegacyDec
	uPnL            math.LegacyDec
	imPositions     math.LegacyDec
	mmPositions     math.LegacyDec
	imWithOrders    math.LegacyDec
	entryLoss       math.LegacyDec
	feeReserve      math.LegacyDec
}

func (a *snapshotAggregator) init(n int) {
	a.buildMarkPrices = make(map[common.Hash]math.LegacyDec, n)
	a.positionMargin = math.LegacyZeroDec()
	a.uPnL = math.LegacyZeroDec()
	a.imPositions = math.LegacyZeroDec()
	a.mmPositions = math.LegacyZeroDec()
	a.imWithOrders = math.LegacyZeroDec()
	a.entryLoss = math.LegacyZeroDec()
	a.feeReserve = math.LegacyZeroDec()
}

// addPosition aggregates position-level risk data (margin, UPnL, IM/MM) for a single market.
func (a *snapshotAggregator) addPosition(
	deps CrossMarginDeps, ctx sdk.Context,
	marketID, subaccountID common.Hash,
	market *v2.DerivativeMarket, markPrice math.LegacyDec, funding *v2.PerpetualMarketFunding,
) math.LegacyDec {
	signedPosQty := math.LegacyZeroDec()
	position := deps.Position(ctx, marketID, subaccountID)
	if position == nil || position.Quantity.IsNil() || position.Quantity.IsZero() {
		return signedPosQty
	}

	// Deep copy to avoid mutating the stored position (LegacyDec wraps *big.Int).
	posCopy := position.Copy()
	_ = v2.ApplyFundingAndGetUpdatedPositionState(posCopy, funding)

	a.positionMargin = a.positionMargin.Add(posCopy.Margin)
	a.uPnL = a.uPnL.Add(posCopy.GetPayoutFromPnl(markPrice, posCopy.Quantity))

	if posCopy.IsLong {
		signedPosQty = posCopy.Quantity
	} else {
		signedPosQty = posCopy.Quantity.Neg()
	}

	notionalAbs := posCopy.Quantity.Abs().Mul(markPrice)
	a.imPositions = a.imPositions.Add(market.InitialMarginRatio.Mul(notionalAbs))
	a.mmPositions = a.mmPositions.Add(market.MaintenanceMarginRatio.Mul(notionalAbs))

	return signedPosQty
}

// addOrders aggregates order-level risk data (entry loss, fee reserve, worst-case exposure) for a single market.
func (a *snapshotAggregator) addOrders(
	deps CrossMarginDeps, ctx sdk.Context,
	marketID, subaccountID common.Hash,
	market *v2.DerivativeMarket, markPrice math.LegacyDec,
	signedPosQty math.LegacyDec,
) {
	feeRateWorst := WorstCaseFeeRate(market, math.LegacyOneDec())

	// Limit order quantities from stored aggregates.
	metaBuy := deps.SubaccountOrderbookMetadata(ctx, marketID, subaccountID, true)
	metaSell := deps.SubaccountOrderbookMetadata(ctx, marketID, subaccountID, false)

	buyQty := math.LegacyZeroDec()
	sellQty := math.LegacyZeroDec()
	if metaBuy != nil {
		buyQty = buyQty.Add(metaBuy.AggregateVanillaQuantity)
	}
	if metaSell != nil {
		sellQty = sellQty.Add(metaSell.AggregateVanillaQuantity)
	}

	// Limit orders: EntryLoss and FeeReserve from subaccount order store (vanilla only).
	for _, isBuy := range []bool{true, false} {
		deps.IterateSubaccountOrders(ctx, marketID, subaccountID, isBuy, func(order *v2.SubaccountOrder) (stop bool) {
			if order == nil || !order.IsVanilla() || order.Quantity.IsZero() {
				return false
			}
			a.entryLoss = a.entryLoss.Add(ComputeEntryLoss(isBuy, order.Price, markPrice, order.Quantity))
			a.feeReserve = a.feeReserve.Add(feeRateWorst.Mul(order.Price).Mul(order.Quantity))
			return false
		})
	}

	// Market orders (transient): quantities + EntryLoss/FeeReserve.
	buyQty, sellQty = a.addMarketOrders(deps, ctx, marketID, subaccountID, market, markPrice, feeRateWorst, buyQty, sellQty)

	// NOTE: Transient limit orders are already covered above — both quantities (via metadata)
	// and EntryLoss/FeeReserve (via IterateSubaccountOrders) include transient placements.

	absWorst := ComputeAbsWorstExposure(signedPosQty, buyQty, sellQty)
	a.imWithOrders = a.imWithOrders.Add(market.InitialMarginRatio.Mul(markPrice).Mul(absWorst))
}

// addMarketOrders iterates transient market orders and accumulates their quantities, entry loss, and fee reserve.
//
//nolint:revive // argument-limit: passing accumulators avoids extra struct indirection
func (a *snapshotAggregator) addMarketOrders(
	deps CrossMarginDeps, ctx sdk.Context,
	marketID, subaccountID common.Hash,
	market *v2.DerivativeMarket, markPrice, feeRateWorst math.LegacyDec,
	buyQty, sellQty math.LegacyDec,
) (math.LegacyDec, math.LegacyDec) {
	for _, isBuy := range []bool{true, false} {
		order := deps.GetTransientDerivativeMarketOrderForSubaccount(ctx, marketID, subaccountID, isBuy)
		if order == nil || !order.IsVanilla() || order.OrderInfo.Quantity.IsZero() {
			continue
		}
		qty := order.OrderInfo.Quantity
		px := order.OrderInfo.Price
		if isBuy {
			buyQty = buyQty.Add(qty)
		} else {
			sellQty = sellQty.Add(qty)
		}
		a.entryLoss = a.entryLoss.Add(ComputeEntryLoss(isBuy, px, markPrice, qty))

		orderFeeRate := feeRateWorst
		if order.OrderType.IsAtomic() {
			orderFeeRate = orderFeeRate.Mul(deps.AtomicMarketOrderFeeMultiplier(ctx, marketID, market.GetMarketType()))
		}
		a.feeReserve = a.feeReserve.Add(orderFeeRate.Mul(px).Mul(qty))
	}
	return buyQty, sellQty
}

// finalise computes the derived snapshot fields from the accumulated per-market data.
func (a *snapshotAggregator) finalise(
	e *Engine, ctx sdk.Context,
	quoteDenom string, quoteBalance math.LegacyDec,
) (*CrossPoolSnapshot, map[common.Hash]math.LegacyDec) {
	positiveHaircut, feesBuffer := e.CrossMarginParams(ctx)
	uPnLEff := EffectiveUPnL(a.uPnL, positiveHaircut)

	equityAdmission := quoteBalance.Add(a.positionMargin).Add(uPnLEff).Sub(feesBuffer)
	equityLiquidation := quoteBalance.Add(a.positionMargin).Add(a.uPnL).Sub(feesBuffer)
	orderLockRequirement := a.imWithOrders.Add(a.entryLoss).Add(a.feeReserve)

	var healthFactor *math.LegacyDec
	if a.mmPositions.IsPositive() {
		hf := equityLiquidation.Quo(a.mmPositions)
		healthFactor = &hf
	}

	return &CrossPoolSnapshot{
		QuoteDenom:   quoteDenom,
		QuoteBalance: quoteBalance,

		PositionMarginTotal: a.positionMargin,
		UnrealizedPnl:       a.uPnL,
		UnrealizedPnlEff:    uPnLEff,

		EquityAdmission:   equityAdmission,
		EquityLiquidation: equityLiquidation,

		InitialMarginTotal:     a.imPositions,
		MaintenanceMarginTotal: a.mmPositions,

		InitialMarginWithOrdersTotal: a.imWithOrders,
		EntryLossTotal:               a.entryLoss,
		FeeReserveTotal:              a.feeReserve,
		OrderLockRequirement:         orderLockRequirement,

		PositiveUPnLHaircutRate: positiveHaircut,
		HealthFactor:            healthFactor,
	}, a.buildMarkPrices
}

// BuildCrossPoolSnapshot computes the canonical cross-margin snapshot for a (subaccount, quoteDenom) pool.
//
// This is primarily intended for keeper-level orchestration (pre-pass, liquidation flows) and must remain
// read-only and deterministic.
//
//nolint:revive // confusing-naming: public BuildCrossPoolSnapshot wraps private buildCrossPoolSnapshot intentionally
func (e *Engine) BuildCrossPoolSnapshot(
	ctx sdk.Context,
	subaccountID common.Hash,
	quoteDenom string,
	quoteDecimals uint32,
) (*CrossPoolSnapshot, error) {
	defer e.Meter(ctx).FuncTiming(&ctx, "Engine.BuildCrossPoolSnapshot")()

	snap, _, err := e.buildCrossPoolSnapshot(ctx, subaccountID, quoteDenom, quoteDecimals)
	return snap, err
}

// CachedMarketParams records the per-market parameters used when building a snapshot.
// Changes to these (via governance proposal or MsgUpdateParams) invalidate the cache.
type CachedMarketParams struct {
	InitialMarginRatio     math.LegacyDec
	MaintenanceMarginRatio math.LegacyDec
	MakerFeeRate           math.LegacyDec
	TakerFeeRate           math.LegacyDec
}

// CachedSnapshotEntry stores a snapshot together with the market data and module params
// that were current when it was built. Storing this per-entry (rather than globally)
// prevents a later snapshot from overwriting an earlier one's validation baseline,
// which would allow stale snapshots to pass validation after mid-block param or oracle changes.
type CachedSnapshotEntry struct {
	Snapshot     *CrossPoolSnapshot
	MarkPrices   map[common.Hash]math.LegacyDec     // marketID → mark price at build time
	MarketParams map[common.Hash]CachedMarketParams // marketID → IM/MM/fee at build time
	HaircutRate  math.LegacyDec                     // CrossMarginPositiveUpnlHaircutRate at build time
	FeesBuffer   math.LegacyDec                     // CrossMarginFeesBuffer at build time
}

// CrossPoolSnapshotCache holds block-scoped cached snapshots keyed by (subaccountID, quoteDenom).
// All validation metadata (mark prices, market params, module params) is stored per entry so that
// each snapshot is validated against its own build-time baseline, not a shared global one.
type CrossPoolSnapshotCache struct {
	Entries map[common.Hash]map[string]*CachedSnapshotEntry
}

func GetSnapshotCache(objStore storetypes.ObjKVStore) *CrossPoolSnapshotCache {
	cached := objStore.Get(exchangetypes.ObjectCrossPoolSnapshotCacheKey)
	if cached == nil {
		return nil
	}
	if c, ok := cached.(*CrossPoolSnapshotCache); ok {
		return c
	}
	return nil
}

func SetSnapshotCache(objStore storetypes.ObjKVStore, cache *CrossPoolSnapshotCache) {
	objStore.Set(exchangetypes.ObjectCrossPoolSnapshotCacheKey, cache)
}

func CopySnapshotMaps(old *CrossPoolSnapshotCache, cloneKey common.Hash) *CrossPoolSnapshotCache {
	newCache := &CrossPoolSnapshotCache{
		Entries: make(map[common.Hash]map[string]*CachedSnapshotEntry),
	}
	if old == nil {
		return newCache
	}

	for k, v := range old.Entries {
		if k == cloneKey {
			// Deep-copy the target subaccount's inner denom map to prevent CacheContext leaks.
			cloned := make(map[string]*CachedSnapshotEntry, len(v)+1)
			for denom, entry := range v {
				clonedEntry := &CachedSnapshotEntry{
					Snapshot:     entry.Snapshot,
					MarkPrices:   make(map[common.Hash]math.LegacyDec, len(entry.MarkPrices)),
					MarketParams: make(map[common.Hash]CachedMarketParams, len(entry.MarketParams)),
					HaircutRate:  entry.HaircutRate,
					FeesBuffer:   entry.FeesBuffer,
				}
				maps.Copy(clonedEntry.MarkPrices, entry.MarkPrices)
				maps.Copy(clonedEntry.MarketParams, entry.MarketParams)
				cloned[denom] = clonedEntry
			}
			newCache.Entries[k] = cloned
		} else {
			newCache.Entries[k] = v
		}
	}

	return newCache
}

// CacheSnapshot stores a snapshot for (subaccountID, quoteDenom) in the object store.
// It deep-copies the target subaccount's inner denom map to prevent CacheContext leaks:
// without the copy, writing to the shared inner map would mutate the parent context's cache
// if a child CacheContext is later discarded.
//
// buildMarkPrices and buildMarketParams record the market data used when building this
// snapshot. They are stored per-entry (not globally) so each snapshot validates against its
// own build-time baseline — preventing a later snapshot from overwriting an earlier one's
// validation data after mid-block oracle or param changes.
func CacheSnapshot(
	objStore storetypes.ObjKVStore,
	subaccountID common.Hash,
	quoteDenom string,
	snapshot *CrossPoolSnapshot,
	buildMarkPrices map[common.Hash]math.LegacyDec,
	buildMarketParams map[common.Hash]CachedMarketParams,
) {
	newCache := CopySnapshotMaps(GetSnapshotCache(objStore), subaccountID)

	if newCache.Entries[subaccountID] == nil {
		newCache.Entries[subaccountID] = make(map[string]*CachedSnapshotEntry, 1)
	}

	// Get or create the entry for this (subaccount, quoteDenom).
	entry := newCache.Entries[subaccountID][quoteDenom]
	if entry == nil {
		entry = &CachedSnapshotEntry{
			MarkPrices:   make(map[common.Hash]math.LegacyDec, len(buildMarkPrices)),
			MarketParams: make(map[common.Hash]CachedMarketParams, len(buildMarketParams)),
		}
	}
	entry.Snapshot = snapshot

	// Merge-without-overwrite: preserve the build-time price/params for markets that are
	// already tracked by this entry. When CacheSnapshot is called from
	// ReserveDerivativeOrderMargin, buildMarkPrices contains only the caller market — we
	// must preserve the original build's values for other markets.
	for mID, price := range buildMarkPrices {
		if _, exists := entry.MarkPrices[mID]; !exists {
			entry.MarkPrices[mID] = price
		}
	}
	for mID, params := range buildMarketParams {
		if _, exists := entry.MarketParams[mID]; !exists {
			entry.MarketParams[mID] = params
		}
	}

	newCache.Entries[subaccountID][quoteDenom] = entry
	SetSnapshotCache(objStore, newCache)
}

// getOrBuildCrossPoolSnapshot returns a cached snapshot if available, otherwise builds and caches one.
// This avoids O(n²) snapshot rebuilds when multiple orders for the same (subaccount, quoteDenom) are
// placed within the same block.
//
// On cache hit, only the markets relevant to the requested snapshot are validated against the current
// oracle and market params. This makes cache-hit cost O(snapshot markets) instead of O(all cached markets),
// which is critical for pools with many markets. Module-level params (haircut, fees buffer) are also checked.
// tryGetCachedSnapshot returns a valid cached snapshot or nil. On stale data it evicts and returns nil.
func (e *Engine) tryGetCachedSnapshot(
	ctx sdk.Context,
	objStore storetypes.ObjKVStore,
	subaccountID common.Hash,
	quoteDenom string,
) *CrossPoolSnapshot {
	cache := GetSnapshotCache(objStore)
	if cache == nil {
		return nil
	}
	byDenom, ok := cache.Entries[subaccountID]
	if !ok {
		return nil
	}
	entry, ok := byDenom[quoteDenom]
	if !ok {
		return nil
	}
	if e.validateCachedEntryParams(ctx, entry) && e.validateCachedEntryMarkets(ctx, entry) {
		return entry.Snapshot
	}
	e.EvictCrossPoolSnapshotCache(ctx, subaccountID)
	return nil
}

func (e *Engine) GetOrBuildCrossPoolSnapshot(
	ctx sdk.Context,
	subaccountID common.Hash,
	quoteDenom string,
	quoteDecimals uint32,
) (*CrossPoolSnapshot, error) {
	defer e.Meter(ctx).FuncTiming(&ctx, "Engine.getOrBuildCrossPoolSnapshot")()

	objStore := e.cross.ObjectStore(ctx)

	if snap := e.tryGetCachedSnapshot(ctx, objStore, subaccountID, quoteDenom); snap != nil {
		return snap, nil
	}

	snapshot, buildMarkPrices, err := e.buildCrossPoolSnapshot(ctx, subaccountID, quoteDenom, quoteDecimals)
	if err != nil {
		return nil, err
	}

	// Collect per-market params for cache validation (detects governance param updates).
	buildMarketParams := make(map[common.Hash]CachedMarketParams, len(buildMarkPrices))
	for mID := range buildMarkPrices {
		market := e.cross.DerivativeMarket(ctx, mID)
		if market != nil {
			buildMarketParams[mID] = CachedMarketParams{
				InitialMarginRatio:     market.InitialMarginRatio,
				MaintenanceMarginRatio: market.MaintenanceMarginRatio,
				MakerFeeRate:           market.GetMakerFeeRate(),
				TakerFeeRate:           market.GetTakerFeeRate(),
			}
		}
	}

	CacheSnapshot(objStore, subaccountID, quoteDenom, snapshot, buildMarkPrices, buildMarketParams)

	// Record the module-level risk params used for this entry's build.
	haircut, fb := e.CrossMarginParams(ctx)
	if c := GetSnapshotCache(objStore); c != nil {
		if entry := c.Entries[subaccountID][quoteDenom]; entry != nil {
			entry.HaircutRate = haircut
			entry.FeesBuffer = fb
			SetSnapshotCache(objStore, c)
		}
	}

	return snapshot, nil
}

// validateCachedEntryParams checks whether a cached entry's module-level params still match current.
func (e *Engine) validateCachedEntryParams(ctx sdk.Context, entry *CachedSnapshotEntry) bool {
	if entry.HaircutRate.IsNil() && entry.FeesBuffer.IsNil() {
		return true
	}
	haircut, fb := e.CrossMarginParams(ctx)
	if !entry.HaircutRate.IsNil() && !entry.HaircutRate.Equal(haircut) {
		return false
	}
	if !entry.FeesBuffer.IsNil() && !entry.FeesBuffer.Equal(fb) {
		return false
	}
	return true
}

// validateCachedEntryMarkets checks whether a cached entry's market-level data (oracle prices,
// IM/MM/fee params) is still current. Each entry stores its own build-time values, so
// different snapshots sharing a market are validated independently.
func (e *Engine) validateCachedEntryMarkets(ctx sdk.Context, entry *CachedSnapshotEntry) bool {
	defer e.Meter(ctx).FuncTiming(&ctx, "Engine.validateCachedEntryMarkets")()

	for mID, cachedPrice := range entry.MarkPrices {
		_, currentPrice, _ := e.cross.DerivativeMarketInfo(ctx, mID)
		if currentPrice.IsNil() || !cachedPrice.Equal(currentPrice) {
			return false
		}
	}
	for mID, cp := range entry.MarketParams {
		market := e.cross.DerivativeMarket(ctx, mID)
		if market == nil {
			return false
		}
		if !cp.InitialMarginRatio.Equal(market.InitialMarginRatio) ||
			!cp.MaintenanceMarginRatio.Equal(market.MaintenanceMarginRatio) ||
			!cp.MakerFeeRate.Equal(market.GetMakerFeeRate()) ||
			!cp.TakerFeeRate.Equal(market.GetTakerFeeRate()) {
			return false
		}
	}
	return true
}

func RebuildCacheExcluding(cache *CrossPoolSnapshotCache, excludeID common.Hash) *CrossPoolSnapshotCache {
	newCache := &CrossPoolSnapshotCache{
		Entries: make(map[common.Hash]map[string]*CachedSnapshotEntry, len(cache.Entries)),
	}
	for k, v := range cache.Entries {
		if k != excludeID {
			newCache.Entries[k] = v
		}
	}
	return newCache
}

// ClearCrossPoolSnapshotCache removes the entire snapshot cache. Used after operations
// that run inside a CacheContext and mutate state for an unknown set of subaccounts
// (e.g. liquidation matching), where tracking individual evictions isn't feasible.
func (e *Engine) ClearCrossPoolSnapshotCache(ctx sdk.Context) {
	if e == nil || e.cross == nil {
		return
	}
	defer e.Meter(ctx).FuncTiming(&ctx, "Engine.ClearCrossPoolSnapshotCache")()
	SetSnapshotCache(e.cross.ObjectStore(ctx), nil)
}

// EvictCrossPoolSnapshotCache removes all cached snapshots for a given subaccount.
// This must be called whenever an equity-affecting mutation occurs (deposit changes,
// position changes, order cancellations).
func (e *Engine) EvictCrossPoolSnapshotCache(ctx sdk.Context, subaccountID common.Hash) {
	if e == nil {
		return
	}
	defer e.Meter(ctx).FuncTiming(&ctx, "Engine.EvictCrossPoolSnapshotCache")()

	if e.cross == nil {
		return
	}
	objStore := e.cross.ObjectStore(ctx)
	cache := GetSnapshotCache(objStore)
	if cache == nil || cache.Entries[subaccountID] == nil {
		return
	}

	newCache := RebuildCacheExcluding(cache, subaccountID)
	SetSnapshotCache(objStore, newCache)
}
