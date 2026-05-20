package base

import (
	"cosmossdk.io/store/prefix"
	storetypes "cosmossdk.io/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"

	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/types"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/types/v2"
)

func (k *BaseKeeper) AppendModifiedSubaccountsByMarket(ctx sdk.Context, marketID common.Hash, subaccountIDs []common.Hash) {
	defer k.Meter(ctx).FuncTiming(&ctx, "AppendModifiedSubaccountsByMarket")()

	if len(subaccountIDs) == 0 {
		return
	}

	store := k.getTransientStore(ctx)
	modifiedPositionsStore := prefix.NewStore(store, types.DerivativePositionModifiedSubaccountPrefix)

	existingSubaccountIDs := k.GetModifiedSubaccountsByMarket(ctx, marketID)
	existingSubaccountIDMap := make(map[common.Hash]struct{})

	if existingSubaccountIDs != nil {
		for _, subaccountID := range existingSubaccountIDs.SubaccountIds {
			existingSubaccountIDMap[common.BytesToHash(subaccountID)] = struct{}{}
		}
	} else {
		existingSubaccountIDs = &v2.SubaccountIDs{
			SubaccountIds: [][]byte{},
		}
	}

	for _, subaccountID := range subaccountIDs {
		// skip adding if already found
		if _, found := existingSubaccountIDMap[subaccountID]; found {
			continue
		}

		existingSubaccountIDs.SubaccountIds = append(existingSubaccountIDs.SubaccountIds, subaccountID.Bytes())
	}

	bz := k.cdc.MustMarshal(existingSubaccountIDs)
	modifiedPositionsStore.Set(marketID.Bytes(), bz)
}

func (k *BaseKeeper) GetModifiedSubaccountsByMarket(ctx sdk.Context, marketID common.Hash) *v2.SubaccountIDs {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetModifiedSubaccountsByMarket")()

	store := k.getTransientStore(ctx)
	modifiedPositionsStore := prefix.NewStore(store, types.DerivativePositionModifiedSubaccountPrefix)

	bz := modifiedPositionsStore.Get(marketID.Bytes())
	if bz == nil {
		return nil
	}

	var subaccountIDs v2.SubaccountIDs
	k.cdc.MustUnmarshal(bz, &subaccountIDs)

	return &subaccountIDs
}

func (k *BaseKeeper) SetPosition(
	ctx sdk.Context,
	marketID, subaccountID common.Hash,
	position *v2.Position,
) {
	defer k.Meter(ctx).FuncTiming(&ctx, "SetPosition")()

	store := k.getStore(ctx)
	positionStore := prefix.NewStore(store, types.DerivativePositionsPrefix)

	key := types.MarketSubaccountInfix(marketID, subaccountID)
	bz := k.cdc.MustMarshal(position)
	positionStore.Set(key, bz)

	if position != nil && !position.Quantity.IsZero() {
		k.SetActiveDerivativeMarketForSubaccount(ctx, subaccountID, marketID)
	} else {
		k.DeleteActiveDerivativeMarketForSubaccount(ctx, subaccountID, marketID)
	}
}

func (k *BaseKeeper) GetPosition(
	ctx sdk.Context,
	marketID, subaccountID common.Hash,
) *v2.Position {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetPosition")()

	store := k.getStore(ctx)
	positionStore := prefix.NewStore(store, types.DerivativePositionsPrefix)

	bz := positionStore.Get(types.MarketSubaccountInfix(marketID, subaccountID))
	if bz == nil {
		return nil
	}

	var position v2.Position
	k.cdc.MustUnmarshal(bz, &position)

	return &position
}

func (k *BaseKeeper) HasPosition(
	ctx sdk.Context,
	marketID, subaccountID common.Hash,
) bool {
	defer k.Meter(ctx).FuncTiming(&ctx, "HasPosition")()

	store := k.getStore(ctx)
	positionStore := prefix.NewStore(store, types.DerivativePositionsPrefix)

	key := types.MarketSubaccountInfix(marketID, subaccountID)
	return positionStore.Has(key)
}

func (k *BaseKeeper) DeletePosition(
	ctx sdk.Context,
	marketID, subaccountID common.Hash,
) {
	defer k.Meter(ctx).FuncTiming(&ctx, "DeletePosition")()

	store := k.getStore(ctx)

	positionStore := prefix.NewStore(store, types.DerivativePositionsPrefix)
	key := types.MarketSubaccountInfix(marketID, subaccountID)
	positionStore.Delete(key)
	k.DeleteActiveDerivativeMarketForSubaccount(ctx, subaccountID, marketID)
}

// IteratePositionsByMarket Iterates over all the positions in a given market calling process on each position.
func (k *BaseKeeper) IteratePositionsByMarket(ctx sdk.Context, marketID common.Hash, process func(*v2.Position, []byte) (stop bool)) {
	defer k.Meter(ctx).FuncTiming(&ctx, "IteratePositionsByMarket")()

	store := k.getStore(ctx)
	positionStore := prefix.NewStore(store, append(types.DerivativePositionsPrefix, marketID.Bytes()...))

	iterateSafe(positionStore.Iterator(nil, nil), func(key, value []byte) bool {
		var position v2.Position
		k.cdc.MustUnmarshal(value, &position)
		return process(&position, key)
	})
}

// IteratePositions iterates over all positions calling process on each position.
func (k *BaseKeeper) IteratePositions(ctx sdk.Context, process func(*v2.Position, []byte) (stop bool)) {
	defer k.Meter(ctx).FuncTiming(&ctx, "IteratePositions")()

	store := k.getStore(ctx)
	positionStore := prefix.NewStore(store, types.DerivativePositionsPrefix)

	iterateSafe(positionStore.Iterator(nil, nil), func(key, value []byte) bool {
		var position v2.Position
		k.cdc.MustUnmarshal(value, &position)
		return process(&position, key)
	})
}

// setTransientPosition sets a subaccount's position in the transient store for a given denom.
func (k *BaseKeeper) SetTransientPosition(
	ctx sdk.Context,
	marketID, subaccountID common.Hash,
	position *v2.Position,
) {
	defer k.Meter(ctx).FuncTiming(&ctx, "SetTransientPosition")()

	store := k.getTransientStore(ctx)
	positionStore := prefix.NewStore(store, types.DerivativePositionsPrefix)

	key := types.MarketSubaccountInfix(marketID, subaccountID)
	bz := k.cdc.MustMarshal(position)
	positionStore.Set(key, bz)
}

// IterateTransientPositions iterates over all positions calling process on each position.
//
//nolint:revive // ok
func (k *BaseKeeper) IterateTransientPositions(
	ctx sdk.Context,
	process func(marketID, subaccountID common.Hash, p *v2.Position) (stop bool),
) {
	defer k.Meter(ctx).FuncTiming(&ctx, "IterateTransientPositions")()

	// todo: this does not work
	//nolint:gocritic // ok
	// store := k.getTransientStore(ctx)
	// positionStore := prefix.NewStore(store, types.DerivativePositionsPrefix)
	//
	// iterateSafe(positionStore.Iterator(nil, nil), func(key, value []byte) bool {
	//	var position v2.Position
	//	k.cdc.MustUnmarshal(value, &position)
	//	return process(&position, key)
	// })

	store := k.getTransientStore(ctx)

	iterator := storetypes.KVStorePrefixIterator(store, types.DerivativePositionsPrefix)
	defer iterator.Close()

	for ; iterator.Valid(); iterator.Next() {
		var position v2.Position
		bz := iterator.Value()
		k.cdc.MustUnmarshal(bz, &position)

		marketID, subaccountID := types.ParsePositionTransientStoreKey(iterator.Key())
		if process(marketID, subaccountID, &position) {
			return
		}
	}
}
