package keeper

import (
	"cosmossdk.io/store/prefix"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/InjectiveLabs/injective-core/injective-chain/modules/oracle/types"
	chaintypes "github.com/InjectiveLabs/injective-core/injective-chain/types"
)

// SetSedaFastPriceState persists a SedaFast price state keyed by the raw bytes
// of the composite feedID. FeedId is encoded in the store key and stripped from
// the stored value to avoid duplication.
func (k *Keeper) SetSedaFastPriceState(ctx sdk.Context, priceState *types.SedaFastPriceState) error {
	defer k.Meter(ctx).FuncTiming(&ctx, "SetSedaFastPriceState")()

	key, err := types.GetSedaFastPriceStoreKey(priceState.FeedId)
	if err != nil {
		return err
	}
	toStore := *priceState
	toStore.FeedId = ""
	bz := k.cdc.MustMarshal(&toStore)
	k.getStore(ctx).Set(key, bz)

	k.AppendPriceRecord(ctx, types.OracleType_SedaFast, priceState.FeedId,
		&types.PriceRecord{
			Timestamp: priceState.PriceState.Timestamp,
			Price:     priceState.PriceState.Price,
		},
	)
	return nil
}

// GetSedaFastPriceState retrieves the SedaFast price state for the given feed
// ID.
func (k *Keeper) GetSedaFastPriceState(ctx sdk.Context, feedID string) *types.SedaFastPriceState {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetSedaFastPriceState")()

	key, err := types.GetSedaFastPriceStoreKey(feedID)
	if err != nil {
		return nil
	}
	bz := k.getStore(ctx).Get(key)
	if bz == nil {
		return nil
	}

	var priceState types.SedaFastPriceState
	k.cdc.MustUnmarshal(bz, &priceState)
	priceState.FeedId = feedID
	return &priceState
}

// GetAllSedaFastPriceStates returns all persisted SedaFast price states.
func (k *Keeper) GetAllSedaFastPriceStates(ctx sdk.Context) []*types.SedaFastPriceState {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetAllSedaFastPriceStates")()

	priceStates := make([]*types.SedaFastPriceState, 0)
	store := ctx.KVStore(k.storeKey)
	priceStore := prefix.NewStore(store, types.SedaFastPriceKey)

	chaintypes.IterateSafe(priceStore.Iterator(nil, nil), func(iterKey, iterVal []byte) bool {
		var priceState types.SedaFastPriceState
		k.cdc.MustUnmarshal(iterVal, &priceState)
		priceState.FeedId = types.GetSedaFastFeedIDFromIterKey(iterKey)
		priceStates = append(priceStates, &priceState)
		return false
	})

	return priceStates
}

// EmitSedaFastPriceUpdate emits the generic EventOraclePriceUpdate for a
// SEDA Fast feed. The Id is the composite SedaFast feed ID.
func (k *Keeper) EmitSedaFastPriceUpdate(ctx sdk.Context, feedID string, priceState *types.PriceState) {
	defer k.Meter(ctx).FuncTiming(&ctx, "EmitSedaFastPriceUpdate")()

	// nolint:errcheck //ignored on purpose
	ctx.EventManager().EmitTypedEvent(&types.EventOraclePriceUpdate{
		OracleType: types.OracleType_SedaFast,
		Id:         feedID,
		PriceState: *priceState,
	})
}
