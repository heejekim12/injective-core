package keeper

import (
	"context"

	"cosmossdk.io/log"
	storetypes "cosmossdk.io/store/types"
	"github.com/InjectiveLabs/metrics/v2"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authkeeper "github.com/cosmos/cosmos-sdk/x/auth/keeper"

	"github.com/InjectiveLabs/injective-core/injective-chain/modules/oracle/types"
)

// Keeper defines a module interface that facilitates the getting and setting of oracle reference data
type Keeper struct {
	types.QueryServer

	storeKey storetypes.StoreKey
	cdc      codec.BinaryCodec
	memKey   storetypes.StoreKey

	accountKeeper authkeeper.AccountKeeper
	bankKeeper    types.BankKeeper
	evmKeeper     types.EVMKeeper

	authority string
	meter     metrics.Meter
}

// NewKeeper creates new instances of the oracle Keeper
func NewKeeper(
	cdc codec.BinaryCodec,
	storeKey storetypes.StoreKey,
	memKey storetypes.StoreKey,
	ak authkeeper.AccountKeeper,
	bk types.BankKeeper,
	evmKeeper types.EVMKeeper,
	authority string,
) Keeper {
	return Keeper{
		storeKey:      storeKey,
		memKey:        memKey,
		cdc:           cdc,
		accountKeeper: ak,
		bankKeeper:    bk,
		evmKeeper:     evmKeeper,
		authority:     authority,
	}
}

func (k *Keeper) Logger(ctx sdk.Context) log.Logger {
	return ctx.Logger().With("module", types.ModuleName)
}

func (k *Keeper) Meter(ctx context.Context) metrics.Meter {
	if k.meter == nil {
		k.meter = sdk.UnwrapSDKContext(ctx).Meter().SubMeter(types.ModuleName, metrics.Tag("svc", types.ModuleName))
	}

	return k.meter
}

func (k *Keeper) getStore(ctx sdk.Context) storetypes.KVStore {
	return ctx.KVStore(k.storeKey)
}
