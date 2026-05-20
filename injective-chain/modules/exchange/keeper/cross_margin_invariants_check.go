package keeper

import (
	"fmt"
	"strings"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"

	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/risk"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/types/v2"
)

// CrossMarginInvariantError collects all violations found during a cross-margin invariant check.
// Callers should inspect the error message for the full list of violations.
type CrossMarginInvariantError struct {
	Violations []string
}

func (e *CrossMarginInvariantError) Error() string {
	return fmt.Sprintf("cross-margin invariant violations (%d):\n  %s", len(e.Violations), strings.Join(e.Violations, "\n  "))
}

func (e *CrossMarginInvariantError) addViolation(format string, args ...any) {
	e.Violations = append(e.Violations, fmt.Sprintf(format, args...))
}

func (e *CrossMarginInvariantError) toError() error {
	if len(e.Violations) == 0 {
		return nil
	}
	return e
}

// ValidateCrossMarginExposureIndexIntegrity checks that the exposure indexes
// (ActiveDerivativeMarketsBySubaccount and ActiveDerivativeOrderMarketsBySubaccount)
// are consistent with actual positions and orders for all cross-margin subaccounts.
//
// This should only be used by tests and fuzz tooling to verify data integrity.
func (k *Keeper) ValidateCrossMarginExposureIndexIntegrity(ctx sdk.Context) error {
	defer k.Meter(ctx).FuncTiming(&ctx, "ValidateCrossMarginExposureIndexIntegrity")()

	violations := &CrossMarginInvariantError{}

	profiles := k.GetAllSubaccountRiskProfiles(ctx)
	allMarkets := k.GetAllDerivativeMarkets(ctx)

	for _, record := range profiles {
		if record.RiskProfile.Mode != v2.RiskMode_RISK_MODE_CROSS {
			continue
		}

		subaccountID := common.HexToHash(record.SubaccountId)
		k.validateSubaccountExposureIndex(ctx, subaccountID, allMarkets, violations)
	}

	return violations.toError()
}

func (k *Keeper) validateSubaccountExposureIndex(ctx sdk.Context, subaccountID common.Hash, allMarkets []*v2.DerivativeMarket, violations *CrossMarginInvariantError) {
	indexedPositionSet := k.validatePositionIndexForward(ctx, subaccountID, violations)

	for _, market := range allMarkets {
		marketID := market.MarketID()
		if _, indexed := indexedPositionSet[marketID]; indexed {
			continue
		}
		if k.HasPosition(ctx, marketID, subaccountID) {
			violations.addViolation(
				"subaccount %s has a position in market %s but market is not in position index",
				subaccountID.Hex(), marketID.Hex(),
			)
		}
	}

	indexedOrderSet := k.validateOrderIndexForward(ctx, subaccountID, violations)

	for _, market := range allMarkets {
		marketID := market.MarketID()
		if _, indexed := indexedOrderSet[marketID]; indexed {
			continue
		}
		metaBuy := k.GetSubaccountOrderbookMetadata(ctx, marketID, subaccountID, true)
		metaSell := k.GetSubaccountOrderbookMetadata(ctx, marketID, subaccountID, false)
		if metaBuy.GetOrderSideCount() > 0 || metaSell.GetOrderSideCount() > 0 {
			violations.addViolation(
				"subaccount %s has orders in market %s (buy: %d, sell: %d) but market is not in order index",
				subaccountID.Hex(), marketID.Hex(),
				metaBuy.GetOrderSideCount(), metaSell.GetOrderSideCount(),
			)
		}
	}
}

func (k *Keeper) validatePositionIndexForward(ctx sdk.Context, subaccountID common.Hash, violations *CrossMarginInvariantError) map[common.Hash]struct{} {
	indexedPositionMarkets := k.GetActiveDerivativeMarketsBySubaccount(ctx, subaccountID)
	indexedPositionSet := make(map[common.Hash]struct{}, len(indexedPositionMarkets))
	for _, marketID := range indexedPositionMarkets {
		indexedPositionSet[marketID] = struct{}{}
		if !k.HasPosition(ctx, marketID, subaccountID) {
			violations.addViolation(
				"position index contains market %s for subaccount %s but no position exists",
				marketID.Hex(), subaccountID.Hex(),
			)
		}
	}
	return indexedPositionSet
}

func (k *Keeper) validateOrderIndexForward(ctx sdk.Context, subaccountID common.Hash, violations *CrossMarginInvariantError) map[common.Hash]struct{} {
	indexedOrderMarkets := k.GetActiveDerivativeOrderMarketsBySubaccount(ctx, subaccountID)
	indexedOrderSet := make(map[common.Hash]struct{}, len(indexedOrderMarkets))
	for _, marketID := range indexedOrderMarkets {
		indexedOrderSet[marketID] = struct{}{}
		metaBuy := k.GetSubaccountOrderbookMetadata(ctx, marketID, subaccountID, true)
		metaSell := k.GetSubaccountOrderbookMetadata(ctx, marketID, subaccountID, false)
		hasBuyOrders := metaBuy.GetOrderSideCount() > 0
		hasSellOrders := metaSell.GetOrderSideCount() > 0
		if !hasBuyOrders && !hasSellOrders {
			violations.addViolation(
				"order index contains market %s for subaccount %s but no orders found in orderbook metadata",
				marketID.Hex(), subaccountID.Hex(),
			)
		}
	}
	return indexedOrderSet
}

// ValidateCrossMarginRiskProfileConsistency checks that every cross-margin subaccount's
// derivative exposure uses only enabled cross-margin quote denoms. A subaccount may have
// exposure across multiple independent quote-denom pools; each pool is evaluated separately.
//
// This should only be used by tests and fuzz tooling to verify data integrity.
func (k *Keeper) ValidateCrossMarginRiskProfileConsistency(ctx sdk.Context) error {
	defer k.Meter(ctx).FuncTiming(&ctx, "ValidateCrossMarginRiskProfileConsistency")()

	violations := &CrossMarginInvariantError{}

	profiles := k.GetAllSubaccountRiskProfiles(ctx)
	params := k.GetParams(ctx)

	enabledDenoms := make(map[string]struct{})
	for _, denom := range params.CrossMarginParams.EnabledQuoteDenoms {
		enabledDenoms[denom] = struct{}{}
	}

	for _, record := range profiles {
		if record.RiskProfile.Mode != v2.RiskMode_RISK_MODE_CROSS {
			continue
		}

		subaccountID := common.HexToHash(record.SubaccountId)

		// Collect all quote denoms from position markets.
		quoteDenoms := make(map[string]struct{})

		positionMarkets := k.GetActiveDerivativeMarketsBySubaccount(ctx, subaccountID)
		for _, marketID := range positionMarkets {
			market := k.GetDerivativeMarketByID(ctx, marketID)
			if market == nil {
				violations.addViolation(
					"subaccount %s has position index entry for market %s but market does not exist",
					subaccountID.Hex(), marketID.Hex(),
				)
				continue
			}
			quoteDenoms[market.GetQuoteDenom()] = struct{}{}
		}

		// Also check order markets.
		orderMarkets := k.GetActiveDerivativeOrderMarketsBySubaccount(ctx, subaccountID)
		for _, marketID := range orderMarkets {
			market := k.GetDerivativeMarketByID(ctx, marketID)
			if market == nil {
				violations.addViolation(
					"subaccount %s has order index entry for market %s but market does not exist",
					subaccountID.Hex(), marketID.Hex(),
				)
				continue
			}
			quoteDenoms[market.GetQuoteDenom()] = struct{}{}
		}

		// Each quote denom must be in the enabled set.
		for denom := range quoteDenoms {
			if _, ok := enabledDenoms[denom]; !ok {
				violations.addViolation(
					"cross-margin subaccount %s has exposure in quote denom %s which is not in enabled denoms %v",
					subaccountID.Hex(), denom, params.CrossMarginParams.EnabledQuoteDenoms,
				)
			}
		}
	}

	return violations.toError()
}

// ValidateCrossMarginSnapshotArithmetic rebuilds the cross-pool snapshot from scratch for every
// cross-margin subaccount and validates internal consistency of the computed values.
//
// This should only be used by tests and fuzz tooling to verify data integrity.
func (k *Keeper) ValidateCrossMarginSnapshotArithmetic(ctx sdk.Context) error {
	defer k.Meter(ctx).FuncTiming(&ctx, "ValidateCrossMarginSnapshotArithmetic")()

	violations := &CrossMarginInvariantError{}

	profiles := k.GetAllSubaccountRiskProfiles(ctx)

	for _, record := range profiles {
		if record.RiskProfile.Mode != v2.RiskMode_RISK_MODE_CROSS {
			continue
		}

		subaccountID := common.HexToHash(record.SubaccountId)
		k.validateSubaccountSnapshotArithmetic(ctx, subaccountID, violations)
	}

	return violations.toError()
}

func (k *Keeper) validateSubaccountSnapshotArithmetic(ctx sdk.Context, subaccountID common.Hash, violations *CrossMarginInvariantError) {
	// Collect all distinct quote denom pools for this subaccount from positions and orders.
	type poolKey struct {
		quoteDenom    string
		quoteDecimals uint32
	}
	pools := make(map[string]poolKey)

	allMarketIDs := risk.MergeAndSortMarketIDs(
		k.GetActiveDerivativeMarketsBySubaccount(ctx, subaccountID),
		k.GetActiveDerivativeOrderMarketsBySubaccount(ctx, subaccountID),
	)

	for _, marketID := range allMarketIDs {
		market := k.GetDerivativeMarketByID(ctx, marketID)
		if market == nil {
			continue
		}
		denom := market.GetQuoteDenom()
		if _, exists := pools[denom]; !exists {
			pools[denom] = poolKey{quoteDenom: denom, quoteDecimals: market.QuoteDecimals}
		}
	}

	for _, pool := range pools {
		snapshot, err := k.RiskEngine().BuildCrossPoolSnapshot(ctx, subaccountID, pool.quoteDenom, pool.quoteDecimals)
		if err != nil {
			violations.addViolation("subaccount %s pool %s: BuildCrossPoolSnapshot failed: %v", subaccountID.Hex(), pool.quoteDenom, err)
			continue
		}

		validateSnapshotMargins(subaccountID, snapshot, violations)
		validateSnapshotHealthFactor(subaccountID, snapshot, violations)
		validateSnapshotHaircut(subaccountID, snapshot, violations)
	}
}

func validateSnapshotMargins(subaccountID common.Hash, snapshot *risk.CrossPoolSnapshot, violations *CrossMarginInvariantError) {
	if snapshot.MaintenanceMarginTotal.IsPositive() && snapshot.EquityLiquidation.IsNegative() {
		violations.addViolation(
			"subaccount %s: negative equity_liquidation %s with maintenance_margin %s (missed liquidation?)",
			subaccountID.Hex(), snapshot.EquityLiquidation.String(), snapshot.MaintenanceMarginTotal.String(),
		)
	}
	if snapshot.MaintenanceMarginTotal.IsNegative() {
		violations.addViolation(
			"subaccount %s: negative maintenance_margin_total %s",
			subaccountID.Hex(), snapshot.MaintenanceMarginTotal.String(),
		)
	}
	if snapshot.InitialMarginTotal.LT(snapshot.MaintenanceMarginTotal) {
		violations.addViolation(
			"subaccount %s: initial_margin_total %s < maintenance_margin_total %s",
			subaccountID.Hex(), snapshot.InitialMarginTotal.String(), snapshot.MaintenanceMarginTotal.String(),
		)
	}
	if snapshot.OrderLockRequirement.IsNegative() {
		violations.addViolation(
			"subaccount %s: negative order_lock_requirement %s",
			subaccountID.Hex(), snapshot.OrderLockRequirement.String(),
		)
	}
}

func validateSnapshotHealthFactor(subaccountID common.Hash, snapshot *risk.CrossPoolSnapshot, violations *CrossMarginInvariantError) {
	if snapshot.HealthFactor == nil || !snapshot.MaintenanceMarginTotal.IsPositive() {
		return
	}
	expected := snapshot.EquityLiquidation.Quo(snapshot.MaintenanceMarginTotal)
	diff := snapshot.HealthFactor.Sub(expected).Abs()
	tolerance := math.LegacyMustNewDecFromStr("0.000001")
	if diff.GT(tolerance) {
		violations.addViolation(
			"subaccount %s: health_factor %s != equity_liquidation/maintenance_margin (%s/%s = %s)",
			subaccountID.Hex(), snapshot.HealthFactor.String(),
			snapshot.EquityLiquidation.String(), snapshot.MaintenanceMarginTotal.String(),
			expected.String(),
		)
	}
}

func validateSnapshotHaircut(subaccountID common.Hash, snapshot *risk.CrossPoolSnapshot, violations *CrossMarginInvariantError) {
	if snapshot.PositiveUPnLHaircutRate.IsNegative() || snapshot.PositiveUPnLHaircutRate.GT(math.LegacyOneDec()) {
		violations.addViolation(
			"subaccount %s: positive_upnl_haircut_rate %s out of [0, 1] range",
			subaccountID.Hex(), snapshot.PositiveUPnLHaircutRate.String(),
		)
	}
	if snapshot.PositiveUPnLHaircutRate.IsPositive() && snapshot.UnrealizedPnlEff.IsPositive() {
		if snapshot.EquityAdmission.GT(snapshot.EquityLiquidation) {
			violations.addViolation(
				"subaccount %s: equity_admission %s > equity_liquidation %s with positive haircut and positive UPnL",
				subaccountID.Hex(), snapshot.EquityAdmission.String(), snapshot.EquityLiquidation.String(),
			)
		}
	}
}

// ValidateCrossMarginDepositConstraints checks that cross-margin subaccounts have valid deposit
// balances for their pool quote denom.
//
// This should only be used by tests and fuzz tooling to verify data integrity.
func (k *Keeper) ValidateCrossMarginDepositConstraints(ctx sdk.Context) error {
	defer k.Meter(ctx).FuncTiming(&ctx, "ValidateCrossMarginDepositConstraints")()

	violations := &CrossMarginInvariantError{}

	profiles := k.GetAllSubaccountRiskProfiles(ctx)

	for _, record := range profiles {
		if record.RiskProfile.Mode != v2.RiskMode_RISK_MODE_CROSS {
			continue
		}

		subaccountID := common.HexToHash(record.SubaccountId)
		positionMarkets := k.GetActiveDerivativeMarketsBySubaccount(ctx, subaccountID)

		// Collect all distinct quote denoms across position markets.
		quoteDenoms := make(map[string]struct{})
		for _, marketID := range positionMarkets {
			market := k.GetDerivativeMarketByID(ctx, marketID)
			if market != nil {
				quoteDenoms[market.GetQuoteDenom()] = struct{}{}
			}
		}

		for quoteDenom := range quoteDenoms {
			deposit := k.GetDeposit(ctx, subaccountID, quoteDenom)

			if deposit == nil {
				violations.addViolation(
					"subaccount %s: has positions but nil deposit for quote denom %s",
					subaccountID.Hex(), quoteDenom,
				)
				continue
			}

			if deposit.TotalBalance.IsNegative() {
				violations.addViolation(
					"subaccount %s: negative total_balance %s for quote denom %s",
					subaccountID.Hex(), deposit.TotalBalance.String(), quoteDenom,
				)
			}

		if deposit.AvailableBalance.IsNegative() {
			violations.addViolation(
				"subaccount %s: negative available_balance %s for quote denom %s",
				subaccountID.Hex(), deposit.AvailableBalance.String(), quoteDenom,
			)
		}

		// Allow small tolerance for available > total (matching existing metadata invariant).
		diff := deposit.AvailableBalance.Sub(deposit.TotalBalance)
		tolerance := math.LegacyMustNewDecFromStr("0.000001")
		if diff.GT(tolerance) {
			violations.addViolation(
				"subaccount %s: available_balance %s exceeds total_balance %s by %s for quote denom %s",
				subaccountID.Hex(), deposit.AvailableBalance.String(),
				deposit.TotalBalance.String(), diff.String(), quoteDenom,
			)
		}
		}
	}

	return violations.toError()
}

// ValidateCrossMarginPoolLimits checks that no cross-margin subaccount exceeds the maximum
// number of active derivative markets per pool.
//
// This should only be used by tests and fuzz tooling to verify data integrity.
func (k *Keeper) ValidateCrossMarginPoolLimits(ctx sdk.Context) error {
	defer k.Meter(ctx).FuncTiming(&ctx, "ValidateCrossMarginPoolLimits")()

	violations := &CrossMarginInvariantError{}

	profiles := k.GetAllSubaccountRiskProfiles(ctx)
	params := k.GetParams(ctx)

	maxMarkets := params.CrossMarginParams.MaxActiveDerivativeMarketsPerPool
	if maxMarkets == 0 {
		maxMarkets = 100 // same default as runtime admission (cross_margin_snapshot.go, msg_server_accounts.go)
	}

	for _, record := range profiles {
		if record.RiskProfile.Mode != v2.RiskMode_RISK_MODE_CROSS {
			continue
		}

		subaccountID := common.HexToHash(record.SubaccountId)
		k.validateSubaccountPoolLimits(ctx, subaccountID, maxMarkets, violations)
	}

	return violations.toError()
}

func (k *Keeper) validateSubaccountPoolLimits(ctx sdk.Context, subaccountID common.Hash, maxMarkets uint32, violations *CrossMarginInvariantError) {
	positionMarkets := k.GetActiveDerivativeMarketsBySubaccount(ctx, subaccountID)
	orderMarkets := k.GetActiveDerivativeOrderMarketsBySubaccount(ctx, subaccountID)

	perPool := make(map[string]map[common.Hash]struct{})

	allMarketIDs := make([]common.Hash, 0, len(positionMarkets)+len(orderMarkets))
	allMarketIDs = append(allMarketIDs, positionMarkets...)
	allMarketIDs = append(allMarketIDs, orderMarkets...)

	for _, marketID := range allMarketIDs {
		market := k.GetDerivativeMarketByID(ctx, marketID)
		if market == nil || market.GetMarketType().IsBinaryOptions() {
			continue
		}
		if perPool[market.QuoteDenom] == nil {
			perPool[market.QuoteDenom] = make(map[common.Hash]struct{})
		}
		perPool[market.QuoteDenom][marketID] = struct{}{}
	}

	for denom, markets := range perPool {
		if uint32(len(markets)) > maxMarkets {
			violations.addViolation(
				"subaccount %s, pool %s: %d active derivative markets exceeds pool limit of %d",
				subaccountID.Hex(), denom, len(markets), maxMarkets,
			)
		}
	}
}

// ValidateAllCrossMarginInvariants runs all cross-margin invariant checks and returns a combined error
// if any violations are found.
//
// This should only be used by tests and fuzz tooling to verify data integrity.
func (k *Keeper) ValidateAllCrossMarginInvariants(ctx sdk.Context) error {
	defer k.Meter(ctx).FuncTiming(&ctx, "ValidateAllCrossMarginInvariants")()

	var allViolations []string

	checks := []struct {
		name string
		fn   func(sdk.Context) error
	}{
		{"exposure_index_integrity", k.ValidateCrossMarginExposureIndexIntegrity},
		{"risk_profile_consistency", k.ValidateCrossMarginRiskProfileConsistency},
		{"snapshot_arithmetic", k.ValidateCrossMarginSnapshotArithmetic},
		{"deposit_constraints", k.ValidateCrossMarginDepositConstraints},
		{"pool_limits", k.ValidateCrossMarginPoolLimits},
	}

	for _, check := range checks {
		if err := check.fn(ctx); err != nil {
			allViolations = append(allViolations, fmt.Sprintf("[%s] %s", check.name, err.Error()))
		}
	}

	if len(allViolations) == 0 {
		return nil
	}

	return fmt.Errorf("cross-margin invariant failures:\n  %s", strings.Join(allViolations, "\n  "))
}
