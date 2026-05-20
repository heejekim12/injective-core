//nolint:revive // max-public-structs: risk engine requires multiple public interface types
package risk

import (
	"context"
	"sync"

	"cosmossdk.io/errors"
	"cosmossdk.io/math"
	"github.com/InjectiveLabs/metrics/v2"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"

	exchangetypes "github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/types"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/types/v2"
)

type ProfileStore interface {
	GetEffectiveSubaccountRiskProfile(ctx sdk.Context, subaccountID common.Hash) (profile *v2.SubaccountRiskProfile, isDefault bool)
}

type ParamsProvider interface {
	GetParams(ctx sdk.Context) v2.Params
}

type Funds interface {
	ChargeAccount(ctx sdk.Context, subaccountID common.Hash, denom string, amount math.LegacyDec) error
	IncrementAvailableBalanceOrBank(ctx sdk.Context, subaccountID common.Hash, denom string, amount math.LegacyDec)
}

type DerivativeInitialMarginChecker interface {
	IsVanilla() bool
	IsAtomic() bool
	CheckInitialMarginRequirementMarkPriceThreshold(initialMarginRatio, markPrice math.LegacyDec) error
}

// ReadOnlyEngine exposes the runtime risk-engine methods without init-time setters.
// Sub-keepers return this interface from RiskEngine() to prevent callers from
// rewiring the engine's dependencies after construction.
type ReadOnlyEngine interface {
	EffectiveProfile(ctx sdk.Context, subaccountID common.Hash) (*v2.SubaccountRiskProfile, bool)
	MakeIsCrossSubaccountFn(ctx sdk.Context) func(common.Hash) bool

	BuildCrossPoolSnapshot(
		ctx sdk.Context, subaccountID common.Hash, quoteDenom string, quoteDecimals uint32,
	) (*CrossPoolSnapshot, error)
	EvictCrossPoolSnapshotCache(ctx sdk.Context, subaccountID common.Hash)
	ClearCrossPoolSnapshotCache(ctx sdk.Context)

	ReserveSpotLimitOrder(
		ctx sdk.Context, funds Funds, subaccountID common.Hash,
		order *v2.SpotOrder, market *v2.SpotMarket,
	) error
	ReserveSpotMarketOrder(
		ctx sdk.Context, funds Funds, subaccountID common.Hash,
		order *v2.SpotOrder, market *v2.SpotMarket,
		feeRate, bestPrice math.LegacyDec,
	) (math.LegacyDec, error)
	RefundSpotLimitOrderCancel(
		ctx sdk.Context, funds Funds, subaccountID common.Hash,
		order *v2.SpotLimitOrder, market *v2.SpotMarket, isTransient bool,
	) error

	ReserveDerivativeOrderMargin(
		ctx sdk.Context, funds Funds, subaccountID common.Hash,
		order *v2.DerivativeOrder, market v2.DerivativeMarketI,
		markPriceToCheck, tradeFeeRate math.LegacyDec,
	) (math.LegacyDec, error)
	RefundDerivativeLimitOrderCancel(
		ctx sdk.Context, funds Funds,
		order *v2.DerivativeLimitOrder, market v2.MarketI, isTransient bool,
	) error
	RefundDerivativeMarketOrderCancel(
		ctx sdk.Context, funds Funds, subaccountID common.Hash,
		market v2.MarketI, refundAmount math.LegacyDec,
	)
	ShouldSkipDerivativeOrderForMarginRequirement(
		ctx sdk.Context, subaccountID common.Hash,
		order DerivativeInitialMarginChecker, market v2.DerivativeMarketI,
		markPrice, remainingQty math.LegacyDec,
	) (bool, error)
	CheckValidPositionToReduce(
		ctx sdk.Context, subaccountID common.Hash,
		position *v2.Position, marketType exchangetypes.MarketType,
		orderPrice math.LegacyDec, isBuy bool, tradeFeeRate math.LegacyDec,
		funding *v2.PerpetualMarketFunding, closeExecutionMargin math.LegacyDec,
	) error
	ValidateDerivativePositionMarginDecrease(
		ctx sdk.Context, subaccountID common.Hash,
		position *v2.Position, market *v2.DerivativeMarket, markPrice math.LegacyDec,
	) error
	DerivativePositionLiquidationCheck(
		ctx sdk.Context, subaccountID common.Hash,
		position *v2.Position, market *v2.DerivativeMarket,
		markPrice math.LegacyDec, funding *v2.PerpetualMarketFunding,
	) (math.LegacyDec, bool, error)

	DecrementLastLookOLR(
		ctx sdk.Context, subaccountID common.Hash,
		order DerivativeInitialMarginChecker, market v2.DerivativeMarketI,
		markPrice, remainingQty math.LegacyDec,
	)
	CheckCrossMarginEmergencyPause(ctx sdk.Context, subaccountID common.Hash) error
	CheckCrossMarginMarketEligibility(ctx sdk.Context, market v2.DerivativeMarketI) error
}

// Engine is the module-level entry point for risk decisions and risk-sensitive state transitions.
//
// Models are selected based on `SubaccountRiskProfile`.
type Engine struct {
	profiles  ProfileStore
	cross     CrossMarginDeps
	params    ParamsProvider
	meter     metrics.Meter
	meterOnce sync.Once
}

func New(profiles ProfileStore) *Engine {
	return &Engine{
		profiles: profiles,
	}
}

// ModelFactory creates a Model implementation for a given engine.
type ModelFactory func(engine *Engine) Model

var (
	crossModelFactory    ModelFactory
	isolatedModelFactory ModelFactory
)

// RegisterCrossModelFactory registers the cross-margin model implementation.
// Must be called before Engine.model() is used (typically via init() in the cross sub-package).
func RegisterCrossModelFactory(f ModelFactory) {
	crossModelFactory = f
}

// RegisterIsolatedModelFactory registers the isolated-margin model implementation.
// Must be called before Engine.model() is used (typically via init() in the isolated sub-package).
func RegisterIsolatedModelFactory(f ModelFactory) {
	isolatedModelFactory = f
}

// CrossDeps returns the cross-margin dependencies wired into the engine.
func (e *Engine) CrossDeps() CrossMarginDeps {
	return e.cross
}

// BuildCrossPoolSnapshotRaw computes a fresh cross-margin snapshot, also returning per-market mark prices.
func (e *Engine) BuildCrossPoolSnapshotRaw(ctx sdk.Context, subaccountID common.Hash, quoteDenom string, quoteDecimals uint32) (*CrossPoolSnapshot, map[common.Hash]math.LegacyDec, error) {
	return e.buildCrossPoolSnapshot(ctx, subaccountID, quoteDenom, quoteDecimals)
}

// CheckCrossMarginMarketEligibility validates that a market is eligible for cross-margin trading.
// Returns nil if the market passes all checks. This is the canonical eligibility gate used by
// both the normal order admission flow and the wasm synthetic trade flow.
func (e *Engine) CheckCrossMarginMarketEligibility(ctx sdk.Context, market v2.DerivativeMarketI) error {
	enabledDenoms, allowPerpetual, allowExpiry := e.CrossMarginEligibility(ctx)

	if _, ok := enabledDenoms[market.GetQuoteDenom()]; !ok {
		return errors.Wrapf(
			exchangetypes.ErrFeatureDisabled,
			"cross margin is disabled for quote denom %s",
			market.GetQuoteDenom(),
		)
	}

	marketType := market.GetMarketType()
	if marketType.IsBinaryOptions() {
		return errors.Wrap(exchangetypes.ErrFeatureDisabled, "binary options are isolated-only")
	}
	if marketType.IsPerpetual() && !allowPerpetual {
		return errors.Wrap(exchangetypes.ErrFeatureDisabled, "cross margin is disabled for perpetual markets")
	}
	if marketType == exchangetypes.MarketType_Expiry && !allowExpiry {
		return errors.Wrap(exchangetypes.ErrFeatureDisabled, "cross margin is disabled for expiry markets")
	}
	if !marketType.IsPerpetual() && marketType != exchangetypes.MarketType_Expiry {
		return errors.Wrap(exchangetypes.ErrFeatureDisabled, "market type is not eligible for cross margin")
	}

	// Per-market opt-out (checked after type/denom gates for more specific error messages).
	if !market.IsCrossMarginEligible() {
		return errors.Wrapf(exchangetypes.ErrFeatureDisabled, "cross margin is disabled for this market")
	}

	return nil
}

// SetCrossMarginDeps wires the on-chain data dependencies required by the cross-margin model.
//
// This indirection avoids a construction-time dependency cycle between keepers.
func (e *Engine) SetCrossMarginDeps(deps CrossMarginDeps) {
	e.cross = deps
}

// SetParamsProvider wires access to module params for cross-margin calculations.
func (e *Engine) SetParamsProvider(provider ParamsProvider) {
	e.params = provider
}

// Meter returns the Engine's metrics sub-meter, lazily initialised from the SDK context.
// Nil-safe: returns a no-op meter when e is nil (e.g. unit tests with no engine wired).
func (e *Engine) Meter(ctx context.Context) metrics.Meter {
	if e == nil {
		return metrics.NewNilMeter()
	}
	e.meterOnce.Do(func() {
		ctxMeter := sdk.UnwrapSDKContext(ctx).Meter()
		if ctxMeter == nil {
			ctxMeter = metrics.NewNilMeter()
		}
		e.meter = ctxMeter.SubMeter("exchange_risk", metrics.Tag("svc", "exchange_risk"))
	})
	return e.meter
}

// EnsureWired validates that all dependencies required for cross-margin functionality are set.
// Call this during keeper initialisation to fail fast if wiring is incomplete.
// Returns an error describing any missing dependencies.
func (e *Engine) EnsureWired() error {
	if e == nil {
		return exchangetypes.ErrInvalidState.Wrap("risk engine is nil")
	}
	if e.profiles == nil {
		return exchangetypes.ErrInvalidState.Wrap("risk engine: profiles not wired")
	}
	if e.cross == nil {
		return exchangetypes.ErrInvalidState.Wrap("risk engine: cross-margin deps not wired")
	}
	if e.params == nil {
		return exchangetypes.ErrInvalidState.Wrap("risk engine: params provider not wired")
	}
	return nil
}

func (e *Engine) EffectiveProfile(ctx sdk.Context, subaccountID common.Hash) (*v2.SubaccountRiskProfile, bool) {
	if e == nil || e.profiles == nil {
		return nil, true
	}
	defer e.Meter(ctx).FuncTiming(&ctx, "Engine.EffectiveProfile")()
	return e.profiles.GetEffectiveSubaccountRiskProfile(ctx, subaccountID)
}

// MakeIsCrossSubaccountFn returns a cached closure that checks whether a subaccount is in cross-margin mode.
// Returns nil if the engine is not wired. The closure caches lookups for efficiency within a single pass.
func (e *Engine) MakeIsCrossSubaccountFn(ctx sdk.Context) func(common.Hash) bool {
	if e == nil || e.profiles == nil {
		return nil
	}
	defer e.Meter(ctx).FuncTiming(&ctx, "Engine.MakeIsCrossSubaccountFn")()

	cache := make(map[common.Hash]bool)
	return func(subaccountID common.Hash) bool {
		if isCross, ok := cache[subaccountID]; ok {
			return isCross
		}

		profile, _ := e.EffectiveProfile(ctx, subaccountID)
		isCross := profile != nil && profile.Mode == v2.RiskMode_RISK_MODE_CROSS
		cache[subaccountID] = isCross
		return isCross
	}
}

// DecrementLastLookOLR adjusts the stage-local last-look OLR cache to reflect a vanilla order
// that was cancelled by a pre-check path (e.g. CheckValidPositionToReduce, emergency pause)
// outside of ShouldSkipDerivativeOrderForMarginRequirement. Without this, the cached OLR remains
// inflated and subsequent orders from the same pool may be incorrectly cancelled.
//
// This is a no-op for isolated-margin subaccounts or when the last-look cache is not active.
// If the pool state does not yet exist (pre-check cancel before the first ShouldSkip call),
// it is created from the pre-pass or a fresh snapshot so the decrement is properly recorded.
//
//nolint:revive // cyclomatic: pool-state initialisation requires multiple fallback paths
func (e *Engine) DecrementLastLookOLR(
	ctx sdk.Context,
	subaccountID common.Hash,
	order DerivativeInitialMarginChecker,
	market v2.DerivativeMarketI,
	markPrice math.LegacyDec,
	remainingQty math.LegacyDec,
) {
	if e == nil || e.profiles == nil {
		return
	}
	defer e.Meter(ctx).FuncTiming(&ctx, "Engine.DecrementLastLookOLR")()

	// Only applies to cross-margin subaccounts with vanilla orders.
	if !order.IsVanilla() {
		return
	}

	profile, _ := e.EffectiveProfile(ctx, subaccountID)
	if profile == nil || profile.Mode != v2.RiskMode_RISK_MODE_CROSS {
		return
	}

	// Check whether the last-look cache is active (we are inside a matching pass).
	llCache, cacheActive := GetCrossMarginLastLookCache(ctx)
	if !cacheActive || llCache == nil {
		return
	}

	quoteDenom := market.GetQuoteDenom()

	// Try to retrieve the existing pool state first (fast path).
	pool, hasPool := GetLastLookPoolState(ctx, subaccountID, quoteDenom)
	if !hasPool {
		// Pool not yet created for this (subaccount, denom) — this happens when a pre-check
		// cancel fires before ShouldSkipDerivativeOrderForMarginRequirement creates the pool.
		// Build the snapshot and create the pool so the decrement is recorded in the baseline.
		var snapshot *CrossPoolSnapshot
		if prepass, ok := GetPrepassResult(ctx); ok && prepass != nil && prepass.CrossPoolSnapshots != nil {
			if byDenom, found := prepass.CrossPoolSnapshots[subaccountID]; found {
				snapshot = byDenom[quoteDenom]
			}
		}

		if snapshot == nil {
			s, _, err := e.buildCrossPoolSnapshot(ctx, subaccountID, quoteDenom, market.GetQuoteDecimals())
			if err != nil {
				ctx.Logger().Debug("DecrementLastLookOLR: failed to build snapshot, skipping decrement",
					"subaccount", subaccountID.Hex(),
					"quote_denom", quoteDenom,
					"error", err.Error(),
				)
				return
			}
			snapshot = s
		}

		pool, _ = GetOrCreateLastLookPoolState(ctx, subaccountID, quoteDenom, snapshot)
		if pool == nil {
			return
		}
	}

	isBuy, _, px, ok := ExtractOrderParams(order)
	if !ok || !remainingQty.IsPositive() {
		return
	}

	atomicMul := math.LegacyOneDec()
	if order.IsAtomic() {
		atomicMul = e.cross.AtomicMarketOrderFeeMultiplier(ctx, market.MarketID(), market.GetMarketType())
	}
	feeRateWorst := WorstCaseFeeRate(market, atomicMul)
	DecrementLastLookOLROnPool(ctx, e.cross, pool, subaccountID, market, markPrice, isBuy, remainingQty, px, feeRateWorst)
}

// CheckCrossMarginEmergencyPause checks if the subaccount is in cross-margin mode and if
// emergency pause is enabled. Returns an error if orders should be blocked.
// This should be called early in the order validation process, BEFORE the IsVanilla() check,
// to ensure that reduce-only orders are also blocked during emergency pause.
func (e *Engine) CheckCrossMarginEmergencyPause(ctx sdk.Context, subaccountID common.Hash) error {
	// Guard against nil engine or missing dependencies during tests/partial init.
	if e == nil || e.profiles == nil || e.params == nil {
		return nil
	}
	defer e.Meter(ctx).FuncTiming(&ctx, "Engine.CheckCrossMarginEmergencyPause")()

	profile, _ := e.EffectiveProfile(ctx, subaccountID)
	if profile == nil || profile.Mode != v2.RiskMode_RISK_MODE_CROSS {
		return nil // Not in cross-margin mode, no emergency pause check needed.
	}

	if e.params.GetParams(ctx).CrossMarginParams.EmergencyPaused {
		return errors.Wrap(exchangetypes.ErrFeatureDisabled, "cross margin is in emergency pause mode")
	}

	return nil
}

type Model interface {
	ReserveSpotLimitOrder(
		ctx sdk.Context, funds Funds, subaccountID common.Hash, order *v2.SpotOrder, market *v2.SpotMarket,
	) error
	ReserveSpotMarketOrder(
		ctx sdk.Context, funds Funds, subaccountID common.Hash, order *v2.SpotOrder,
		market *v2.SpotMarket, feeRate, bestPrice math.LegacyDec,
	) (balanceHold math.LegacyDec, err error)
	RefundSpotLimitOrderCancel(
		ctx sdk.Context, funds Funds, subaccountID common.Hash, order *v2.SpotLimitOrder,
		market *v2.SpotMarket, isTransient bool,
	) error

	ReserveDerivativeOrderMargin(
		ctx sdk.Context, funds Funds, subaccountID common.Hash, order *v2.DerivativeOrder,
		market v2.DerivativeMarketI, markPriceToCheck, tradeFeeRate math.LegacyDec,
	) (marginHold math.LegacyDec, err error)
	RefundDerivativeLimitOrderCancel(
		ctx sdk.Context, funds Funds, order *v2.DerivativeLimitOrder, market v2.MarketI, isTransient bool,
	) error
	RefundDerivativeMarketOrderCancel(
		ctx sdk.Context, funds Funds, subaccountID common.Hash, market v2.MarketI, refundAmount math.LegacyDec,
	)
	ShouldSkipDerivativeOrderForMarginRequirement(
		ctx sdk.Context, subaccountID common.Hash, order DerivativeInitialMarginChecker,
		market v2.DerivativeMarketI, markPrice math.LegacyDec, remainingQty math.LegacyDec,
	) (bool, error)
	CheckValidPositionToReduce(
		ctx sdk.Context, subaccountID common.Hash, position *v2.Position,
		marketType exchangetypes.MarketType, orderPrice math.LegacyDec, isBuy bool,
		tradeFeeRate math.LegacyDec, funding *v2.PerpetualMarketFunding, closeExecutionMargin math.LegacyDec,
	) error
	ValidateDerivativePositionMarginDecrease(
		ctx sdk.Context, subaccountID common.Hash, position *v2.Position,
		market *v2.DerivativeMarket, markPrice math.LegacyDec,
	) error
	// Note: For cross-margin, liquidation eligibility is determined at pool level
	// (Equity_liquidation < MM_positions), but the API remains position-scoped for
	// compatibility with existing MsgLiquidatePosition. The cross-margin model checks
	// pool-level eligibility when this method is called. See spec section
	// "Known limitation: Asymmetric eligibility vs targeting" for details.
	DerivativePositionLiquidationCheck(
		ctx sdk.Context, subaccountID common.Hash, position *v2.Position, market *v2.DerivativeMarket,
		markPrice math.LegacyDec, funding *v2.PerpetualMarketFunding,
	) (liquidationPrice math.LegacyDec, shouldLiquidate bool, err error)
}

// model returns the appropriate margin model for a subaccount based on its risk profile.
//
// NOTE: Defensive nil checks provide graceful degradation to isolated margin if deps aren't
// fully wired (e.g., during tests or partial initialisation). For production, call EnsureWired()
// during keeper startup to fail fast if wiring is incomplete.
func (e *Engine) model(ctx sdk.Context, subaccountID common.Hash) (Model, error) {
	if e == nil || e.profiles == nil {
		return nil, exchangetypes.ErrInvalidState.Wrap("risk engine not wired: call EnsureWired() during keeper startup")
	}
	defer e.Meter(ctx).FuncTiming(&ctx, "Engine.model")()

	profile, _ := e.EffectiveProfile(ctx, subaccountID)
	if profile == nil {
		return isolatedModelFactory(e), nil
	}

	switch profile.Mode {
	case v2.RiskMode_RISK_MODE_ISOLATED:
		return isolatedModelFactory(e), nil
	case v2.RiskMode_RISK_MODE_CROSS:
		return crossModelFactory(e), nil
	case v2.RiskMode_RISK_MODE_PORTFOLIO:
		return nil, exchangetypes.ErrFeatureDisabled.Wrap("portfolio-margin model not implemented yet")
	default:
		return nil, exchangetypes.ErrFeatureDisabled.Wrap("unknown risk mode")
	}
}

func (e *Engine) ReserveSpotLimitOrder(ctx sdk.Context, funds Funds, subaccountID common.Hash, order *v2.SpotOrder, market *v2.SpotMarket) error {
	defer e.Meter(ctx).FuncTiming(&ctx, "Engine.ReserveSpotLimitOrder")()
	m, err := e.model(ctx, subaccountID)
	if err != nil {
		return err
	}
	return m.ReserveSpotLimitOrder(ctx, funds, subaccountID, order, market)
}

func (e *Engine) ReserveSpotMarketOrder(
	ctx sdk.Context, funds Funds, subaccountID common.Hash, order *v2.SpotOrder,
	market *v2.SpotMarket, feeRate, bestPrice math.LegacyDec,
) (math.LegacyDec, error) {
	defer e.Meter(ctx).FuncTiming(&ctx, "Engine.ReserveSpotMarketOrder")()
	m, err := e.model(ctx, subaccountID)
	if err != nil {
		return math.LegacyZeroDec(), err
	}
	return m.ReserveSpotMarketOrder(ctx, funds, subaccountID, order, market, feeRate, bestPrice)
}

func (e *Engine) RefundSpotLimitOrderCancel(ctx sdk.Context, funds Funds, subaccountID common.Hash, order *v2.SpotLimitOrder, market *v2.SpotMarket, isTransient bool) error {
	defer e.Meter(ctx).FuncTiming(&ctx, "Engine.RefundSpotLimitOrderCancel")()
	m, err := e.model(ctx, subaccountID)
	if err != nil {
		return err
	}
	return m.RefundSpotLimitOrderCancel(ctx, funds, subaccountID, order, market, isTransient)
}

func (e *Engine) ReserveDerivativeOrderMargin(
	ctx sdk.Context, funds Funds, subaccountID common.Hash, order *v2.DerivativeOrder,
	market v2.DerivativeMarketI, markPriceToCheck, tradeFeeRate math.LegacyDec,
) (math.LegacyDec, error) {
	defer e.Meter(ctx).FuncTiming(&ctx, "Engine.ReserveDerivativeOrderMargin")()
	m, err := e.model(ctx, subaccountID)
	if err != nil {
		return math.LegacyZeroDec(), err
	}
	return m.ReserveDerivativeOrderMargin(ctx, funds, subaccountID, order, market, markPriceToCheck, tradeFeeRate)
}

func (e *Engine) RefundDerivativeLimitOrderCancel(ctx sdk.Context, funds Funds, order *v2.DerivativeLimitOrder, market v2.MarketI, isTransient bool) error {
	defer e.Meter(ctx).FuncTiming(&ctx, "Engine.RefundDerivativeLimitOrderCancel")()
	m, err := e.model(ctx, order.SubaccountID())
	if err != nil {
		return err
	}
	return m.RefundDerivativeLimitOrderCancel(ctx, funds, order, market, isTransient)
}

func (e *Engine) RefundDerivativeMarketOrderCancel(
	ctx sdk.Context, funds Funds, subaccountID common.Hash, market v2.MarketI, refundAmount math.LegacyDec,
) {
	defer e.Meter(ctx).FuncTiming(&ctx, "Engine.RefundDerivativeMarketOrderCancel")()
	m, err := e.model(ctx, subaccountID)
	if err != nil {
		return
	}
	m.RefundDerivativeMarketOrderCancel(ctx, funds, subaccountID, market, refundAmount)
}

func (e *Engine) ShouldSkipDerivativeOrderForMarginRequirement(
	ctx sdk.Context, subaccountID common.Hash, order DerivativeInitialMarginChecker,
	market v2.DerivativeMarketI, markPrice math.LegacyDec, remainingQty math.LegacyDec,
) (bool, error) {
	defer e.Meter(ctx).FuncTiming(&ctx, "Engine.ShouldSkipDerivativeOrderForMarginRequirement")()
	m, err := e.model(ctx, subaccountID)
	if err != nil {
		return true, err
	}
	return m.ShouldSkipDerivativeOrderForMarginRequirement(ctx, subaccountID, order, market, markPrice, remainingQty)
}

//nolint:revive // argument-limit: function signature matches Model interface
func (e *Engine) CheckValidPositionToReduce(
	ctx sdk.Context, subaccountID common.Hash, position *v2.Position,
	marketType exchangetypes.MarketType, orderPrice math.LegacyDec, isBuy bool,
	tradeFeeRate math.LegacyDec, funding *v2.PerpetualMarketFunding, closeExecutionMargin math.LegacyDec,
) error {
	defer e.Meter(ctx).FuncTiming(&ctx, "Engine.CheckValidPositionToReduce")()
	m, err := e.model(ctx, subaccountID)
	if err != nil {
		return err
	}
	return m.CheckValidPositionToReduce(ctx, subaccountID, position, marketType, orderPrice, isBuy, tradeFeeRate, funding, closeExecutionMargin)
}

func (e *Engine) ValidateDerivativePositionMarginDecrease(
	ctx sdk.Context, subaccountID common.Hash, position *v2.Position,
	market *v2.DerivativeMarket, markPrice math.LegacyDec,
) error {
	defer e.Meter(ctx).FuncTiming(&ctx, "Engine.ValidateDerivativePositionMarginDecrease")()
	m, err := e.model(ctx, subaccountID)
	if err != nil {
		return err
	}
	return m.ValidateDerivativePositionMarginDecrease(ctx, subaccountID, position, market, markPrice)
}

func (e *Engine) DerivativePositionLiquidationCheck(
	ctx sdk.Context, subaccountID common.Hash, position *v2.Position, market *v2.DerivativeMarket,
	markPrice math.LegacyDec, funding *v2.PerpetualMarketFunding,
) (math.LegacyDec, bool, error) {
	defer e.Meter(ctx).FuncTiming(&ctx, "Engine.DerivativePositionLiquidationCheck")()
	m, err := e.model(ctx, subaccountID)
	if err != nil {
		return math.LegacyZeroDec(), false, err
	}
	return m.DerivativePositionLiquidationCheck(ctx, subaccountID, position, market, markPrice, funding)
}
