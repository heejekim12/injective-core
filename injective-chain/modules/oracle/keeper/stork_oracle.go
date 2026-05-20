package keeper

import (
	"cosmossdk.io/store/prefix"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"

	"github.com/InjectiveLabs/injective-core/injective-chain/modules/oracle/assistant/stork"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/oracle/types"
	chaintypes "github.com/InjectiveLabs/injective-core/injective-chain/types"
)

// EmitStorkPricesUpdated emits a typed event for updated Stork price states.
func (k *Keeper) EmitStorkPricesUpdated(ctx sdk.Context, states []*types.StorkPriceState) {
	if len(states) == 0 {
		return
	}
	defer k.Meter(ctx).FuncTiming(&ctx, "EmitStorkPricesUpdated")()
	// nolint:errcheck //ignored on purpose
	ctx.EventManager().EmitTypedEvent(&types.EventSetStorkPrices{Prices: states})
}

// SetStorkPriceState stores a given stork price state.
func (k *Keeper) SetStorkPriceState(ctx sdk.Context, priceData *types.StorkPriceState) {
	defer k.Meter(ctx).FuncTiming(&ctx, "SetStorkPriceState")()

	priceKey := types.GetStorkPriceStoreKey(priceData.Symbol)
	toStore := *priceData
	toStore.Symbol = ""
	bz := k.cdc.MustMarshal(&toStore)

	k.getStore(ctx).Set(priceKey, bz)

	k.AppendPriceRecord(ctx, types.OracleType_Stork, priceData.Symbol, &types.PriceRecord{
		Timestamp: priceData.PriceState.Timestamp,
		Price:     priceData.PriceState.Price,
	})
}

func (k *Keeper) GetStorkPriceState(ctx sdk.Context, symbol string) *types.StorkPriceState {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetStorkPriceState")()

	var priceState types.StorkPriceState
	bz := k.getStore(ctx).Get(types.GetStorkPriceStoreKey(symbol))
	if bz == nil {
		return nil
	}

	k.cdc.MustUnmarshal(bz, &priceState)
	priceState.Symbol = symbol
	return &priceState
}

// GetAllStorkPriceStates fetches all stork price states.
func (k *Keeper) GetAllStorkPriceStates(ctx sdk.Context) []*types.StorkPriceState {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetAllStorkPriceStates")()

	priceStates := make([]*types.StorkPriceState, 0)
	store := ctx.KVStore(k.storeKey)

	priceStore := prefix.NewStore(store, types.StorkPriceKey)

	chaintypes.IterateSafe(priceStore.Iterator(nil, nil), func(iterKey, iterVal []byte) bool {
		pd := new(types.StorkPriceState)
		k.cdc.MustUnmarshal(iterVal, pd)
		pd.Symbol = string(iterKey)
		priceStates = append(priceStates, pd)
		return false
	})

	return priceStates
}

// SetStorkPublisher stores a given stork publisher address
func (k *Keeper) SetStorkPublisher(ctx sdk.Context, address string) {
	defer k.Meter(ctx).FuncTiming(&ctx, "SetStorkPublisher")()

	store := ctx.KVStore(k.storeKey)

	storkPublisherDataStore := prefix.NewStore(store, types.StorkPublisherKey)
	storkPublisherDataStore.Set(common.HexToAddress(address).Bytes(), []byte(""))
}

// DeleteStorkPublisher delete a given stork publisher address
func (k *Keeper) DeleteStorkPublisher(ctx sdk.Context, address string) {
	defer k.Meter(ctx).FuncTiming(&ctx, "DeleteStorkPublisher")()

	store := ctx.KVStore(k.storeKey)

	storkPublisherStore := prefix.NewStore(store, types.StorkPublisherKey)
	storkPublisherStore.Delete(common.HexToAddress(address).Bytes())
}

// GetAllStorkPublishers fetches all stork publisher addresses.
func (k *Keeper) GetAllStorkPublishers(ctx sdk.Context) []string {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetAllStorkPublishers")()

	publishers := make([]string, 0)
	store := ctx.KVStore(k.storeKey)

	publisherStore := prefix.NewStore(store, types.StorkPublisherKey)

	chaintypes.IterateKeysSafe(publisherStore.Iterator(nil, nil), func(iterKey []byte) bool {
		publishers = append(publishers, common.BytesToAddress(iterKey).Hex())
		return false
	})

	return publishers
}

func (k *Keeper) IsStorkPublisher(ctx sdk.Context, address string) bool {
	defer k.Meter(ctx).FuncTiming(&ctx, "IsStorkPublisher")()

	store := ctx.KVStore(k.storeKey)
	storkPublisherStore := prefix.NewStore(store, types.StorkPublisherKey)

	return storkPublisherStore.Has(common.HexToAddress(address).Bytes())
}

func (k *Keeper) ProcessStorkAssetPairsData(ctx sdk.Context, assetPairs []*types.AssetPair) {
	defer k.Meter(ctx).FuncTiming(&ctx, "ProcessStorkAssetPairsData")()

	stork.NewAssistant(k).ProcessAssetPairsData(ctx, assetPairs)
}
