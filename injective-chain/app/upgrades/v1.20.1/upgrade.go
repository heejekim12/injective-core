//revive:disable-next-line:package-directory-mismatch // Semver upgrade directory names cannot be valid Go package identifiers.
package v1dot20dot1

import (
	"fmt"
	"runtime/debug"
	"strings"
	"time"

	"cosmossdk.io/errors"
	"cosmossdk.io/log"
	"cosmossdk.io/math"
	storetypes "cosmossdk.io/store/types"
	probabilistic "github.com/cardano-foundation/cardano-ibc-incubator/cosmos/cardano-probabilistic-light-client-v8"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingkeeper "github.com/cosmos/cosmos-sdk/x/staking/keeper"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"github.com/InjectiveLabs/injective-core/injective-chain/app/upgrades"
	oracletypes "github.com/InjectiveLabs/injective-core/injective-chain/modules/oracle/types"
)

const UpgradeVersion = "v1.20.1"

func StoreUpgrades() storetypes.StoreUpgrades {
	return storetypes.StoreUpgrades{
		Added:   nil,
		Renamed: nil,
		Deleted: nil,
	}
}

func UpgradeSteps() []*upgrades.UpgradeHandlerStep {
	return []*upgrades.UpgradeHandlerStep{
		upgrades.NewUpgradeHandlerStep(
			"Allow Cardano probabilistic IBC client",
			UpgradeVersion,
			upgrades.MainnetChainID,
			AllowCardanoProbabilisticIBCClient,
		),
		upgrades.NewUpgradeHandlerStep(
			"Backfill peggy rate limit oracle type",
			UpgradeVersion,
			upgrades.MainnetChainID,
			MigratePeggyRateLimitOracleToPythPro,
		),
		upgrades.NewUpgradeHandlerStep(
			"Remove dust delegations",
			UpgradeVersion,
			upgrades.MainnetChainID,
			RemoveDustDelegations,
		),
	}
}

func AllowCardanoProbabilisticIBCClient(ctx sdk.Context, app upgrades.InjectiveApplication, logger log.Logger) error {
	clientKeeper := app.GetIBCKeeper().ClientKeeper
	params := clientKeeper.GetParams(ctx)

	if params.IsAllowedClient(probabilistic.ModuleName) {
		logger.Info("Cardano probabilistic IBC client already allowed", "client_type", probabilistic.ModuleName)
		return nil
	}

	params.AllowedClients = append(params.AllowedClients, probabilistic.ModuleName)
	if err := params.Validate(); err != nil {
		return errors.Wrapf(err, "validate IBC client params after adding %s", probabilistic.ModuleName)
	}

	clientKeeper.SetParams(ctx, params)
	logger.Info(
		"Cardano probabilistic IBC client allowed",
		"client_type", probabilistic.ModuleName,
		"allowed_clients", params.AllowedClients,
	)

	return nil
}

// MigratePeggyRateLimitOracleToPythPro sets all existing Peggy rate limits to use
// Pyth Pro price feeds.
func MigratePeggyRateLimitOracleToPythPro(ctx sdk.Context, app upgrades.InjectiveApplication, logger log.Logger) error {
	env := map[string]map[string]string{
		upgrades.MainnetChainID: {
			"0xe28b3B32B6c345A34Ff64674606124Dd5Aceca30": "46", // INJ (Pyth Pro)
			"0xdAC17F958D2ee523a2206206994597C13D831ec7": "8",  // USDT (Pyth Pro)
		},
		upgrades.TestnetChainID: {
			"0x73642f6b754d276Ce286E96E8117f1f452acaf02": "46", // INJ (Pyth Pro)
			"0x87aB3B4C8661e07D6372361211B96ed4Dc36B1B5": "8",  // USDT (Pyth Pro)
		},
		upgrades.DevnetChainID: {},
	}

	// correctly populate existing rate limits oracle types
	pk := app.GetPeggyKeeper()
	for _, rateLimit := range pk.GetRateLimits(ctx) {
		rateLimit.TokenOracleType = oracletypes.OracleType_Pyth // all existing records are in Pyth
		pk.SetRateLimit(ctx, rateLimit)
	}

	// sanity check
	ok := app.GetOracleKeeper()
	price := ok.GetReferencePrice(ctx, oracletypes.OracleType_PythPro, "46", oracletypes.QuoteUSD)
	if price == nil || price.IsNil() || !price.IsPositive() {
		logger.Error("no reference price present for pyth pro", "base", "46", "token", "INJ")
		return nil
	}

	price = ok.GetReferencePrice(ctx, oracletypes.OracleType_PythPro, "8", oracletypes.QuoteUSD)
	if price == nil || price.IsNil() || !price.IsPositive() {
		logger.Error("no reference price present for pyth pro", "base", "8", "token", "USDT")
		return nil
	}

	// migrate to pyth pro
	for _, rateLimit := range pk.GetRateLimits(ctx) {
		rateLimit.TokenOracleType = oracletypes.OracleType_PythPro
		rateLimit.TokenPriceId = env[ctx.ChainID()][rateLimit.TokenAddress]

		pk.SetRateLimit(ctx, rateLimit)
		logger.Info("updated peggy rate limit oracle type", "type", rateLimit.TokenOracleType.String(), "token", rateLimit.TokenAddress)
	}

	return nil
}

// RemoveDustDelegations removes all dust delegations from the staking module state.
//
// A delegation is considered dust when it holds a positive amount of shares whose
// equivalent staked token value settles to zero through the staking keeper's Unbond
// routine (i.e. validator.TokensFromShares(shares) — the rounding conversion Unbond
// uses to settle — truncates to 0).
func RemoveDustDelegations(ctx sdk.Context, app upgrades.InjectiveApplication, logger log.Logger) (returnErr error) {
	commitStarted := false
	defer func() {
		if r := recover(); r != nil {
			if commitStarted {
				logger.Error("dust delegation sweep panicked while committing, aborting upgrade step to avoid partial state",
					"panic", fmt.Sprint(r),
					"stack", string(debug.Stack()),
				)
				panic(r)
			}

			// The sweep is optional: if anything panics before the final cache write,
			// discard its cached writes instead of aborting the upgrade.
			logger.Error("dust delegation sweep panicked, skipping migration",
				"panic", fmt.Sprint(r),
				"stack", string(debug.Stack()),
			)
			returnErr = nil
		}
	}()

	start := time.Now()
	sk := app.GetStakingKeeper()
	var totalCleaned, totalFailed int
	totalDustShares := math.LegacyZeroDec()

	stats, candidateDelegations, statsIterationErr := collectDustDelegationCandidates(ctx, sk, logger)
	logger.Info("dust delegation sweep starting",
		"total_to_cleanup", stats.TotalToCleanup,
		"total_shares_to_cleanup", legacyDecLogString(stats.TotalShares),
		"min_share", legacyDecLogString(stats.MinShares),
		"max_share", legacyDecLogString(stats.MaxShares),
		"failed_to_inspect", stats.FailedToInspect,
		"inspection_iteration_error", statsIterationErr,
	)

	// Never write to the context that backed IterateAllDelegations until the iterator
	// has been closed. The staking store iterator contract does not allow mutation
	// during iteration, even via a child CacheContext that writes back to this context.
	sweepCtx, writeSweep := ctx.CacheContext()
	for _, delegation := range candidateDelegations {
		cleaned, shares, failed := tryCleanDustDelegation(sweepCtx, sk, delegation, logger)
		if failed {
			totalFailed++
			continue
		}
		if cleaned {
			totalCleaned++
			totalDustShares = totalDustShares.Add(shares)
			if totalCleaned%5000 == 0 {
				logger.Info("dust delegation sweep progress",
					"progress", fmt.Sprintf("%d/%d total", totalCleaned, stats.TotalToCleanup),
					"removed", totalCleaned,
					"total_to_cleanup", stats.TotalToCleanup,
					"percent_done", dustCleanupPercent(totalCleaned, stats.TotalToCleanup),
					"total_removed_shares", legacyDecLogString(totalDustShares),
				)
			}
		}
	}

	logger.Info("dust delegation sweep finished",
		"cleaned", totalCleaned,
		"failed", totalFailed,
		"total_dust_shares", legacyDecLogString(totalDustShares),
		"iteration_error", statsIterationErr,
		"elapsed", time.Since(start).String(),
	)

	commitStarted = true
	writeSweep()

	// The sweep is best-effort: never fail the upgrade because of it.
	return nil
}

type dustDelegationStats struct {
	TotalToCleanup  int
	FailedToInspect int
	TotalShares     math.LegacyDec
	MinShares       math.LegacyDec
	MaxShares       math.LegacyDec
}

func collectDustDelegationCandidates(
	ctx sdk.Context,
	sk *stakingkeeper.Keeper,
	logger log.Logger,
) (dustDelegationStats, []stakingtypes.Delegation, error) {
	stats := dustDelegationStats{
		TotalShares: math.LegacyZeroDec(),
		MinShares:   math.LegacyZeroDec(),
		MaxShares:   math.LegacyZeroDec(),
	}
	candidateDelegations := make([]stakingtypes.Delegation, 0)

	iterationErr := sk.IterateAllDelegations(ctx, func(delegation stakingtypes.Delegation) (stop bool) {
		defer func() {
			if r := recover(); r != nil {
				stats.FailedToInspect++
				logger.Error("panic while inspecting dust delegation, skipping from initial stats",
					"delegator", delegation.DelegatorAddress,
					"validator", delegation.ValidatorAddress,
					"panic", fmt.Sprint(r),
				)
			}
		}()

		_, shares, isDust, err := dustDelegationShares(ctx, sk, delegation)
		if err != nil {
			stats.FailedToInspect++
			return false
		}
		if !isDust {
			return false
		}

		stats.TotalToCleanup++
		stats.TotalShares = stats.TotalShares.Add(shares)
		candidateDelegations = append(candidateDelegations, delegation)
		if stats.TotalToCleanup == 1 || shares.LT(stats.MinShares) {
			stats.MinShares = shares
		}
		if stats.TotalToCleanup == 1 || shares.GT(stats.MaxShares) {
			stats.MaxShares = shares
		}

		return false
	})

	return stats, candidateDelegations, iterationErr
}

func tryCleanDustDelegation(
	ctx sdk.Context,
	sk *stakingkeeper.Keeper,
	delegation stakingtypes.Delegation,
	logger log.Logger,
) (cleaned bool, shares math.LegacyDec, failed bool) {
	commitStarted := false
	defer func() {
		if r := recover(); r != nil {
			if commitStarted {
				panic(r)
			}

			logger.Error("panic while cleaning dust delegation, skipping",
				"delegator", delegation.DelegatorAddress,
				"validator", delegation.ValidatorAddress,
				"panic", fmt.Sprint(r),
			)
			cleaned = false
			shares = math.LegacyDec{}
			failed = true
		}
	}()

	// Unbond touches staking and distribution state; commit each candidate only
	// after the full operation succeeds. The write goes to the sweep cache, not to
	// the context that was used by IterateAllDelegations.
	delegationCtx, writeDelegation := ctx.CacheContext()
	cleaned, shares, err := cleanDustDelegation(delegationCtx, sk, delegation, logger)
	if err != nil {
		return false, math.LegacyDec{}, true
	}
	if cleaned {
		commitStarted = true
		writeDelegation()
	}

	return cleaned, shares, false
}

func cleanDustDelegation(
	ctx sdk.Context,
	sk *stakingkeeper.Keeper,
	delegation stakingtypes.Delegation,
	logger log.Logger,
) (cleaned bool, shares math.LegacyDec, err error) {
	valAddr, shares, isDust, err := dustDelegationShares(ctx, sk, delegation)
	if err != nil {
		logger.Warn("failed to inspect dust delegation, skipping",
			"delegator", delegation.DelegatorAddress,
			"validator", delegation.ValidatorAddress,
			"error", err,
		)
		return false, math.LegacyDec{}, err
	}
	if !isDust {
		return false, math.LegacyDec{}, nil // not dust
	}

	validator, err := sk.GetValidator(ctx, valAddr)
	if err != nil {
		logger.Warn("failed to load validator for dust delegation, skipping",
			"delegator", delegation.DelegatorAddress,
			"validator", delegation.ValidatorAddress,
			"shares", legacyDecLogString(shares),
			"error", err,
		)
		return false, math.LegacyDec{}, err
	}

	if !validator.DelegatorShares.Sub(shares).IsPositive() {
		logger.Info("skipping dust delegation that would remove validator",
			"delegator", delegation.DelegatorAddress,
			"validator", delegation.ValidatorAddress,
			"shares", legacyDecLogString(shares),
			"validator_shares", legacyDecLogString(validator.DelegatorShares),
		)
		return false, math.LegacyDec{}, nil
	}

	delAddr, err := sdk.AccAddressFromBech32(delegation.DelegatorAddress)
	if err != nil {
		logger.Warn("failed to decode delegator address for dust delegation, skipping",
			"delegator", delegation.DelegatorAddress,
			"validator", delegation.ValidatorAddress,
			"error", err,
		)
		return false, math.LegacyDec{}, err
	}

	// Unbond performs the full, consistent teardown: it removes the delegation record,
	// calls the staking hooks and rebalances the validator tokens/shares (including the
	// power index). Because this is dust (tokens == 0) the token delta on the validator
	// is effectively zero. It does not create an unbonding-delegation entry, which is
	// the behaviour we want here.
	if _, err := sk.Unbond(ctx, delAddr, valAddr, shares); err != nil {
		logger.Warn("failed to remove dust delegation, skipping",
			"delegator", delegation.DelegatorAddress,
			"validator", delegation.ValidatorAddress,
			"shares", legacyDecLogString(shares),
			"error", err,
		)
		return false, math.LegacyDec{}, err
	}

	return true, shares, nil
}

func dustDelegationShares(
	ctx sdk.Context,
	sk *stakingkeeper.Keeper,
	delegation stakingtypes.Delegation,
) (sdk.ValAddress, math.LegacyDec, bool, error) {
	if !delegation.Shares.IsPositive() {
		return nil, math.LegacyDec{}, false, nil
	}

	valAddr, err := sdk.ValAddressFromBech32(delegation.ValidatorAddress)
	if err != nil {
		return nil, math.LegacyDec{}, false, err
	}

	validator, err := sk.GetValidator(ctx, valAddr)
	if err != nil {
		return nil, math.LegacyDec{}, false, err
	}

	// IMPORTANT: a delegation is dust only when the staking settlement itself (Unbond ->
	// RemoveValidatorTokensAndShares -> RemoveDelShares) would debit zero tokens.
	// RemoveDelShares uses the rounding conversion TokensFromShares (Quo, banker's
	// rounding), NOT the truncating one (QuoTruncate). At the 18-dp boundary the two can
	// disagree: a delegation whose truncated token value is 0 can still cause
	// RemoveDelShares to debit 1 token (e.g. tokens=1, delegShares=2,
	// shares=1.999999999999999999). Calling Unbond on such a delegation would remove up to
	// 1 validator token with no matching pool move. We therefore classify dust using the
	// SAME conversion RemoveDelShares uses, so we only ever Unbond shares that settle to 0.
	tokens := validator.TokensFromShares(delegation.Shares).TruncateInt()
	if !tokens.IsZero() {
		return nil, math.LegacyDec{}, false, nil
	}

	return valAddr, delegation.Shares, true, nil
}

func dustCleanupPercent(cleaned, total int) string {
	if total == 0 {
		return "100%"
	}

	percent := math.LegacyNewDec(int64(cleaned)).MulInt64(100).QuoInt64(int64(total))
	return legacyDecLogString(percent) + "%"
}

func legacyDecLogString(dec math.LegacyDec) string {
	value := dec.String()
	if !strings.Contains(value, ".") {
		return value
	}

	value = strings.TrimRight(value, "0")
	value = strings.TrimRight(value, ".")
	if value == "-0" {
		return "0"
	}

	return value
}
