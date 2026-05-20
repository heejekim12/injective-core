package keeper

import (
	"runtime/debug"
	"sort"

	"github.com/cosmos/cosmos-sdk/telemetry"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"

	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/risk"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/types/v2"
)

// BuildDerivativeStageRiskPrepass computes a deterministic risk pre-pass for a specific derivative matching stage.
//
// Cross margin (FULL_HOLD / order locking):
// - derives the canonical touched subaccounts (bounded transient sources),
// - materializes effective profiles,
// - precomputes per-(subaccount, quoteDenom) cross-margin snapshots for touched cross-mode subaccounts.
//
// NOTE: this pre-pass intentionally does not attempt to scan the full global orderbook for all resting orders.
// Snapshots are computed for the canonical touched set and are used as a cache/fast-path; any missing snapshots
// must be handled deterministically by matching-time fallbacks.
//
// This function is safe to call from EndBlocker - it recovers from panics and returns an empty result
// rather than halting consensus. Matching-time last-look pruning handles missing snapshots deterministically.
//
//nolint:revive // cyclomatic: prepass builds snapshots across multiple dimensions
func (k *Keeper) BuildDerivativeStageRiskPrepass(ctx sdk.Context, stageMarketIDs []common.Hash) (result *risk.PrepassResult) {
	defer k.Meter(ctx).FuncTiming(&ctx, "BuildDerivativeStageRiskPrepass")()

	// Recover from any panics to prevent consensus halt.
	// Missing prepass results are handled deterministically by matching-time fallbacks.
	defer func() {
		if r := recover(); r != nil {
			ctx.Logger().Error("BuildDerivativeStageRiskPrepass recovered from panic, returning empty result",
				"panic", r,
				"stack", string(debug.Stack()),
			)
			telemetry.IncrCounter(1, "exchange", "risk_prepass_panic")
			result = &risk.PrepassResult{
				TouchedSubaccounts: nil,
				CrossPoolSnapshots: nil,
			}
		}
	}()

	return k.buildDerivativeStageRiskPrepassInternal(ctx, stageMarketIDs)
}

// buildDerivativeStageRiskPrepassInternal is the actual implementation, separated for panic recovery.
//
//nolint:revive // cyclomatic: prepass builds snapshots across multiple dimensions
func (k *Keeper) buildDerivativeStageRiskPrepassInternal(ctx sdk.Context, stageMarketIDs []common.Hash) *risk.PrepassResult {
	defer k.Meter(ctx).FuncTiming(&ctx, "buildDerivativeStageRiskPrepassInternal")()

	touched := k.GetRiskTouchedSubaccounts(ctx)

	crossSubaccounts := make([]common.Hash, 0)
	for _, subaccountID := range touched {
		profile, _ := k.GetEffectiveSubaccountRiskProfile(ctx, subaccountID)
		if profile == nil {
			continue
		}
		if profile.Mode == v2.RiskMode_RISK_MODE_CROSS {
			crossSubaccounts = append(crossSubaccounts, subaccountID)
		}
	}

	// Group stage markets by quote denom for stage-consistent snapshot building.
	type denomGroup struct {
		quoteDecimals uint32
	}
	groups := make(map[string]denomGroup)

	for _, marketID := range stageMarketIDs {
		market, _ := k.GetDerivativeOrBinaryOptionsMarketWithMarkPrice(ctx, marketID, true)
		if market == nil {
			continue
		}
		if market.GetMarketType().IsBinaryOptions() {
			// Binary options remain isolated-only.
			continue
		}

		quoteDenom := market.GetQuoteDenom()
		quoteDecimals := market.GetQuoteDecimals()

		// Validate quoteDecimals to catch configuration errors early.
		// Zero decimals would cause incorrect notional calculations.
		if quoteDecimals == 0 {
			k.Logger(ctx).Error("market has zero quoteDecimals, skipping in risk prepass",
				"marketID", marketID.Hex(),
				"quoteDenom", quoteDenom)
			continue
		}

		group := groups[quoteDenom]
		if group.quoteDecimals == 0 {
			group.quoteDecimals = quoteDecimals
		}
		groups[quoteDenom] = group
	}

	snapshots := make(map[common.Hash]map[string]*risk.CrossPoolSnapshot, len(crossSubaccounts))
	for _, subaccountID := range crossSubaccounts {
		// Determine which quote denoms this subaccount has activity in,
		// and only build snapshots for those denoms. This avoids unnecessary snapshot
		// computation when the stage has many denoms but the subaccount only trades in a few.
		activeMarkets := k.GetAllActiveDerivativeMarketIDsForSubaccount(ctx, subaccountID)

		relevantDenoms := make(map[string]struct{})
		for _, marketID := range activeMarkets {
			market, _ := k.GetDerivativeOrBinaryOptionsMarketWithMarkPrice(ctx, marketID, true)
			if market == nil {
				continue
			}
			quoteDenom := market.GetQuoteDenom()
			if group, ok := groups[quoteDenom]; ok && group.quoteDecimals != 0 {
				relevantDenoms[quoteDenom] = struct{}{}
			}
		}

		if len(relevantDenoms) == 0 {
			continue
		}

		// Sort quote denoms for deterministic iteration order across validators.
		sortedDenoms := make([]string, 0, len(relevantDenoms))
		for denom := range relevantDenoms {
			sortedDenoms = append(sortedDenoms, denom)
		}
		sort.Strings(sortedDenoms)

		byDenom := make(map[string]*risk.CrossPoolSnapshot, len(relevantDenoms))
		for _, quoteDenom := range sortedDenoms {
			group := groups[quoteDenom]
			snapshot, err := k.RiskEngine().BuildCrossPoolSnapshot(ctx, subaccountID, quoteDenom, group.quoteDecimals)
			if err != nil {
				// Best-effort only: missing snapshots must be handled deterministically during matching.
				continue
			}

			byDenom[quoteDenom] = snapshot
		}
		if len(byDenom) > 0 {
			snapshots[subaccountID] = byDenom
		}
	}

	return &risk.PrepassResult{
		TouchedSubaccounts: touched,
		CrossPoolSnapshots: snapshots,
	}
}
