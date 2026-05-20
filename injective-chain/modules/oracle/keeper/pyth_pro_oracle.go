package keeper

import (
	"strconv"

	"cosmossdk.io/store/prefix"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/InjectiveLabs/injective-core/injective-chain/modules/oracle/types"
	chaintypes "github.com/InjectiveLabs/injective-core/injective-chain/types"
)

// SetPythProPriceState persists a PythPro price state.
// The FeedId is encoded in the store key and stripped from the stored value to avoid duplication.
func (k *Keeper) SetPythProPriceState(ctx sdk.Context, priceState *types.PythProPriceState) {
	defer k.Meter(ctx).FuncTiming(&ctx, "SetPythProPriceState")()

	key := types.GetPythProPriceStoreKey(priceState.FeedId)
	toStore := *priceState
	toStore.FeedId = 0
	bz := k.cdc.MustMarshal(&toStore)

	k.getStore(ctx).Set(key, bz)

	k.AppendPriceRecord(ctx, types.OracleType_PythPro,
		strconv.FormatUint(uint64(priceState.FeedId), 10),
		&types.PriceRecord{
			Timestamp: priceState.PriceState.Timestamp,
			Price:     priceState.PriceState.Price,
		},
	)
}

// GetPythProPriceState retrieves the PythPro price state for the given feed ID,
// restoring the FeedId from the store key.
func (k *Keeper) GetPythProPriceState(ctx sdk.Context, feedID uint32) *types.PythProPriceState {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetPythProPriceState")()

	bz := k.getStore(ctx).Get(types.GetPythProPriceStoreKey(feedID))
	if bz == nil {
		return nil
	}

	var priceState types.PythProPriceState
	k.cdc.MustUnmarshal(bz, &priceState)
	priceState.FeedId = feedID
	return &priceState
}

// GetAllPythProPriceStates returns all persisted PythPro price states.
func (k *Keeper) GetAllPythProPriceStates(ctx sdk.Context) []*types.PythProPriceState {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetAllPythProPriceStates")()

	priceStates := make([]*types.PythProPriceState, 0)
	store := ctx.KVStore(k.storeKey)
	priceStore := prefix.NewStore(store, types.PythProPriceKey)

	chaintypes.IterateSafe(priceStore.Iterator(nil, nil), func(iterKey, iterVal []byte) bool {
		var priceState types.PythProPriceState
		k.cdc.MustUnmarshal(iterVal, &priceState)
		priceState.FeedId = types.GetPythProFeedIDFromIterKey(iterKey)
		priceStates = append(priceStates, &priceState)
		return false
	})

	return priceStates
}

// EmitPythProPriceUpdate emits the new generic EventOraclePriceUpdate event for a PythPro feed.
func (k *Keeper) EmitPythProPriceUpdate(ctx sdk.Context, feedID uint32, priceState *types.PriceState) {
	defer k.Meter(ctx).FuncTiming(&ctx, "EmitPythProPriceUpdate")()

	// nolint:errcheck //ignored on purpose
	ctx.EventManager().EmitTypedEvent(&types.EventOraclePriceUpdate{
		OracleType: types.OracleType_PythPro,
		Id:         strconv.FormatUint(uint64(feedID), 10),
		PriceState: *priceState,
	})
}
