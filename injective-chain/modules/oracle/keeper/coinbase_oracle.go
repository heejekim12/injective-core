package keeper

import (
	"bytes"
	"encoding/binary"

	"cosmossdk.io/errors"
	"cosmossdk.io/store/prefix"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/InjectiveLabs/injective-core/injective-chain/modules/oracle/types"
	chaintypes "github.com/InjectiveLabs/injective-core/injective-chain/types"
)

func (k *Keeper) GetCoinbasePriceState(ctx sdk.Context, key string) *types.CoinbasePriceState {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetCoinbasePriceState")()

	return k.getLastCoinbasePriceState(ctx, key)
}

// HasCoinbasePriceState checks whether a price state exists for a given coinbase price key.
func (k *Keeper) HasCoinbasePriceState(ctx sdk.Context, key string) bool {
	defer k.Meter(ctx).FuncTiming(&ctx, "HasCoinbasePriceState")()

	store := ctx.KVStore(k.storeKey)
	iterationKey := types.GetCoinbasePriceStoreIterationKey(key)
	prefixStore := prefix.NewStore(store, iterationKey)
	var found bool
	chaintypes.IterateSafe(prefixStore.Iterator(nil, nil), func(iterKey, _ []byte) bool {
		if len(iterKey) != 8 {
			return false
		}
		found = true
		return true
	})
	return found
}
// GetCoinbasePriceStates fetches the coinbase price states for a given coinbase price key.
func (k *Keeper) GetCoinbasePriceStates(ctx sdk.Context, key string) []*types.CoinbasePriceState {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetCoinbasePriceStates")()

	priceDatas := make([]*types.CoinbasePriceState, 0)
	store := ctx.KVStore(k.storeKey)

	coinbasePriceDataStore := prefix.NewStore(store, types.GetCoinbasePriceStoreIterationKey(key))

	chaintypes.IterateSafe(coinbasePriceDataStore.ReverseIterator(nil, nil), func(iterKey, bz []byte) bool {
		if len(iterKey) != 8 {
			return false
		}
		pd := new(types.CoinbasePriceState)
		k.cdc.MustUnmarshal(bz, pd)
		pd.Kind, pd.Key, pd.Timestamp = "prices", key, binary.BigEndian.Uint64(iterKey)
		priceDatas = append(priceDatas, pd)
		return false
	})

	return priceDatas
}

// SetCoinbasePriceState stores a given coinbase price state.
func (k *Keeper) SetCoinbasePriceState(ctx sdk.Context, priceData *types.CoinbasePriceState) error {
	defer k.Meter(ctx).FuncTiming(&ctx, "SetCoinbasePriceState")()

	priceFeedInfoKey := types.GetCoinbasePriceStoreKey(priceData.Key, priceData.Timestamp)

	lastPriceData := k.getLastCoinbasePriceState(ctx, priceData.Key)
	if lastPriceData != nil {
		if lastPriceData.Timestamp == priceData.Timestamp {
			return nil
		} else if lastPriceData.Timestamp > priceData.Timestamp {
			return errors.Wrapf(types.ErrBadCoinbaseMessageTimestamp, "existing price data timestamp is %d but got %d", lastPriceData.Timestamp, priceData.Timestamp)
		}
	}

	price := priceData.GetDecPrice()

	toStore := *priceData
	toStore.Kind, toStore.Key, toStore.Timestamp = "", "", 0
	bz := k.cdc.MustMarshal(&toStore)
	k.getStore(ctx).Set(priceFeedInfoKey, bz)

	k.AppendPriceRecord(ctx, types.OracleType_Coinbase, priceData.Key, &types.PriceRecord{
		Timestamp: priceData.PriceState.Timestamp,
		Price:     price,
	})

	// remove old coinbase price states outside of TWAP window when set price data
	k.PruneOldCoinbasePriceStates(ctx, priceData.Key)

	// nolint:errcheck //ignored on purpose
	ctx.EventManager().EmitTypedEvent(&types.SetCoinbasePriceEvent{
		Symbol:    priceData.Key,
		Price:     price,
		Timestamp: priceData.Timestamp,
	})
	return nil
}

// GetAllCoinbasePriceStates fetches all coinbase price states.
func (k *Keeper) GetAllCoinbasePriceStates(ctx sdk.Context) []*types.CoinbasePriceState {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetAllCoinbasePriceStates")()

	priceDatas := make([]*types.CoinbasePriceState, 0)
	store := ctx.KVStore(k.storeKey)

	coinbasePriceDataStore := prefix.NewStore(store, types.CoinbasePriceKey)

	chaintypes.IterateSafe(coinbasePriceDataStore.Iterator(nil, nil), func(iterKey, bz []byte) bool {
		sym, ts, ok := types.ParseCoinbasePriceStoreIterKey(iterKey)
		if !ok {
			return false
		}
		pd := new(types.CoinbasePriceState)
		k.cdc.MustUnmarshal(bz, pd)
		pd.Kind, pd.Key, pd.Timestamp = "prices", sym, ts
		priceDatas = append(priceDatas, pd)
		return false
	})

	return priceDatas
}

func (k *Keeper) PruneOldCoinbasePriceStates(ctx sdk.Context, key string) {
	defer k.Meter(ctx).FuncTiming(&ctx, "PruneOldCoinbasePriceStates")()

	now := ctx.BlockTime().Unix()
	windowStart := sdk.Uint64ToBigEndian(uint64(now - types.TwapWindow))

	store := ctx.KVStore(k.storeKey)
	coinbasePriceDataStore := prefix.NewStore(store, types.GetCoinbasePriceStoreIterationKey(key))

	// Find the most recent price sample strictly before the TWAP window start.
	// We keep this entry so the TWAP calculation can correctly weight the
	// interval from windowStart to the first in-window sample.
	var anchorKey []byte
	chaintypes.IterateKeysSafe(coinbasePriceDataStore.ReverseIterator(nil, windowStart), func(iterKey []byte) bool {
		anchorKey = bytes.Clone(iterKey)
		return true
	})
	if anchorKey == nil {
		return
	}

	var keysToDelete [][]byte
	chaintypes.IterateKeysSafe(coinbasePriceDataStore.Iterator(nil, anchorKey), func(iterKey []byte) bool {
		keysToDelete = append(keysToDelete, bytes.Clone(iterKey))
		return false
	})
	for _, dk := range keysToDelete {
		coinbasePriceDataStore.Delete(dk)
	}
}

// getLastCoinbasePriceState fetches the last coinbase price state for a given coinbase price key.
func (k *Keeper) getLastCoinbasePriceState(ctx sdk.Context, key string) *types.CoinbasePriceState {
	defer k.Meter(ctx).FuncTiming(&ctx, "getLastCoinbasePriceState")()

	iterationKey := types.GetCoinbasePriceStoreIterationKey(key)
	prefixStore := prefix.NewStore(k.getStore(ctx), iterationKey)

	var priceFeedInfo *types.CoinbasePriceState
	chaintypes.IterateSafe(prefixStore.ReverseIterator(nil, nil), func(iterKey, iterVal []byte) bool {
		if len(iterKey) != 8 {
			return false
		}
		pd := new(types.CoinbasePriceState)
		k.cdc.MustUnmarshal(iterVal, pd)
		pd.Kind, pd.Key, pd.Timestamp = "prices", key, binary.BigEndian.Uint64(iterKey)
		priceFeedInfo = pd
		return true
	})
	return priceFeedInfo
}
