package keeper

import (
	"cosmossdk.io/store/prefix"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/InjectiveLabs/injective-core/injective-chain/modules/oracle/types"
	chaintypes "github.com/InjectiveLabs/injective-core/injective-chain/types"
)

// SetSedaFastPriceState persists a SedaFast price state keyed by
// keccak256(feedID). Unlike PythPro (where feedID is a fixed-width uint32 that
// can be decoded from the key), the SEDA Fast feedID is a variable-length hex
// string. Therefore the feedID is kept in the stored proto value so that
// GetAllSedaFastPriceStates can restore it without an external index.
func (k *Keeper) SetSedaFastPriceState(ctx sdk.Context, priceState *types.SedaFastPriceState) {
	defer k.Meter(ctx).FuncTiming(&ctx, "SetSedaFastPriceState")()

	key := types.GetSedaFastPriceStoreKey(priceState.FeedId)
	bz := k.cdc.MustMarshal(priceState)
	k.getStore(ctx).Set(key, bz)

	k.AppendPriceRecord(ctx, types.OracleType_SedaFast, priceState.FeedId,
		&types.PriceRecord{
			Timestamp: priceState.PriceState.Timestamp,
			Price:     priceState.PriceState.Price,
		},
	)
}

// GetSedaFastPriceState retrieves the SedaFast price state for the given feed
// ID.
func (k *Keeper) GetSedaFastPriceState(ctx sdk.Context, feedID string) *types.SedaFastPriceState {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetSedaFastPriceState")()

	bz := k.getStore(ctx).Get(types.GetSedaFastPriceStoreKey(feedID))
	if bz == nil {
		return nil
	}

	var priceState types.SedaFastPriceState
	k.cdc.MustUnmarshal(bz, &priceState)
	return &priceState
}

// GetAllSedaFastPriceStates returns all persisted SedaFast price states,
// including the feedID field which is stored in the proto value.
func (k *Keeper) GetAllSedaFastPriceStates(ctx sdk.Context) []*types.SedaFastPriceState {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetAllSedaFastPriceStates")()

	priceStates := make([]*types.SedaFastPriceState, 0)
	store := ctx.KVStore(k.storeKey)
	priceStore := prefix.NewStore(store, types.SedaFastPriceKey)

	chaintypes.IterateSafe(priceStore.Iterator(nil, nil), func(_, iterVal []byte) bool {
		var priceState types.SedaFastPriceState
		k.cdc.MustUnmarshal(iterVal, &priceState)
		priceStates = append(priceStates, &priceState)
		return false
	})

	return priceStates
}

// EmitSedaFastPriceUpdate emits the generic EventOraclePriceUpdate for a
// SEDA Fast feed. The Id is the feedID (hex-encoded execInputs).
func (k *Keeper) EmitSedaFastPriceUpdate(ctx sdk.Context, feedID string, priceState *types.PriceState) {
	defer k.Meter(ctx).FuncTiming(&ctx, "EmitSedaFastPriceUpdate")()

	// nolint:errcheck //ignored on purpose
	ctx.EventManager().EmitTypedEvent(&types.EventOraclePriceUpdate{
		OracleType: types.OracleType_SedaFast,
		Id:         feedID,
		PriceState: *priceState,
	})
}
