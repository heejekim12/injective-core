//revive:disable-next-line:package-directory-mismatch // Semver upgrade directory names cannot be valid Go package identifiers.
package v1dot20dot0

import (
	"bytes"
	"cmp"
	"encoding/hex"
	"math/big"
	"slices"
	"strings"

	"cosmossdk.io/log"
	"cosmossdk.io/math"
	"cosmossdk.io/store/prefix"
	storetypes "cosmossdk.io/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"

	"github.com/InjectiveLabs/injective-core/injective-chain/app/upgrades"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/types/v2"
	oracletypes "github.com/InjectiveLabs/injective-core/injective-chain/modules/oracle/types"
	peggytypes "github.com/InjectiveLabs/injective-core/injective-chain/modules/peggy/types"
	chaintypes "github.com/InjectiveLabs/injective-core/injective-chain/types"
)

const (
	UpgradeVersion = "v1.20.0"

	// Noble USDC spot market IDs to demolish on mainnet.
	NobleUSDCnbUSDTMarketID    = "0x9c8a91a894f773792b1e8d0b6a8224a6b748753738e9945020ee566266f817be"
	NobleUSDCpeggyUSDCMarketID = "0x33712922dbaf5160f173a65ddfe09b1917b22b2ef5cf9f49264dcd37cfe2a87a"

	// jsonProgramID is the initial program ID for SEDA Fast feeds whose result
	// is a JSON object containing {price:{mantissa,expo},...}.
	jsonProgramID = "ad880830f3d6a46024715640b1ffd80a373f61008003aaefbbdded06cd27e5ad"

	// simpleProgramID is the initial program ID for SEDA Fast feeds whose
	// result is an ASCII decimal string (e.g. "384.48255").
	simpleProgramID = "e1fe25ac6da5502d0ff590e5d9fd6f00cdc230443b1c98b9483efee7f4b6cada"

	// sedaFastPublicKey is the SEC1-compressed secp256k1 public key published
	// by the SEDA Fast service at GET /info.
	sedaFastPublicKey = "025316f89e976d2e41b1437f7a6552a547a1e4900e861ca485c842ed3f016a7824"
)

var (
	legacyLastPriceTimestampsBlobKey = []byte{0x52}
	lastPriceTimestampPrefix         = []byte{0x52}

	nobleUSDCMarketIDsToDemolish = []string{
		NobleUSDCnbUSDTMarketID,
		NobleUSDCpeggyUSDCMarketID,
	}
)

func StoreUpgrades() storetypes.StoreUpgrades {
	return storetypes.StoreUpgrades{
		Added:   nil,
		Renamed: nil,
		Deleted: []string{"hyperlane", "warp"},
	}
}

func UpgradeSteps() []*upgrades.UpgradeHandlerStep {
	migrateStep := "Migrate oracle store"
	initPythProStep := "Init PythPro oracle params"
	initSedaFastStep := "Init SedaFast oracle params"
	setINJMinNotionalStep := "Set INJ denom min notional"
	return []*upgrades.UpgradeHandlerStep{
		upgrades.NewUpgradeHandlerStep(
			migrateStep,
			UpgradeVersion,
			upgrades.MainnetChainID,
			MigrateOracleStoreV120,
		),
		upgrades.NewUpgradeHandlerStep(
			initPythProStep,
			UpgradeVersion,
			upgrades.MainnetChainID,
			SetPythProOracleParams,
		),
		upgrades.NewUpgradeHandlerStep(
			"Migrate cross-margin params to defaults",
			UpgradeVersion,
			upgrades.MainnetChainID,
			MigrateCrossMarginParams,
		),
		upgrades.NewUpgradeHandlerStep(
			"Backfill cross-margin active market indexes",
			UpgradeVersion,
			upgrades.MainnetChainID,
			BackfillCrossMarginIndexes,
		),
		upgrades.NewUpgradeHandlerStep(
			setINJMinNotionalStep,
			UpgradeVersion,
			upgrades.MainnetChainID,
			SetINJDenomMinNotional,
		),
		upgrades.NewUpgradeHandlerStep(
			"Migrate old fee index entries in peggy",
			UpgradeVersion,
			upgrades.MainnetChainID,
			MigratePeggySecondaryFeeIndex,
		),
		upgrades.NewUpgradeHandlerStep(
			initSedaFastStep,
			UpgradeVersion,
			upgrades.MainnetChainID,
			SetSedaFastOracleParams,
		),
		upgrades.NewUpgradeHandlerStep(
			"Demolish Noble USDC spot markets",
			UpgradeVersion,
			upgrades.MainnetChainID,
			DemolishNobleUSDCSpotMarkets,
		),
	}
}

// SetSedaFastOracleParams initializes SEDA Fast oracle params as part of the
// v1.20.0 upgrade, including the SEDA Fast service public key so that
// MsgRelaySedaFastPrices is active immediately after the upgrade.
func SetSedaFastOracleParams(ctx sdk.Context, app upgrades.InjectiveApplication, logger log.Logger) error {
	oracleKeeper := app.GetOracleKeeper()
	params := oracleKeeper.GetParams(ctx)
	pubKey, _ := hex.DecodeString(sedaFastPublicKey) // 33-byte compressed key — always valid
	params.SedaFastParams = oracletypes.SedaFastParams{
		PublicKey:        pubKey,
		JsonProgramIds:   []string{jsonProgramID},
		SimpleProgramIds: []string{simpleProgramID},
	}
	oracleKeeper.SetParams(ctx, params)
	logger.Info("SedaFast oracle params initialized",
		"public_key", sedaFastPublicKey,
		"json_program_ids", params.SedaFastParams.JsonProgramIds,
		"simple_program_ids", params.SedaFastParams.SimpleProgramIds,
	)
	return nil
}

// SetPythProOracleParams initializes PythPro-related oracle module params after the v1.20.0 upgrade.
func SetPythProOracleParams(ctx sdk.Context, app upgrades.InjectiveApplication, logger log.Logger) error {
	oracleKeeper := app.GetOracleKeeper()
	params := oracleKeeper.GetParams(ctx)
	params.PythProVerifierContract = "0xACeA761c27A909d4D3895128EBe6370FDE2dF481"
	params.PythProVerificationGasLimit = 500_000
	params.PythProVerificationFee = 1
	oracleKeeper.SetParams(ctx, params)
	logger.Info("PythPro oracle params initialized",
		"pyth_pro_verifier_contract", params.PythProVerifierContract,
		"pyth_pro_verification_gas_limit", params.PythProVerificationGasLimit,
		"pyth_pro_verification_fee", params.PythProVerificationFee,
	)
	return nil
}

func MigrateOracleStoreV120(ctx sdk.Context, app upgrades.InjectiveApplication, _ log.Logger) error {
	migrateLegacyTimestampBlob(ctx, app)
	migrateProviderPriceEncoding(ctx, app)
	migrateHistoricalPriceRecordKeys(ctx, app)
	deleteLastPriceTimestampIndex(ctx, app)
	migratePythPriceStates(ctx, app)
	migrateChainlinkDataStreamsPriceStates(ctx, app)
	migrateStorkPriceStates(ctx, app)
	migrateCoinbasePriceStates(ctx, app)
	return nil
}

func migrateLegacyTimestampBlob(ctx sdk.Context, app upgrades.InjectiveApplication) {
	store := ctx.KVStore(app.GetKey(oracletypes.StoreKey))
	store.Delete(legacyLastPriceTimestampsBlobKey)
}

func migrateProviderPriceEncoding(ctx sdk.Context, app upgrades.InjectiveApplication) {
	store := ctx.KVStore(app.GetKey(oracletypes.StoreKey))
	cdc := app.AppCodec()
	infos := app.GetOracleKeeper().GetAllProviderInfos(ctx)
	for _, info := range infos {
		if info == nil {
			continue
		}
		provider := info.Provider
		pstore := prefix.NewStore(store, oracletypes.GetProviderPricePrefix(provider))
		var updates [][2][]byte
		chaintypes.IterateSafe(pstore.Iterator(nil, nil), func(k, v []byte) bool {
			sym := string(k)
			var pps oracletypes.ProviderPriceState
			if err := cdc.Unmarshal(v, &pps); err != nil {
				return false
			}
			if pps.Symbol == sym && pps.State != nil {
				updates = append(updates, [2][]byte{bytes.Clone(k), cdc.MustMarshal(pps.State)})
			}
			return false
		})
		for _, u := range updates {
			pstore.Set(u[0], u[1])
		}
	}
}

func migrateHistoricalPriceRecordKeys(ctx sdk.Context, app upgrades.InjectiveApplication) {
	store := ctx.KVStore(app.GetKey(oracletypes.StoreKey))
	cdc := app.AppCodec()
	histStore := prefix.NewStore(store, oracletypes.SymbolHistoricalPriceRecordsPrefix)

	var keysToDelete [][]byte
	var newWrites [][2][]byte

	chaintypes.IterateSafe(histStore.Iterator(nil, nil), func(k, v []byte) bool {
		fullKey := append(append([]byte{}, oracletypes.SymbolHistoricalPriceRecordsPrefix...), k...)
		if _, _, _, ok := oracletypes.ParseSymbolHistoricalPriceRecordKey(fullKey); ok {
			return false
		}
		var pr oracletypes.PriceRecords
		if err := cdc.Unmarshal(v, &pr); err != nil || len(pr.LatestPriceRecords) == 0 {
			return false
		}
		ot, sym, ok := parseOldHistoricalKeySuffix(k)
		if !ok {
			return false
		}
		for _, rec := range pr.LatestPriceRecords {
			if rec == nil {
				continue
			}
			nk := oracletypes.GetSymbolHistoricalPriceRecordKey(ot, sym, rec.Timestamp)
			bz, err := rec.Price.Marshal()
			if err != nil {
				continue
			}
			newWrites = append(newWrites, [2][]byte{nk, bz})
		}
		keysToDelete = append(keysToDelete, append([]byte{}, fullKey...))
		return false
	})

	for _, w := range newWrites {
		store.Set(w[0], w[1])
	}
	for _, dk := range keysToDelete {
		store.Delete(dk)
	}
}

func deleteLastPriceTimestampIndex(ctx sdk.Context, app upgrades.InjectiveApplication) {
	store := ctx.KVStore(app.GetKey(oracletypes.StoreKey))
	tsStore := prefix.NewStore(store, lastPriceTimestampPrefix)
	var keys [][]byte
	chaintypes.IterateKeysSafe(tsStore.Iterator(nil, nil), func(k []byte) bool {
		fullKey := append(append([]byte{}, lastPriceTimestampPrefix...), k...)
		keys = append(keys, fullKey)
		return false
	})
	for _, key := range keys {
		store.Delete(key)
	}
}

func migratePythPriceStates(ctx sdk.Context, app upgrades.InjectiveApplication) {
	store := ctx.KVStore(app.GetKey(oracletypes.StoreKey))
	cdc := app.AppCodec()
	pythStore := prefix.NewStore(store, oracletypes.PythPriceKey)
	var updates [][2][]byte
	chaintypes.IterateSafe(pythStore.Iterator(nil, nil), func(iterKey, iterVal []byte) bool {
		var ps oracletypes.PythPriceState
		if err := cdc.Unmarshal(iterVal, &ps); err != nil {
			return false
		}
		if ps.PriceId == "" {
			return false
		}
		ps.PriceId = ""
		fullKey := append(append([]byte{}, oracletypes.PythPriceKey...), iterKey...)
		updates = append(updates, [2][]byte{fullKey, cdc.MustMarshal(&ps)})
		return false
	})
	for _, u := range updates {
		store.Set(u[0], u[1])
	}
}

func migrateChainlinkDataStreamsPriceStates(ctx sdk.Context, app upgrades.InjectiveApplication) {
	store := ctx.KVStore(app.GetKey(oracletypes.StoreKey))
	cdc := app.AppCodec()
	cdsStore := prefix.NewStore(store, oracletypes.ChainlinkDataStreamsPriceKey)
	var snapshots [][2][]byte
	chaintypes.IterateSafe(cdsStore.Iterator(nil, nil), func(iterKey, iterVal []byte) bool {
		snapshots = append(snapshots, [2][]byte{bytes.Clone(iterKey), bytes.Clone(iterVal)})
		return false
	})
	var toDelete [][]byte
	for _, snap := range snapshots {
		iterKey, iterVal := snap[0], snap[1]
		var ps oracletypes.ChainlinkDataStreamsPriceState
		if err := cdc.Unmarshal(iterVal, &ps); err != nil {
			continue
		}
		if ps.FeedId == "" && len(iterKey) == 32 {
			continue
		}
		feedID := ps.FeedId
		if feedID == "" {
			feedID = string(iterKey)
		}
		ps.FeedId = ""
		newKey := oracletypes.GetChainlinkDataStreamsPriceStoreKey(feedID)
		newBz := cdc.MustMarshal(&ps)
		oldFullKey := append(append([]byte{}, oracletypes.ChainlinkDataStreamsPriceKey...), iterKey...)
		store.Set(newKey, newBz)
		if !bytes.Equal(oldFullKey, newKey) {
			toDelete = append(toDelete, oldFullKey)
		}
	}
	for _, k := range toDelete {
		store.Delete(k)
	}
}

func migrateStorkPriceStates(ctx sdk.Context, app upgrades.InjectiveApplication) {
	store := ctx.KVStore(app.GetKey(oracletypes.StoreKey))
	cdc := app.AppCodec()
	storkStore := prefix.NewStore(store, oracletypes.StorkPriceKey)
	var updates [][2][]byte
	chaintypes.IterateSafe(storkStore.Iterator(nil, nil), func(iterKey, iterVal []byte) bool {
		var ps oracletypes.StorkPriceState
		if err := cdc.Unmarshal(iterVal, &ps); err != nil {
			return false
		}
		if ps.Symbol == "" {
			return false
		}
		ps.Symbol = ""
		fullKey := append(append([]byte{}, oracletypes.StorkPriceKey...), iterKey...)
		updates = append(updates, [2][]byte{fullKey, cdc.MustMarshal(&ps)})
		return false
	})
	for _, u := range updates {
		store.Set(u[0], u[1])
	}
}

func migrateCoinbasePriceStates(ctx sdk.Context, app upgrades.InjectiveApplication) {
	store := ctx.KVStore(app.GetKey(oracletypes.StoreKey))
	cdc := app.AppCodec()
	coinbaseStore := prefix.NewStore(store, oracletypes.CoinbasePriceKey)
	var updates [][2][]byte
	chaintypes.IterateSafe(coinbaseStore.Iterator(nil, nil), func(iterKey, iterVal []byte) bool {
		var ps oracletypes.CoinbasePriceState
		if err := cdc.Unmarshal(iterVal, &ps); err != nil {
			return false
		}
		if ps.Key == "" {
			return false
		}
		ps.Kind, ps.Key, ps.Timestamp = "", "", 0
		fullKey := append(append([]byte{}, oracletypes.CoinbasePriceKey...), iterKey...)
		updates = append(updates, [2][]byte{fullKey, cdc.MustMarshal(&ps)})
		return false
	})
	for _, u := range updates {
		store.Set(u[0], u[1])
	}
}

func parseOldHistoricalKeySuffix(suffix []byte) (oracletypes.OracleType, string, bool) {
	s := string(suffix)
	type cand struct {
		ot   oracletypes.OracleType
		name string
	}
	cands := make([]cand, 0, 13)
	for i := 1; i <= 13; i++ {
		ot := oracletypes.OracleType(i)
		cands = append(cands, cand{ot: ot, name: ot.String()})
	}
	slices.SortFunc(cands, func(a, b cand) int {
		return cmp.Compare(len(b.name), len(a.name))
	})
	for _, c := range cands {
		p := c.name + "_"
		if strings.HasPrefix(s, p) {
			return c.ot, s[len(p):], true
		}
	}
	return 0, "", false
}

// MigrateCrossMarginParams populates the new cross-margin param fields with their DefaultParams()
// values.
func MigrateCrossMarginParams(ctx sdk.Context, app upgrades.InjectiveApplication, logger log.Logger) error {
	exchangeKeeper := app.GetExchangeKeeper()
	params := exchangeKeeper.GetParams(ctx)
	cm := &params.CrossMarginParams
	defaultCM := v2.DefaultCrossMarginParams()

	if cm.PositiveUpnlHaircutRate.IsNil() {
		cm.PositiveUpnlHaircutRate = defaultCM.PositiveUpnlHaircutRate
	}
	if cm.FeesBuffer.IsNil() {
		cm.FeesBuffer = defaultCM.FeesBuffer
	}
	if cm.EnabledQuoteDenoms == nil {
		cm.EnabledQuoteDenoms = defaultCM.EnabledQuoteDenoms
	}
	// PerpetualEnabled: proto3 zero is false, but default is true.
	// We cannot distinguish "explicitly set to false" from "unset" via proto3 alone,
	// so we unconditionally set to the default. This is safe because cross-margin
	// is gated behind EnabledQuoteDenoms (empty = disabled).
	cm.PerpetualEnabled = defaultCM.PerpetualEnabled
	// ExpiryEnabled: same proto3-zero ambiguity as PerpetualEnabled. Set to default.
	cm.ExpiryEnabled = defaultCM.ExpiryEnabled
	if cm.MaxActiveDerivativeMarketsPerPool == 0 {
		cm.MaxActiveDerivativeMarketsPerPool = defaultCM.MaxActiveDerivativeMarketsPerPool
	}
	// EmergencyPaused defaults to false, matching proto3 zero — no action needed.

	exchangeKeeper.SetParams(ctx, params)
	logger.Info("Migrated cross-margin params to defaults",
		"haircut", cm.PositiveUpnlHaircutRate.String(),
		"fees_buffer", cm.FeesBuffer.String(),
		"perpetual_enabled", cm.PerpetualEnabled,
		"max_active_markets", cm.MaxActiveDerivativeMarketsPerPool,
	)
	return nil
}

// BackfillCrossMarginIndexes populates the ActiveDerivativeMarketsBySubaccount and
// ActiveDerivativeOrderMarketsBySubaccount indexes for all existing derivative positions
// and orders. This is required for cross-margin eligibility checks and snapshot building
// to work correctly for positions/orders that existed before this upgrade.
func BackfillCrossMarginIndexes(ctx sdk.Context, app upgrades.InjectiveApplication, logger log.Logger) error {
	exchangeKeeper := app.GetExchangeKeeper()

	logger.Info("Starting backfill of ActiveDerivativeMarketsBySubaccount index (positions)")
	exchangeKeeper.BackfillActiveDerivativeMarketIndexes(ctx)
	logger.Info("Completed backfill of ActiveDerivativeMarketsBySubaccount index")

	logger.Info("Starting backfill of ActiveDerivativeOrderMarketsBySubaccount index (orders)")
	exchangeKeeper.BackfillActiveDerivativeOrderMarketIndexes(ctx)
	logger.Info("Completed backfill of ActiveDerivativeOrderMarketsBySubaccount index")

	return nil
}

// DemolishNobleUSDCSpotMarkets demolishes Noble USDC spot markets on mainnet. It cancels all
// resting limit orders and moves each market to Demolished status, matching the canonical on-chain
// demolish flow used by ExecuteSpotMarketParamUpdateProposal. Markets that are already Demolished or
// not present are silently skipped so the step is idempotent.
func DemolishNobleUSDCSpotMarkets(ctx sdk.Context, app upgrades.InjectiveApplication, logger log.Logger) error {
	k := app.GetExchangeKeeper()
	for _, idHex := range nobleUSDCMarketIDsToDemolish {
		marketID := common.HexToHash(idHex)
		market := k.GetSpotMarketByID(ctx, marketID)
		if market == nil {
			logger.Info("Noble USDC spot market not found, skipping", "market_id", idHex)
			continue
		}
		if market.Status == v2.MarketStatus_Demolished {
			logger.Info("Noble USDC spot market already demolished, skipping", "market_id", idHex)
			continue
		}
		k.CancelAllRestingLimitOrdersFromSpotMarket(ctx, market, marketID)
		if _, err := k.SetSpotMarketStatus(ctx, marketID, v2.MarketStatus_Demolished); err != nil {
			logger.Error("Failed to demolish Noble USDC spot market", "market_id", idHex, "error", err)
			continue
		}
		logger.Info("Demolished Noble USDC spot market", "market_id", idHex, "ticker", market.Ticker)
	}
	return nil
}

func SetINJDenomMinNotional(ctx sdk.Context, app upgrades.InjectiveApplication, logger log.Logger) error {
	minNotional := math.LegacyMustNewDecFromStr("0.1")
	app.GetExchangeKeeper().SetMinNotionalForDenom(ctx, chaintypes.InjectiveCoin, minNotional)
	logger.Info("Set INJ denom min notional",
		"denom", chaintypes.InjectiveCoin,
		"min_notional", minNotional.String(),
	)
	return nil
}

func MigratePeggySecondaryFeeIndex(ctx sdk.Context, app upgrades.InjectiveApplication, _ log.Logger) error {
	var (
		pk          = app.GetPeggyKeeper()
		cdc         = app.AppCodec()
		oldKeys     = make([][]byte, 0)
		peggyStore  = ctx.KVStore(app.GetKey(peggytypes.StoreKey))
		oldFeeStore = prefix.NewStore(peggyStore, peggytypes.SecondIndexOutgoingTXFeeKey)
	)

	// migrate each fee entry to the new store
	chaintypes.IterateSafe(oldFeeStore.Iterator(nil, nil), func(key, value []byte) bool {
		token := common.BytesToAddress(key[:20])
		feeAmount := math.NewIntFromBigInt(big.NewInt(0).SetBytes(key[20:52]))
		fee := &peggytypes.ERC20Token{Amount: feeAmount}

		var txIDs peggytypes.IDSet
		cdc.MustUnmarshal(value, &txIDs)
		for _, txID := range txIDs.Ids {
			pk.SetOutgoingTxFee(ctx, token, fee, txID)
		}

		oldKeys = append(oldKeys, bytes.Clone(key))

		return false
	})

	// clean the old store
	for _, key := range oldKeys {
		oldFeeStore.Delete(key)
	}

	return nil
}
