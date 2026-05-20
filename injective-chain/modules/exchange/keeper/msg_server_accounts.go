package keeper

import (
	"context"
	"slices"

	sdkerrors "cosmossdk.io/errors"
	"cosmossdk.io/math"
	storetypes "cosmossdk.io/store/types"
	"github.com/cosmos/cosmos-sdk/telemetry"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"

	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/keeper/base"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/risk"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/types"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/types/v2"
)

type AccountsMsgServer struct {
	*Keeper
}

// AccountsMsgServerImpl returns an implementation of the bank MsgServer interface for the provided Keeper for account functions.
func AccountsMsgServerImpl(keeper *Keeper) AccountsMsgServer {
	return AccountsMsgServer{
		Keeper: keeper,
	}
}

func (k AccountsMsgServer) Deposit(
	c context.Context,
	msg *v2.MsgDeposit,
) (*v2.MsgDepositResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer k.Meter(ctx).FuncTiming(&ctx, "Deposit")()

	if k.IsFixedGasEnabled() {
		ctx.GasMeter().ConsumeGas(DetermineGas(msg), "MsgDeposit")
		ctx = ctx.WithGasMeter(storetypes.NewInfiniteGasMeter())
	}

	if err := k.ExecuteDeposit(ctx, msg); err != nil {
		return nil, err
	}

	// Evict cross-pool snapshot cache: deposit changes equity.
	depositSubaccountID := types.MustGetSubaccountIDOrDeriveFromNonce(sdk.MustAccAddressFromBech32(msg.Sender), msg.SubaccountId)
	k.RiskEngine().EvictCrossPoolSnapshotCache(ctx, depositSubaccountID)

	return &v2.MsgDepositResponse{}, nil
}

func (k AccountsMsgServer) Withdraw(
	c context.Context,
	msg *v2.MsgWithdraw,
) (*v2.MsgWithdrawResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer k.Meter(ctx).FuncTiming(&ctx, "Withdraw")()

	if k.IsFixedGasEnabled() {
		ctx.GasMeter().ConsumeGas(DetermineGas(msg), "MsgWithdraw")
		ctx = ctx.WithGasMeter(storetypes.NewInfiniteGasMeter())
	}

	withdrawAddr := sdk.MustAccAddressFromBech32(msg.Sender)
	subaccountID := types.MustGetSubaccountIDOrDeriveFromNonce(withdrawAddr, msg.SubaccountId)
	denom := msg.Amount.Denom
	amount := msg.Amount.Amount.ToLegacyDec()

	// Cross margin: collateral-decreasing actions must preserve pool maintenance.
	// Bank balance is excluded from cross-margin QuoteBalance, so withdrawals reduce pool
	// equity for all subaccount types (including default, where funds move to bank).
	if err := k.ensureCrossMarginMaintenanceAfterCollateralDecrease(ctx, subaccountID, denom, amount); err != nil {
		return nil, err
	}

	if err := k.ExecuteWithdraw(ctx, msg); err != nil {
		return nil, err
	}

	// Evict cross-pool snapshot cache: withdrawal changes equity.
	k.RiskEngine().EvictCrossPoolSnapshotCache(ctx, subaccountID)

	return &v2.MsgWithdrawResponse{}, nil
}

func (k AccountsMsgServer) UpdateSubaccountRiskProfile(
	goCtx context.Context,
	msg *v2.MsgUpdateSubaccountRiskProfile,
) (*v2.MsgUpdateSubaccountRiskProfileResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	defer k.Meter(ctx).FuncTiming(&ctx, "UpdateSubaccountRiskProfile")()

	// In fixed-gas mode, save a reference to the real gas meter so we can charge per-order
	// gas after the isolated→cross hold release (which iterates all orders). The base gas
	// is consumed up front; per-order gas is added after the switch logic completes.
	var (
		fixedGasEnabled = k.IsFixedGasEnabled()
		realGasMeter    storetypes.GasMeter
	)
	if fixedGasEnabled {
		realGasMeter = ctx.GasMeter()
		realGasMeter.ConsumeGas(MsgUpdateSubaccountRiskProfileGas, "MsgUpdateSubaccountRiskProfile")
		ctx = ctx.WithGasMeter(storetypes.NewInfiniteGasMeter())
	}

	if err := msg.ValidateBasic(); err != nil {
		return nil, err
	}

	sender := sdk.MustAccAddressFromBech32(msg.Sender)
	subaccountID, err := types.GetSubaccountIDOrDeriveFromNonce(sender, msg.SubaccountId)
	if err != nil {
		return nil, err
	}

	// Default subaccounts (nonce 0) cannot switch to cross-margin mode.
	// SetDepositOrSendToBank auto-sweeps integer balances to bank for default subaccounts,
	// but cross-margin QuoteBalance intentionally excludes bank balances. Execution gains
	// would be swept out of the pool, making subsequent admission/maintenance checks
	// understate collateral.
	if types.IsDefaultSubaccountID(subaccountID) && msg.RiskProfile.Mode == v2.RiskMode_RISK_MODE_CROSS {
		return nil, sdkerrors.Wrap(types.ErrFeatureDisabled, "default subaccounts cannot switch to cross-margin mode")
	}

	current, _ := k.RiskEngine().EffectiveProfile(ctx, subaccountID)
	current, _ = base.NormalizeRiskProfile(current)

	target, _ := base.NormalizeRiskProfile(&msg.RiskProfile)

	releasedOrderCount, err := k.validateAndApplyModeSwitch(ctx, current, target, subaccountID)

	// Charge per-order gas for hold-release work even on failure — the cache context is
	// discarded but the compute was real. Must happen before the error return.
	if fixedGasEnabled && releasedOrderCount > 0 {
		realGasMeter.ConsumeGas(PerOrderHoldReleaseGas*releasedOrderCount, "per-order hold release")
	}

	if err != nil {
		return nil, err
	}

	if err := k.SetSubaccountRiskProfile(ctx, subaccountID, target); err != nil {
		return nil, err
	}

	// Profile change invalidates any cached cross-pool snapshot for this subaccount.
	k.RiskEngine().EvictCrossPoolSnapshotCache(ctx, subaccountID)

	newProfile, isDefault := k.RiskEngine().EffectiveProfile(ctx, subaccountID)
	newProfile, _ = base.NormalizeRiskProfile(newProfile)

	k.EmitEvent(ctx, &v2.EventSubaccountRiskProfileUpdated{
		SubaccountId:    subaccountID.Hex(),
		PreviousProfile: *current,
		NewProfile:      *newProfile,
		IsDefault:       isDefault,
	})

	return &v2.MsgUpdateSubaccountRiskProfileResponse{}, nil
}

// validateAndApplyModeSwitch validates the mode transition and applies side effects
// (e.g. releasing isolated holds for ISOLATED→CROSS, checking no positions for CROSS→ISOLATED).
func (k AccountsMsgServer) validateAndApplyModeSwitch(
	ctx sdk.Context,
	current, target *v2.SubaccountRiskProfile,
	subaccountID common.Hash,
) (releasedOrderCount uint64, err error) {
	params := k.GetParams(ctx)
	// During emergency pause, block new cross-margin activity (ISOLATED→CROSS) but allow
	// opting out (CROSS→ISOLATED). Blocking opt-out would trap users in cross-margin mode
	// indefinitely during a pause.
	if params.CrossMarginParams.EmergencyPaused && target.Mode == v2.RiskMode_RISK_MODE_CROSS {
		return 0, sdkerrors.Wrap(types.ErrFeatureDisabled, "cross margin is in emergency pause mode")
	}

	// Conservative switching rules:
	// - Isolated -> Cross: only allowed if all existing derivative positions + orders are eligible and their quote denoms are enabled.
	//   Also reject if any involved quote-denom pool would be inadmissible immediately after switching.
	// - Cross -> Isolated: only allowed if no derivative positions and no derivative orders.
	if current.Mode == v2.RiskMode_RISK_MODE_ISOLATED && target.Mode == v2.RiskMode_RISK_MODE_CROSS {
		return k.handleIsolatedToCrossSwitch(ctx, subaccountID)
	}

	if current.Mode == v2.RiskMode_RISK_MODE_CROSS && target.Mode == v2.RiskMode_RISK_MODE_ISOLATED {
		return 0, k.validateCrossToIsolatedSwitch(ctx, subaccountID)
	}

	return 0, nil
}

// handleIsolatedToCrossSwitch validates eligibility, releases isolated per-order holds,
// and checks cross-pool admission for an ISOLATED→CROSS mode switch.
//
func (k AccountsMsgServer) handleIsolatedToCrossSwitch(
	ctx sdk.Context,
	subaccountID common.Hash,
) (releasedOrderCount uint64, err error) {
	// First, get the marketIDs and validate basic eligibility (denoms enabled, market types, etc.)
	// This does NOT perform the admission check yet.
	marketIDs, err := k.getMarketIDsAndValidateEligibilityForCrossSwitch(ctx, subaccountID)
	if err != nil {
		return 0, err
	}

	// Use a cache context to release isolated per-order holds and validate admission.
	// If admission fails, the cache is discarded and state remains unchanged.
	// This prevents a scenario where holds are released but the switch fails,
	// leaving the subaccount with extra available balance (enabling double-refund on cancel).
	cacheCtx, commit := ctx.CacheContext()

	// Release existing isolated per-order holds for vanilla derivative orders BEFORE the admission check.
	// This ensures equity is computed with the released funds available.
	// Cross mode uses pool-level order locking, so these per-order holds are no longer needed.
	releasedOrderCount = k.releaseIsolatedDerivativeOrderHoldsForCrossSwitch(cacheCtx, subaccountID, marketIDs)

	// Evict any inherited cross-pool snapshot cache so the admission check below uses
	// post-refund equity (the hold release changed deposits in cacheCtx).
	k.RiskEngine().EvictCrossPoolSnapshotCache(cacheCtx, subaccountID)

	// Now validate that the cross-margin pools are admissible with the released funds.
	if err := k.validateCrossPoolAdmissionAfterSwitch(cacheCtx, subaccountID, marketIDs); err != nil {
		// Admission failed: discard the cache, holds remain locked, account stays isolated.
		// Return releasedOrderCount so the caller can still charge for the hold-release work
		// that was performed in the cache context (even though it's discarded).
		return releasedOrderCount, err
	}

	// Admission passed: commit the released holds.
	commit()
	return releasedOrderCount, nil
}

// getMarketIDsAndValidateEligibilityForCrossSwitch checks that all existing derivative positions and orders
// are eligible for cross-margin (denoms enabled, market types allowed, max markets not exceeded).
// It does NOT check admission (equity >= OLR) - that is deferred to validateCrossPoolAdmissionAfterSwitch
// so that isolated holds can be released first.
//
func (k AccountsMsgServer) getMarketIDsAndValidateEligibilityForCrossSwitch(
	ctx sdk.Context,
	subaccountID common.Hash,
) (marketIDs []common.Hash, err error) {
	defer k.Meter(ctx).FuncTiming(&ctx, "getMarketIDsAndValidateEligibilityForCrossSwitch")()

	params := k.GetParams(ctx)

	maxActiveMarkets := params.CrossMarginParams.MaxActiveDerivativeMarketsPerPool
	if maxActiveMarkets == 0 {
		maxActiveMarkets = 100
	}

	enabledDenoms := make(map[string]struct{}, len(params.CrossMarginParams.EnabledQuoteDenoms))
	for _, denom := range params.CrossMarginParams.EnabledQuoteDenoms {
		enabledDenoms[denom] = struct{}{}
	}
	if len(enabledDenoms) == 0 {
		return nil, sdkerrors.Wrap(types.ErrFeatureDisabled, "cross margin is disabled")
	}

	marketIDs = k.getDerivativeActivityMarketIDs(ctx, subaccountID)

	activeMarketsByDenom := make(map[string]uint32, 4)

	for _, marketID := range marketIDs {
		if err := k.validateMarketEligibilityForCross(ctx, marketID, &params.CrossMarginParams, enabledDenoms, activeMarketsByDenom, maxActiveMarkets); err != nil {
			return nil, err
		}
	}

	return marketIDs, nil
}

func (k AccountsMsgServer) validateMarketEligibilityForCross(
	ctx sdk.Context,
	marketID common.Hash,
	crossParams *v2.CrossMarginParams,
	enabledDenoms map[string]struct{},
	activeMarketsByDenom map[string]uint32,
	maxActiveMarkets uint32,
) error {
	// Use GetDerivativeMarketByID to check both enabled and disabled markets.
	// Positions/orders may exist in disabled markets and must be accounted for.
	market := k.GetDerivativeMarketByID(ctx, marketID)
	if market == nil {
		if k.GetBinaryOptionsMarketByID(ctx, marketID) != nil {
			return sdkerrors.Wrap(types.ErrFeatureDisabled, "binary options are isolated-only")
		}
		return nil
	}

	if err := checkMarketCrossEligibility(market, crossParams, enabledDenoms); err != nil {
		return err
	}

	activeMarketsByDenom[market.QuoteDenom]++
	if activeMarketsByDenom[market.QuoteDenom] > maxActiveMarkets {
		return sdkerrors.Wrapf(
			types.ErrFeatureDisabled,
			"cross margin exceeds max active derivative markets per pool for quote denom %s: %d",
			market.QuoteDenom,
			maxActiveMarkets,
		)
	}

	return nil
}

func checkMarketCrossEligibility(market *v2.DerivativeMarket, crossParams *v2.CrossMarginParams, enabledDenoms map[string]struct{}) error {
	marketType := market.GetMarketType()
	if marketType.IsBinaryOptions() {
		return sdkerrors.Wrap(types.ErrFeatureDisabled, "binary options are isolated-only")
	}

	if marketType.IsPerpetual() && !crossParams.PerpetualEnabled {
		return sdkerrors.Wrap(types.ErrFeatureDisabled, "cross margin is disabled for perpetual markets")
	}
	if marketType == types.MarketType_Expiry && !crossParams.ExpiryEnabled {
		return sdkerrors.Wrap(types.ErrFeatureDisabled, "cross margin is disabled for expiry markets")
	}

	if !market.IsCrossMarginEligible() {
		return sdkerrors.Wrap(types.ErrFeatureDisabled, "cross margin is disabled for this market")
	}

	if _, ok := enabledDenoms[market.QuoteDenom]; !ok {
		return sdkerrors.Wrapf(types.ErrFeatureDisabled, "cross margin is disabled for quote denom %s", market.QuoteDenom)
	}

	return nil
}

// validateCrossPoolAdmissionAfterSwitch checks that all cross-margin pools for the subaccount
// are admissible (equity >= OLR) after switching from isolated to cross mode.
// This should be called AFTER isolated holds have been released.
//
func (k AccountsMsgServer) validateCrossPoolAdmissionAfterSwitch(
	ctx sdk.Context,
	subaccountID common.Hash,
	marketIDs []common.Hash,
) error {
	defer k.Meter(ctx).FuncTiming(&ctx, "validateCrossPoolAdmissionAfterSwitch")()

	quoteDecimalsByDenom := make(map[string]uint32, 4)

	for _, marketID := range marketIDs {
		// Use GetDerivativeMarketByID to check both enabled and disabled markets.
		// Positions/orders may exist in disabled markets and must be accounted for in admission checks.
		market := k.GetDerivativeMarketByID(ctx, marketID)
		if market == nil {
			continue
		}

		if existing, ok := quoteDecimalsByDenom[market.QuoteDenom]; !ok {
			quoteDecimalsByDenom[market.QuoteDenom] = market.QuoteDecimals
		} else if market.QuoteDecimals != existing {
			return sdkerrors.Wrapf(
				types.ErrInvalidQuoteDenom,
				"inconsistent quote decimals in cross-pool: market %s has %d, expected %d (quote denom %s)",
				marketID.Hex(),
				market.QuoteDecimals,
				existing,
				market.QuoteDenom,
			)
		}
	}

	if len(quoteDecimalsByDenom) == 0 {
		// Empty subaccount: no admission check needed.
		return nil
	}

	// Deterministic iteration order.
	denoms := make([]string, 0, len(quoteDecimalsByDenom))
	for denom := range quoteDecimalsByDenom {
		denoms = append(denoms, denom)
	}
	slices.Sort(denoms)

	for _, denom := range denoms {
		if err := k.checkDenomPoolAdmission(ctx, subaccountID, denom, quoteDecimalsByDenom[denom]); err != nil {
			return err
		}
	}

	return nil
}

func (k AccountsMsgServer) checkDenomPoolAdmission(
	ctx sdk.Context,
	subaccountID common.Hash,
	denom string,
	quoteDecimals uint32,
) error {
	snapshot, err := k.RiskEngine().BuildCrossPoolSnapshot(ctx, subaccountID, denom, quoteDecimals)
	if err != nil {
		return err
	}

	// Reject if the pool would be immediately liquidatable.
	if snapshot.MaintenanceMarginTotal.IsPositive() && snapshot.EquityLiquidation.LT(snapshot.MaintenanceMarginTotal) {
		return sdkerrors.Wrapf(
			types.ErrInsufficientMargin,
			"cross-margin opt-in would be immediately liquidatable for quote denom %s: equity_liquidation %s < maintenance_margin %s",
			denom,
			snapshot.EquityLiquidation.String(),
			snapshot.MaintenanceMarginTotal.String(),
		)
	}

	if snapshot.EquityAdmission.LT(snapshot.OrderLockRequirement) {
		return sdkerrors.Wrapf(
			types.ErrInsufficientMargin,
			"cross-margin opt-in inadmissible for quote denom %s: equity_admission %s < order_lock_requirement %s",
			denom,
			snapshot.EquityAdmission.String(),
			snapshot.OrderLockRequirement.String(),
		)
	}

	return nil
}

func (k AccountsMsgServer) validateCrossToIsolatedSwitch(ctx sdk.Context, subaccountID common.Hash) error {
	defer k.Meter(ctx).FuncTiming(&ctx, "validateCrossToIsolatedSwitch")()

	if len(k.GetActiveDerivativeMarketsBySubaccount(ctx, subaccountID)) > 0 {
		return sdkerrors.Wrap(types.ErrFeatureDisabled, "cross->isolated is only allowed when there are no derivative positions")
	}
	if len(k.GetActiveDerivativeOrderMarketsBySubaccount(ctx, subaccountID)) > 0 {
		return sdkerrors.Wrap(types.ErrFeatureDisabled, "cross->isolated is only allowed when there are no derivative orders")
	}
	// Check for live transient derivative orders (limit + market). Indicators are not reliable
	// here because they persist for the whole block even after cancel/fill; instead we probe
	// candidate markets from indicators and verify actual orders exist.
	if len(k.getLiveTransientDerivativeOrderMarketsBySubaccount(ctx, subaccountID)) > 0 {
		return sdkerrors.Wrap(types.ErrFeatureDisabled, "cross->isolated is only allowed when there are no derivative orders")
	}
	return nil
}

func (k AccountsMsgServer) getDerivativeActivityMarketIDs(ctx sdk.Context, subaccountID common.Hash) []common.Hash {
	defer k.Meter(ctx).FuncTiming(&ctx, "getDerivativeActivityMarketIDs")()

	// Include live transient derivative order markets (not raw indicators, which persist after
	// cancel/fill). This ensures isolated→cross migration releases holds and validates
	// exposure for all markets with actual transient orders.
	return risk.MergeAndSortMarketIDs(
		k.GetActiveDerivativeMarketsBySubaccount(ctx, subaccountID),
		k.GetActiveDerivativeOrderMarketsBySubaccount(ctx, subaccountID),
		k.getLiveTransientDerivativeOrderMarketsBySubaccount(ctx, subaccountID),
	)
}

// getLiveTransientDerivativeOrderMarketsBySubaccount returns markets where the subaccount
// has actual live transient derivative orders (limit or market). It uses the transient
// order indicators as candidate markets, then verifies at least one live order exists in
// each candidate. This avoids false positives from stale indicators that persist after
// cancel/fill within the block.
func (k AccountsMsgServer) getLiveTransientDerivativeOrderMarketsBySubaccount(
	ctx sdk.Context,
	subaccountID common.Hash,
) []common.Hash {
	defer k.Meter(ctx).FuncTiming(&ctx, "getLiveTransientDerivativeOrderMarketsBySubaccount")()

	candidates := k.GetTransientDerivativeOrderIndicatorMarketsBySubaccount(ctx, subaccountID)
	if len(candidates) == 0 {
		return nil
	}

	live := make([]common.Hash, 0, len(candidates))
	for _, marketID := range candidates {
		if k.hasLiveTransientDerivativeOrder(ctx, marketID, subaccountID) {
			live = append(live, marketID)
		}
	}
	return live
}

// hasLiveTransientDerivativeOrder checks whether the subaccount has at least one live
// transient derivative order (limit or market, buy or sell) in the given market.
func (k AccountsMsgServer) hasLiveTransientDerivativeOrder(
	ctx sdk.Context,
	marketID, subaccountID common.Hash,
) bool {
	defer k.Meter(ctx).FuncTiming(&ctx, "hasLiveTransientDerivativeOrder")()

	found := false

	for _, isBuy := range []bool{true, false} {
		if found {
			break
		}
		k.IterateTransientDerivativeLimitOrdersBySubaccount(ctx, marketID, isBuy, subaccountID, func(_ *v2.DerivativeLimitOrder) (stop bool) {
			found = true
			return true
		})
	}

	for _, isBuy := range []bool{true, false} {
		if found {
			break
		}
		found = k.HasTransientDerivativeMarketOrderForSubaccount(ctx, marketID, subaccountID, isBuy)
	}

	return found
}

// releaseIsolatedDerivativeOrderHoldsForCrossSwitch refunds the per-order margin holds
// that were charged under isolated margin mode. Cross margin uses pool-level order locking
// instead of per-order holds, so these must be released when switching modes.
//
// This function handles ALL vanilla derivative order types:
//   - Resting limit orders (across blocks): uses maker fee rate
//   - Transient limit orders (this block): uses taker fee rate
//   - Transient market orders (this block): uses stored MarginHold
//   - Conditional limit orders: uses taker fee rate
//   - Conditional market orders: uses stored MarginHold
//
// Non-vanilla orders (reduce-only, close-only) are not charged per-order holds under isolated
// margin and are therefore skipped.
//
// NOTE: This function does not emit events for balance refunds, consistent with other internal
// balance adjustment paths. Order lifecycle events (cancel, fill) are emitted elsewhere.
//
func (k AccountsMsgServer) releaseIsolatedDerivativeOrderHoldsForCrossSwitch(
	ctx sdk.Context,
	subaccountID common.Hash,
	marketIDs []common.Hash,
) (orderCount uint64) {
	defer k.Meter(ctx).FuncTiming(&ctx, "releaseIsolatedDerivativeOrderHoldsForCrossSwitch")()

	// Accumulate refunds per quote denom rather than applying them one-by-one.
	// This avoids using IncrementAvailableBalanceOrBank which calls SetDepositOrSendToBank,
	// auto-sweeping integer balances to bank for default subaccounts. Cross-margin equity
	// calculations intentionally exclude bank balances, so swept funds would become invisible
	// to the pool, causing valid switches to fail or reducing post-switch pool equity.
	refundsByDenom := make(map[string]math.LegacyDec)

	addRefund := func(denom string, chainAmount math.LegacyDec) {
		if !chainAmount.IsPositive() {
			return
		}
		if existing, ok := refundsByDenom[denom]; ok {
			refundsByDenom[denom] = existing.Add(chainAmount)
		} else {
			refundsByDenom[denom] = chainAmount
		}
	}

	for _, marketID := range marketIDs {
		// Use GetDerivativeMarketByID to include both enabled and disabled markets.
		// Orders can exist in disabled/paused markets and their holds must be released.
		market := k.GetDerivativeMarketByID(ctx, marketID)
		if market == nil {
			continue
		}
		if market.GetMarketType().IsBinaryOptions() {
			continue
		}

		orderCount += k.releaseMarketOrderHolds(ctx, marketID, subaccountID, market, addRefund)
	}

	// Apply accumulated refunds directly to exchange deposits without bank sweeping.
	// Deterministic iteration order is required for store writes to ensure consensus.
	sortedDenoms := make([]string, 0, len(refundsByDenom))
	for denom := range refundsByDenom {
		sortedDenoms = append(sortedDenoms, denom)
	}
	slices.Sort(sortedDenoms)

	for _, denom := range sortedDenoms {
		deposit := k.GetDeposit(ctx, subaccountID, denom)
		deposit.AvailableBalance = deposit.AvailableBalance.Add(refundsByDenom[denom])
		k.SetDeposit(ctx, subaccountID, denom, deposit)
	}

	return orderCount
}

func (k AccountsMsgServer) releaseMarketOrderHolds(
	ctx sdk.Context,
	marketID, subaccountID common.Hash,
	market *v2.DerivativeMarket,
	addRefund func(string, math.LegacyDec),
) (orderCount uint64) {
	// Resting derivative limit orders (across blocks): holds are based on maker fees (worst-case held amount already adjusted on persistence).
	for _, isBuy := range []bool{true, false} {
		k.IterateDerivativeLimitOrdersBySubaccount(ctx, marketID, isBuy, subaccountID, func(order v2.DerivativeLimitOrder) (stop bool) {
			if !order.IsVanilla() {
				return false
			}
			orderCount++
			refund := order.GetCancelRefundAmount(market.GetMakerFeeRate())
			addRefund(market.QuoteDenom, market.NotionalToChainFormat(refund))
			return false
		})
	}

	// Transient derivative limit orders (this block): holds were charged using taker fees
	// for regular orders but maker fees for post-only orders (see EnsureValidDerivativeOrder).
	for _, isBuy := range []bool{true, false} {
		k.IterateTransientDerivativeLimitOrdersBySubaccount(ctx, marketID, isBuy, subaccountID, func(order *v2.DerivativeLimitOrder) (stop bool) {
			if order == nil || !order.IsVanilla() {
				return false
			}
			orderCount++
			feeRate := market.GetTakerFeeRate()
			if order.OrderType.IsPostOnly() {
				feeRate = market.GetMakerFeeRate()
			}
			refund := order.GetCancelRefundAmount(feeRate)
			addRefund(market.QuoteDenom, market.NotionalToChainFormat(refund))
			return false
		})
	}

	// Transient derivative market orders (this block): holds are stored explicitly as MarginHold.
	for _, isBuy := range []bool{true, false} {
		k.IterateDerivativeMarketOrdersBySubaccount(ctx, marketID, subaccountID, isBuy, func(order *v2.DerivativeMarketOrder) (stop bool) {
			if order == nil || !order.IsVanilla() {
				return false
			}
			orderCount++
			refund := order.GetCancelRefundAmount()
			addRefund(market.QuoteDenom, market.NotionalToChainFormat(refund))
			return false
		})
	}

	// Conditional derivative orders (across blocks).
	orderCount += k.releaseConditionalOrderHolds(ctx, marketID, subaccountID, market, addRefund)
	return orderCount
}

// releaseConditionalOrderHolds computes refunds for conditional derivative orders (limit + market)
// in a single market. Holds were charged using taker fees for regular orders but maker fees for
// post-only orders (mirrors EnsureValidDerivativeOrder placement logic).
func (k AccountsMsgServer) releaseConditionalOrderHolds(
	ctx sdk.Context,
	marketID, subaccountID common.Hash,
	market *v2.DerivativeMarket,
	addRefund func(string, math.LegacyDec),
) (orderCount uint64) {
	for _, isHigher := range []bool{true, false} {
		limitHashes := k.GetAllConditionalOrderHashesBySubaccountAndMarket(ctx, marketID, isHigher, false, subaccountID)
		for _, hash := range limitHashes {
			isHigherCopy := isHigher
			limitOrder, _ := k.GetConditionalDerivativeLimitOrderBySubaccountIDAndHash(ctx, marketID, &isHigherCopy, subaccountID, hash)
			if limitOrder == nil || !limitOrder.IsVanilla() {
				continue
			}
			orderCount++
			feeRate := market.GetTakerFeeRate()
			if limitOrder.OrderType.IsPostOnly() {
				feeRate = market.GetMakerFeeRate()
			}
			refund := limitOrder.GetCancelRefundAmount(feeRate)
			addRefund(market.QuoteDenom, market.NotionalToChainFormat(refund))
		}

		marketHashes := k.GetAllConditionalOrderHashesBySubaccountAndMarket(ctx, marketID, isHigher, true, subaccountID)
		for _, hash := range marketHashes {
			isHigherCopy := isHigher
			marketOrder, _ := k.GetConditionalDerivativeMarketOrderBySubaccountIDAndHash(ctx, marketID, &isHigherCopy, subaccountID, hash)
			if marketOrder == nil || !marketOrder.IsVanilla() {
				continue
			}
			orderCount++
			refund := marketOrder.GetCancelRefundAmount()
			addRefund(market.QuoteDenom, market.NotionalToChainFormat(refund))
		}
	}
	return orderCount
}

func (k AccountsMsgServer) SubaccountTransfer(
	c context.Context,
	msg *v2.MsgSubaccountTransfer,
) (*v2.MsgSubaccountTransferResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer k.Meter(ctx).FuncTiming(&ctx, "SubaccountTransfer")()

	var (
		denom           = msg.Amount.Denom
		amount          = msg.Amount.Amount.ToLegacyDec()
		sender          = sdk.MustAccAddressFromBech32(msg.Sender)
		srcSubaccountID = types.MustGetSubaccountIDOrDeriveFromNonce(sender, msg.SourceSubaccountId)
		dstSubaccountID = types.MustGetSubaccountIDOrDeriveFromNonce(sender, msg.DestinationSubaccountId)
	)

	if k.IsFixedGasEnabled() {
		ctx.GasMeter().ConsumeGas(DetermineGas(msg), "MsgSubaccountTransfer")
		ctx = ctx.WithGasMeter(storetypes.NewInfiniteGasMeter())
	}

	// Cross margin: collateral-decreasing actions must preserve pool maintenance.
	// Skip for self-transfers (src == dst): decrement + increment on the same subaccount is net-zero.
	if srcSubaccountID != dstSubaccountID {
		if err := k.ensureCrossMarginMaintenanceAfterCollateralDecrease(ctx, srcSubaccountID, denom, amount); err != nil {
			return nil, err
		}
	}

	if err := k.Keeper.DecrementDeposit(ctx, srcSubaccountID, denom, amount); err != nil {

		return nil, err
	}

	if err := k.Keeper.IncrementDepositForNonDefaultSubaccount(ctx, dstSubaccountID, denom, amount); err != nil {
		return nil, err
	}

	// Evict cross-pool snapshot cache: transfer changes equity for both subaccounts.
	k.RiskEngine().EvictCrossPoolSnapshotCache(ctx, srcSubaccountID)
	if srcSubaccountID != dstSubaccountID {
		k.RiskEngine().EvictCrossPoolSnapshotCache(ctx, dstSubaccountID)
	}

	k.EmitEvent(ctx, &v2.EventSubaccountBalanceTransfer{
		SrcSubaccountId: srcSubaccountID.Hex(),
		DstSubaccountId: dstSubaccountID.Hex(),
		Amount:          msg.Amount,
	})

	return &v2.MsgSubaccountTransferResponse{}, nil
}

func (k AccountsMsgServer) ExternalTransfer(
	c context.Context,
	msg *v2.MsgExternalTransfer,
) (*v2.MsgExternalTransferResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer k.Meter(ctx).FuncTiming(&ctx, "ExternalTransfer")()

	var (
		denom           = msg.Amount.Denom
		amount          = msg.Amount.Amount.ToLegacyDec()
		sender          = sdk.MustAccAddressFromBech32(msg.Sender)
		srcSubaccountID = types.MustGetSubaccountIDOrDeriveFromNonce(sender, msg.SourceSubaccountId)
		dstSubaccountID = common.HexToHash(msg.DestinationSubaccountId)
		recipientAddr   = types.SubaccountIDToSdkAddress(dstSubaccountID)
	)

	if k.IsFixedGasEnabled() {
		ctx.GasMeter().ConsumeGas(DetermineGas(msg), "MsgExternalTransfer")
		ctx = ctx.WithGasMeter(storetypes.NewInfiniteGasMeter())
	}

	// disable subaccount transfers from and to permissioned addresses
	if k.permissionsKeeper.IsEnforcedRestrictionsDenom(ctx, denom) {
		if _, err := k.permissionsKeeper.SendRestrictionFn(ctx, sender, recipientAddr, msg.Amount); err != nil {
			return nil, sdkerrors.Wrapf(err, "can't transfer deposit %s from %s to %s", msg.Amount, sender, recipientAddr)
		}
	}

	// Self-transfers (src == dst) are no-ops for exchange deposits: decrement + increment cancel out.
	// However, for default subaccounts, IncrementDepositOrSendToBank sweeps integer balances to
	// bank. Cross-margin equity intentionally excludes bank balances, so the sweep silently
	// reduces pool equity — bypassing maintenance/OLR checks regardless of the transfer amount.
	// Short-circuit self-transfers to avoid the sweep entirely.
	if srcSubaccountID == dstSubaccountID {
		// Validate that the balance exists (consistent with non-self DecrementDeposit behaviour).
		deposit := k.GetDeposit(ctx, srcSubaccountID, denom)
		if deposit.IsEmpty() || deposit.AvailableBalance.LT(amount) || deposit.TotalBalance.LT(amount) {
			return nil, types.ErrInsufficientDeposit
		}
		// No state change — decrement + increment is net-zero. Skip to event emission.
	} else {
		// Cross margin: collateral-decreasing actions must preserve pool maintenance.
		if err := k.ensureCrossMarginMaintenanceAfterCollateralDecrease(ctx, srcSubaccountID, denom, amount); err != nil {
			return nil, err
		}

		if err := k.DecrementDeposit(ctx, srcSubaccountID, denom, amount); err != nil {
			return nil, err
		}

		// create new account for recipient if it doesn't exist already
		if !k.AccountKeeper.HasAccount(ctx, recipientAddr) {
			defer telemetry.IncrCounter(1, "new", "account")
			k.AccountKeeper.SetAccount(ctx, k.AccountKeeper.NewAccountWithAddress(ctx, recipientAddr))
		}

		if types.IsDefaultSubaccountID(dstSubaccountID) {
			k.IncrementDepositOrSendToBank(ctx, dstSubaccountID, denom, amount)
		} else {
			if err := k.IncrementDepositForNonDefaultSubaccount(ctx, dstSubaccountID, denom, amount); err != nil {
				return nil, err
			}
		}
	}

	// Evict cross-pool snapshot cache: transfer changes equity for both subaccounts.
	k.RiskEngine().EvictCrossPoolSnapshotCache(ctx, srcSubaccountID)
	if srcSubaccountID != dstSubaccountID {
		k.RiskEngine().EvictCrossPoolSnapshotCache(ctx, dstSubaccountID)
	}

	k.EmitEvent(ctx, &v2.EventSubaccountBalanceTransfer{
		SrcSubaccountId: srcSubaccountID.Hex(),
		DstSubaccountId: dstSubaccountID.Hex(),
		Amount:          msg.Amount,
	})

	return &v2.MsgExternalTransferResponse{}, nil
}

func (k AccountsMsgServer) ensureCrossMarginMaintenanceAfterCollateralDecrease(
	ctx sdk.Context,
	subaccountID common.Hash,
	denom string,
	amountChain math.LegacyDec,
) error {
	defer k.Meter(ctx).FuncTiming(&ctx, "ensureCrossMarginMaintenanceAfterCollateralDecrease")()

	profile, _ := k.RiskEngine().EffectiveProfile(ctx, subaccountID)
	if profile == nil || profile.Mode != v2.RiskMode_RISK_MODE_CROSS {
		return nil
	}

	quoteDecimals, ok, err := k.getCrossPoolQuoteDecimalsForDenom(ctx, subaccountID, denom)
	if err != nil {
		return err
	}
	if !ok {
		// No active positions or orders in this denom pool, so both maintenance and order-lock requirements are zero.
		return nil
	}

	snapshot, err := k.RiskEngine().BuildCrossPoolSnapshot(ctx, subaccountID, denom, quoteDecimals)
	if err != nil {
		return err
	}

	// No positions and no orders: maintenance and order-lock thresholds are both zero,
	// so the pool has no risk exposure and the withdrawal is unconditionally safe.
	// This also avoids false rejections from the fees-buffer equity deduction when stale
	// transient order indicators cause this path to run for an otherwise empty pool.
	if !snapshot.MaintenanceMarginTotal.IsPositive() && !snapshot.OrderLockRequirement.IsPositive() {
		return nil
	}

	amountNotional := types.NotionalFromChainFormat(amountChain, quoteDecimals)

	equityLiqAfter := snapshot.EquityLiquidation.Sub(amountNotional)
	equityAdmAfter := snapshot.EquityAdmission.Sub(amountNotional)

	// Emergency pause: strip positive UPnL to enforce isolated-margin-level maintenance,
	// and skip OLR check since open orders cannot match during pause (blocked by
	// CheckCrossMarginEmergencyPause in both derivative and spot order matching).
	if k.GetParams(ctx).CrossMarginParams.EmergencyPaused {
		equityLiqAfter, _ = snapshot.StripPositiveUPnL(equityLiqAfter, equityAdmAfter)
		return checkMaintenanceAfterDecrease(snapshot, equityLiqAfter)
	}

	return checkMaintenanceAndAdmissionAfterDecrease(snapshot, equityLiqAfter, equityAdmAfter)
}

func checkMaintenanceAfterDecrease(snapshot *risk.CrossPoolSnapshot, equityLiqAfter math.LegacyDec) error {
	if equityLiqAfter.LT(snapshot.MaintenanceMarginTotal) {
		return sdkerrors.Wrapf(
			types.ErrInsufficientMargin,
			"cross-margin maintenance check failed after collateral decrease: equity_after %s < maintenance %s",
			equityLiqAfter.String(),
			snapshot.MaintenanceMarginTotal.String(),
		)
	}
	return nil
}

func checkMaintenanceAndAdmissionAfterDecrease(
	snapshot *risk.CrossPoolSnapshot,
	equityLiqAfter, equityAdmAfter math.LegacyDec,
) error {
	if err := checkMaintenanceAfterDecrease(snapshot, equityLiqAfter); err != nil {
		return err
	}

	// Collateral-decreasing actions must also not render existing open orders inadmissible.
	if equityAdmAfter.LT(snapshot.OrderLockRequirement) {
		return sdkerrors.Wrapf(
			types.ErrInsufficientMargin,
			"cross-margin admission check failed after collateral decrease: equity_admission_after %s < order_lock_requirement %s",
			equityAdmAfter.String(),
			snapshot.OrderLockRequirement.String(),
		)
	}

	return nil
}

func (k AccountsMsgServer) getCrossPoolQuoteDecimalsForDenom(
	ctx sdk.Context,
	subaccountID common.Hash,
	quoteDenom string,
) (uint32, bool, error) {
	defer k.Meter(ctx).FuncTiming(&ctx, "getCrossPoolQuoteDecimalsForDenom")()

	marketIDs := k.GetAllActiveDerivativeMarketIDsForSubaccount(ctx, subaccountID)

	var found bool
	var result uint32

	for _, marketID := range marketIDs {
		// Use GetDerivativeMarketByID to check both enabled and disabled markets.
		// Positions/orders may exist in disabled markets and must be accounted for.
		market := k.GetDerivativeMarketByID(ctx, marketID)
		if market == nil {
			continue
		}
		if market.GetMarketType().IsBinaryOptions() {
			continue
		}
		if market.QuoteDenom != quoteDenom {
			continue
		}
		if market.QuoteDecimals == 0 {
			// Fail closed: if there is activity in this denom pool but the market has invalid decimals,
			// collateral-decreasing actions must be blocked (otherwise maintenance/admission checks can be skipped).
			return 0, false, sdkerrors.Wrapf(
				types.ErrInvalidQuoteDenom,
				"invalid quote decimals (0) for market %s (quote denom %s)",
				marketID.Hex(),
				quoteDenom,
			)
		}
		if !found {
			result = market.QuoteDecimals
			found = true
		} else if market.QuoteDecimals != result {
			// All markets sharing a quote denom in a cross-pool must agree on QuoteDecimals.
			// A mismatch means the balance conversion factor would be ambiguous.
			return 0, false, sdkerrors.Wrapf(
				types.ErrInvalidQuoteDenom,
				"inconsistent quote decimals in cross-pool: market %s has %d, expected %d (quote denom %s)",
				marketID.Hex(),
				market.QuoteDecimals,
				result,
				quoteDenom,
			)
		}
	}

	return result, found, nil
}

func (k AccountsMsgServer) RewardsOptOut(
	c context.Context,
	msg *v2.MsgRewardsOptOut,
) (*v2.MsgRewardsOptOutResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer k.Meter(ctx).FuncTiming(&ctx, "RewardsOptOut")()

	account, _ := sdk.AccAddressFromBech32(msg.Sender)
	if isAlreadyOptedOut := k.GetIsOptedOutOfRewards(ctx, account); isAlreadyOptedOut {
		return nil, types.ErrAlreadyOptedOutOfRewards
	}

	k.SetIsOptedOutOfRewards(ctx, account, true)

	return &v2.MsgRewardsOptOutResponse{}, nil
}

func (k AccountsMsgServer) AuthorizeStakeGrants(
	c context.Context,
	msg *v2.MsgAuthorizeStakeGrants,
) (*v2.MsgAuthorizeStakeGrantsResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer k.Meter(ctx).FuncTiming(&ctx, "AuthorizeStakeGrants")()

	if err := msg.ValidateBasic(); err != nil {
		return nil, err
	}

	granter := sdk.MustAccAddressFromBech32(msg.Sender)
	granterStake := k.CalculateStakedAmountWithoutCache(ctx, granter, types.MaxGranterDelegations)

	// ensure that the granter has enough stake to cover the grants
	grantAmountDelta := math.ZeroInt()

	// calculate the net change in grant amounts
	for _, grant := range msg.Grants {
		grantee := sdk.MustAccAddressFromBech32(grant.Grantee)
		newAmount := grant.Amount
		oldAmount := k.GetGrantAuthorization(ctx, granter, grantee)
		grantAmountDelta = grantAmountDelta.Add(newAmount).Sub(oldAmount)
	}

	existingTotalGrantAmount := k.GetTotalGrantAmount(ctx, granter)
	newTotalGrantAmount := existingTotalGrantAmount.Add(grantAmountDelta)

	if newTotalGrantAmount.GT(granterStake) {
		return nil, sdkerrors.Wrapf(types.ErrInsufficientStake,
			"new total grant amount %s exceeds total stake %s",
			newTotalGrantAmount.String(),
			granterStake.String(),
		)
	}

	// update the last delegation check time
	k.SetLastValidGrantDelegationCheckTime(ctx, msg.Sender, ctx.BlockTime().Unix())

	// process the grants
	for _, grant := range msg.Grants {
		grantee := sdk.MustAccAddressFromBech32(grant.Grantee)
		k.AuthorizeStakeGrant(ctx, granter, grantee, grant.Amount)
	}

	k.EmitEvent(ctx, &v2.EventGrantAuthorizations{
		Granter: granter.String(),
		Grants:  msg.Grants,
	})

	return &v2.MsgAuthorizeStakeGrantsResponse{}, nil
}

func (k AccountsMsgServer) ActivateStakeGrant(
	c context.Context,
	msg *v2.MsgActivateStakeGrant,
) (*v2.MsgActivateStakeGrantResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer k.Meter(ctx).FuncTiming(&ctx, "ActivateStakeGrant")()

	if err := msg.ValidateBasic(); err != nil {
		return nil, err
	}
	grantee := sdk.MustAccAddressFromBech32(msg.Sender)
	granter := sdk.MustAccAddressFromBech32(msg.Granter)

	if !k.ExistsGrantAuthorization(ctx, granter, grantee) {
		return nil, sdkerrors.Wrapf(types.ErrInvalidStakeGrant, "grant from %s for %s does not exist", granter.String(), grantee.String())
	}

	granterStake := k.CalculateStakedAmountWithoutCache(ctx, granter, types.MaxGranterDelegations)
	totalGrantAmount := k.GetTotalGrantAmount(ctx, granter)

	if totalGrantAmount.GT(granterStake) {
		return nil, sdkerrors.Wrapf(
			types.ErrInvalidStakeGrant,
			"grant from %s to %s is invalid since granter staked amount %v is smaller than granter total stake delegated amount %v",
			granter.String(),
			grantee.String(),
			granterStake,
			totalGrantAmount,
		)
	}

	grantAuthorizationAmount := k.GetGrantAuthorization(ctx, granter, grantee)
	k.SetActiveGrant(ctx, grantee, v2.NewActiveGrant(granter, grantAuthorizationAmount))

	return &v2.MsgActivateStakeGrantResponse{}, nil
}
