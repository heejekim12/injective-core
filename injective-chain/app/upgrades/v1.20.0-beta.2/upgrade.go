//revive:disable-next-line:package-directory-mismatch // Semver upgrade directory names cannot be valid Go package identifiers.
package v1dot20dot0beta2

import (
	"cosmossdk.io/log"
	"cosmossdk.io/store/prefix"
	storetypes "cosmossdk.io/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/InjectiveLabs/injective-core/injective-chain/app/upgrades"
	oracletypes "github.com/InjectiveLabs/injective-core/injective-chain/modules/oracle/types"
	chaintypes "github.com/InjectiveLabs/injective-core/injective-chain/types"
)

const UpgradeVersion = "v1.20.0-beta.2"

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
			"Wipe SedaFast oracle state",
			UpgradeVersion,
			upgrades.TestnetChainID,
			WipeSedaFastOracleState,
		),
	}
}

func WipeSedaFastOracleState(ctx sdk.Context, app upgrades.InjectiveApplication, logger log.Logger) error {
	store := ctx.KVStore(app.GetKey(oracletypes.StoreKey))
	priceStatesDeleted := wipeStorePrefix(store, oracletypes.SedaFastPriceKey)

	historicalPrefix := append(append([]byte{}, oracletypes.SymbolHistoricalPriceRecordsPrefix...), byte(oracletypes.OracleType_SedaFast))
	historicalRecordsDeleted := wipeStorePrefix(store, historicalPrefix)

	logger.Info("Wiped SedaFast oracle state",
		"price_states", priceStatesDeleted,
		"historical_price_records", historicalRecordsDeleted,
	)
	return nil
}

func wipeStorePrefix(store storetypes.KVStore, storePrefix []byte) int {
	prefixStore := prefix.NewStore(store, storePrefix)
	keys := make([][]byte, 0)
	chaintypes.IterateKeysSafe(prefixStore.Iterator(nil, nil), func(key []byte) bool {
		keys = append(keys, key)
		return false
	})

	for _, key := range keys {
		prefixStore.Delete(key)
	}

	return len(keys)
}
