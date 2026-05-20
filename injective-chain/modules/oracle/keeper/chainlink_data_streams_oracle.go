package keeper

import (
	"cosmossdk.io/math"
	"cosmossdk.io/store/prefix"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/InjectiveLabs/injective-core/injective-chain/modules/oracle/types"
	chaintypes "github.com/InjectiveLabs/injective-core/injective-chain/types"
)

// SetChainlinkDataStreamsPriceState stores a given Chainlink Data Streams price state.
func (k *Keeper) SetChainlinkDataStreamsPriceState(ctx sdk.Context, priceState *types.ChainlinkDataStreamsPriceState) {
	defer k.Meter(ctx).FuncTiming(&ctx, "SetChainlinkDataStreamsPriceState")()

	priceKey := types.GetChainlinkDataStreamsPriceStoreKey(priceState.FeedId)
	toStore := *priceState
	toStore.FeedId = ""
	bz := k.cdc.MustMarshal(&toStore)

	k.getStore(ctx).Set(priceKey, bz)

	k.AppendPriceRecord(ctx, types.OracleType_ChainlinkDataStreams, priceState.FeedId, &types.PriceRecord{
		Timestamp: priceState.PriceState.Timestamp,
		Price:     priceState.PriceState.Price,
	})
}

// GetChainlinkDataStreamsPriceState retrieves the Chainlink Data Streams price state for a given feed ID.
func (k *Keeper) GetChainlinkDataStreamsPriceState(ctx sdk.Context, feedID string) *types.ChainlinkDataStreamsPriceState {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetChainlinkDataStreamsPriceState")()

	var priceState types.ChainlinkDataStreamsPriceState
	bz := k.getStore(ctx).Get(types.GetChainlinkDataStreamsPriceStoreKey(feedID))
	if bz == nil {
		return nil
	}

	k.cdc.MustUnmarshal(bz, &priceState)
	priceState.FeedId = feedID
	return &priceState
}

// GetAllChainlinkDataStreamsPriceStates fetches all Chainlink Data Streams price states.
func (k *Keeper) GetAllChainlinkDataStreamsPriceStates(ctx sdk.Context) []*types.ChainlinkDataStreamsPriceState {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetAllChainlinkDataStreamsPriceStates")()

	priceStates := make([]*types.ChainlinkDataStreamsPriceState, 0)
	store := ctx.KVStore(k.storeKey)

	priceStore := prefix.NewStore(store, types.ChainlinkDataStreamsPriceKey)

	chaintypes.IterateSafe(priceStore.Iterator(nil, nil), func(iterKey, iterVal []byte) bool {
		var priceState types.ChainlinkDataStreamsPriceState
		k.cdc.MustUnmarshal(iterVal, &priceState)
		priceState.FeedId = types.GetChainlinkDataStreamsFeedIDFromIterKey(iterKey)
		priceStates = append(priceStates, &priceState)
		return false
	})

	return priceStates
}

// ProcessChainlinkDataStreamsReport processes a Chainlink Data Streams report and updates the price state.
// The caller must ensure price is non-nil and positive; otherwise the report is invalid and should be rejected.
func (k *Keeper) ProcessChainlinkDataStreamsReport(
	ctx sdk.Context,
	feedID string,
	reportPrice math.Int,
	validFromTimestamp uint64,
	observationsTimestamp uint64,
	expiresAt uint64,
	price math.LegacyDec,
) {
	defer k.Meter(ctx).FuncTiming(&ctx, "ProcessChainlinkDataStreamsReport")()

	priceState := k.GetChainlinkDataStreamsPriceState(ctx, feedID)
	blockTime := ctx.BlockTime().Unix()

	if priceState == nil {
		priceState = types.NewChainlinkDataStreamsPriceState(
			feedID,
			reportPrice,
			validFromTimestamp,
			observationsTimestamp,
			expiresAt,
			price,
			blockTime,
		)
	} else {
		// don't update prices with an older observation timestamp
		if priceState.ObservationsTimestamp >= observationsTimestamp {
			return
		}

		// skip price update if the price changes beyond 100x or less than 1% of the last price
		if types.CheckPriceFeedThreshold(priceState.PriceState.Price, price) {
			return
		}
		priceState.Update(reportPrice, validFromTimestamp, observationsTimestamp, expiresAt, price, blockTime)
	}

	k.SetChainlinkDataStreamsPriceState(ctx, priceState)

	// nolint:errcheck //ignored on purpose
	ctx.EventManager().EmitTypedEvent(&types.EventSetChainlinkDataStreamsPrices{
		Prices: []*types.ChainlinkDataStreamsPriceState{priceState},
	})
}
