// nolint:revive // ok
package base

import (
	"cosmossdk.io/math"
	"cosmossdk.io/store/prefix"
	storetypes "cosmossdk.io/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"

	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/types"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/types/v2"
)

func (k *BaseKeeper) UnmarshalDerivativeLimitOrder(bz []byte) v2.DerivativeLimitOrder {
	var order v2.DerivativeLimitOrder
	k.cdc.MustUnmarshal(bz, &order)
	return order
}

//nolint:revive // ok
func (k *BaseKeeper) DerivativeLimitOrdersIterator(
	ctx sdk.Context,
	marketID common.Hash,
	isBuy bool,
) storetypes.Iterator {
	defer k.Meter(ctx).FuncTiming(&ctx, "DerivativeLimitOrdersIterator")()

	store := k.getStore(ctx)
	prefixKey := types.DerivativeLimitOrdersPrefix
	prefixKey = append(prefixKey, types.MarketDirectionPrefix(marketID, isBuy)...)
	ordersStore := prefix.NewStore(store, prefixKey)

	var iterator storetypes.Iterator

	if isBuy {
		iterator = ordersStore.ReverseIterator(nil, nil)
	} else {
		iterator = ordersStore.Iterator(nil, nil)
	}

	return iterator
}

// IterateRestingDerivativeLimitOrderHashesBySubaccount iterates over the derivative limits order index for
// a given subaccountID and marketID and direction
//
//nolint:revive // ok
func (k *BaseKeeper) IterateRestingDerivativeLimitOrderHashesBySubaccount(
	ctx sdk.Context,
	marketID common.Hash,
	isBuy bool,
	subaccountID common.Hash,
	process func(orderHash common.Hash) (stop bool),
) {
	defer k.Meter(ctx).FuncTiming(&ctx, "IterateRestingDerivativeLimitOrderHashesBySubaccount")()

	store := k.getStore(ctx)
	orderIndexStore := prefix.NewStore(store, types.GetDerivativeLimitOrderIndexPrefix(marketID, isBuy, subaccountID))
	var iterator storetypes.Iterator
	if isBuy {
		iterator = orderIndexStore.ReverseIterator(nil, nil)
	} else {
		iterator = orderIndexStore.Iterator(nil, nil)
	}

	iterateSafe(iterator, func(_, v []byte) bool {
		orderHash := getOrderHashFromDerivativeOrderIndexKey(v)
		return process(orderHash)
	})
}

func (k *BaseKeeper) BasicSetNewDerivativeLimitOrder(
	ctx sdk.Context,
	order *v2.DerivativeLimitOrder,
	marketID common.Hash,
) {
	defer k.Meter(ctx).FuncTiming(&ctx, "BasicSetNewDerivativeLimitOrder")()

	store := k.getStore(ctx)
	ordersStore := prefix.NewStore(store, types.DerivativeLimitOrdersPrefix)

	// set main derivative order store
	priceKey := types.GetLimitOrderByPriceKeyPrefix(marketID, order.IsBuy(), order.Price(), order.Hash())
	bz := k.cdc.MustMarshal(order)
	ordersStore.Set(priceKey, bz)
}

func (k *BaseKeeper) SetNewDerivativeLimitOrder(
	ctx sdk.Context,
	order *v2.DerivativeLimitOrder,
	marketID common.Hash,
) {
	defer k.Meter(ctx).FuncTiming(&ctx, "SetNewDerivativeLimitOrder")()

	store := k.getStore(ctx)
	ordersIndexStore := prefix.NewStore(store, types.DerivativeLimitOrdersIndexPrefix)

	k.BasicSetNewDerivativeLimitOrder(ctx, order, marketID)

	// set subaccount index key store
	priceKey := types.GetLimitOrderByPriceKeyPrefix(marketID, order.IsBuy(), order.Price(), order.Hash())
	subaccountKey := types.GetLimitOrderIndexKey(marketID, order.IsBuy(), order.SubaccountID(), order.Hash())
	ordersIndexStore.Set(subaccountKey, priceKey)

	k.SetActiveDerivativeOrderMarketForSubaccount(ctx, order.SubaccountID(), marketID)
}

// DeleteDerivativeLimitOrderByFields deletes the DerivativeLimitOrder.
func (k *BaseKeeper) DeleteDerivativeLimitOrderByFields(
	ctx sdk.Context,
	marketID common.Hash,
	price math.LegacyDec,
	isBuy bool,
	hash common.Hash,
) *v2.DerivativeLimitOrder {
	defer k.Meter(ctx).FuncTiming(&ctx, "DeleteDerivativeLimitOrderByFields")()

	store := k.getStore(ctx)
	ordersStore := prefix.NewStore(store, types.DerivativeLimitOrdersPrefix)
	priceKey := types.GetLimitOrderByPriceKeyPrefix(marketID, isBuy, price, hash)
	orderBz := ordersStore.Get(priceKey)
	if orderBz == nil {
		return k.DeleteTransientDerivativeLimitOrderByFields(ctx, marketID, price, isBuy, hash)
	}

	var order v2.DerivativeLimitOrder
	k.cdc.MustUnmarshal(orderBz, &order)

	k.DeleteDerivativeLimitOrder(ctx, marketID, &order)

	return &order
}

func (k *BaseKeeper) DeleteSubaccountOrder(
	ctx sdk.Context,
	marketID common.Hash,
	order *v2.DerivativeLimitOrder,
) {
	defer k.Meter(ctx).FuncTiming(&ctx, "DeleteSubaccountOrder")()

	store := k.getStore(ctx)
	subaccountOrderKey := types.GetSubaccountOrderKey(marketID, order.SubaccountID(), order.IsBuy(), order.Price(), order.Hash())

	// delete from subaccount order store as well
	store.Delete(subaccountOrderKey)
}

// BasicDeleteDerivativeLimitOrder deletes the DerivativeLimitOrder.
func (k *BaseKeeper) BasicDeleteDerivativeLimitOrder(
	ctx sdk.Context,
	marketID common.Hash,
	order *v2.DerivativeLimitOrder,
) {
	defer k.Meter(ctx).FuncTiming(&ctx, "BasicDeleteDerivativeLimitOrder")()

	store := k.getStore(ctx)

	priceKey := types.GetLimitOrderByPriceKeyPrefix(marketID, order.IsBuy(), order.Price(), order.Hash())
	subaccountIndexKey := types.GetLimitOrderIndexKey(marketID, order.IsBuy(), order.SubaccountID(), order.Hash())

	ordersStore := prefix.NewStore(store, types.DerivativeLimitOrdersPrefix)
	ordersStore.Delete(priceKey)

	// delete from subaccount index key store
	ordersIndexStore := prefix.NewStore(store, types.DerivativeLimitOrdersIndexPrefix)
	ordersIndexStore.Delete(subaccountIndexKey)
}

// DeleteDerivativeLimitOrder deletes the DerivativeLimitOrder.
func (k *BaseKeeper) DeleteDerivativeLimitOrder(
	ctx sdk.Context,
	marketID common.Hash,
	order *v2.DerivativeLimitOrder,
) {
	defer k.Meter(ctx).FuncTiming(&ctx, "DeleteDerivativeLimitOrder")()

	k.BasicDeleteDerivativeLimitOrder(ctx, marketID, order)
	k.DeleteSubaccountOrder(ctx, marketID, order)
	k.DeleteCid(ctx, false, order.SubaccountID(), order.Cid())
	k.DecrementOrderbookPriceLevelQuantity(ctx, marketID, order.IsBuy(), false, order.GetPrice(), order.GetFillable())

	// NOTE: MaybeDeleteActiveDerivativeOrderMarketForSubaccount is NOT called here because
	// SubaccountOrderbookMetadata (used by hasAnyDerivativeOrdersInMarketForSubaccount) is
	// updated in batch after all order deletions. Callers that delete the last order in a market
	// must call MaybeDeleteActiveDerivativeOrderMarketForSubaccount after metadata is applied.
}

// IterateDerivativeLimitOrdersByMarketDirection iterates over derivative limits for a given marketID and direction.
// For buy limit orders, starts iteration over the highest price derivative limit orders
// For sell limit orders, starts iteration over the lowest price derivative limit orders
//
//nolint:revive // ok
func (k *BaseKeeper) IterateDerivativeLimitOrdersByMarketDirection(
	ctx sdk.Context,
	marketID common.Hash,
	isBuy bool,
	process func(order *v2.DerivativeLimitOrder) (stop bool),
) {
	defer k.Meter(ctx).FuncTiming(&ctx, "IterateDerivativeLimitOrdersByMarketDirection")()

	store := k.getStore(ctx)

	prefixKey := types.DerivativeLimitOrdersPrefix
	prefixKey = append(prefixKey, types.MarketDirectionPrefix(marketID, isBuy)...)
	ordersStore := prefix.NewStore(store, prefixKey)

	var iter storetypes.Iterator
	if isBuy {
		iter = ordersStore.ReverseIterator(nil, nil)
	} else {
		iter = ordersStore.Iterator(nil, nil)
	}

	iterateSafe(iter, func(_, v []byte) bool {
		var order v2.DerivativeLimitOrder
		k.cdc.MustUnmarshal(v, &order)
		return process(&order)
	})
}

// GetDerivativeLimitOrderBySubaccountIDAndHash returns the active derivative limit order from hash and subaccountID.
func (k *BaseKeeper) GetDerivativeLimitOrderBySubaccountIDAndHash(
	ctx sdk.Context,
	marketID common.Hash,
	isBuy *bool,
	subaccountID common.Hash,
	orderHash common.Hash,
) *v2.DerivativeLimitOrder {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetDerivativeLimitOrderBySubaccountIDAndHash")()

	store := k.getStore(ctx)
	ordersStore := prefix.NewStore(store, types.DerivativeLimitOrdersPrefix)
	ordersIndexStore := prefix.NewStore(store, types.DerivativeLimitOrdersIndexPrefix)

	priceKey, _ := fetchPriceKeyFromOrdersIndexStore(ordersIndexStore, marketID, isBuy, subaccountID, orderHash)
	if priceKey == nil {
		return nil
	}

	orderBz := ordersStore.Get(priceKey)
	if orderBz == nil {
		return nil
	}

	var order v2.DerivativeLimitOrder
	k.cdc.MustUnmarshal(orderBz, &order)

	return &order
}

// IterateDerivativeLimitOrdersBySubaccount iterates over the derivative limits order index for
// a given subaccountID and marketID and direction
//
//nolint:revive // ok
func (k *BaseKeeper) IterateDerivativeLimitOrdersBySubaccount(
	ctx sdk.Context,
	marketID common.Hash,
	isBuy bool,
	subaccountID common.Hash,
	process func(order v2.DerivativeLimitOrder) (stop bool),
) {
	defer k.Meter(ctx).FuncTiming(&ctx, "IterateDerivativeLimitOrdersBySubaccount")()

	store := k.getStore(ctx)
	ordersStore := prefix.NewStore(store, types.DerivativeLimitOrdersPrefix)
	orderIndexStore := prefix.NewStore(store, types.GetDerivativeLimitOrderIndexPrefix(marketID, isBuy, subaccountID))

	var iterator storetypes.Iterator
	if isBuy {
		iterator = orderIndexStore.ReverseIterator(nil, nil)
	} else {
		iterator = orderIndexStore.Iterator(nil, nil)
	}

	iterateSafe(iterator, func(_, v []byte) bool {
		var order v2.DerivativeLimitOrder
		k.cdc.MustUnmarshal(ordersStore.Get(v), &order)

		return process(order)
	})
}

// IterateDerivativeLimitOrdersByAddress iterates over the derivative limits order index for
// a given account address and marketID and direction
//
//nolint:revive // ok
func (k *BaseKeeper) IterateDerivativeLimitOrdersByAddress(
	ctx sdk.Context,
	marketID common.Hash,
	isBuy bool,
	accountAddress sdk.AccAddress,
	process func(order v2.DerivativeLimitOrder) (stop bool),
) {
	defer k.Meter(ctx).FuncTiming(&ctx, "IterateDerivativeLimitOrdersByAddress")()

	store := k.getStore(ctx)
	ordersStore := prefix.NewStore(store, types.DerivativeLimitOrdersPrefix)
	orderIndexStore := prefix.NewStore(store, types.GetDerivativeLimitOrderIndexByAccountAddressPrefix(marketID, isBuy, accountAddress))

	var iter storetypes.Iterator
	if isBuy {
		iter = orderIndexStore.ReverseIterator(nil, nil)
	} else {
		iter = orderIndexStore.Iterator(nil, nil)
	}

	iterateSafe(iter, func(_, v []byte) bool {
		var order v2.DerivativeLimitOrder
		k.cdc.MustUnmarshal(ordersStore.Get(v), &order)
		return process(order)
	})
}

// getOrderHashFromDerivativeOrderIndexKey returns the order hash contained in the second to last 32 bytes (HashLength) of the index key
func getOrderHashFromDerivativeOrderIndexKey(indexKey []byte) common.Hash {
	startIdx := len(indexKey) - common.HashLength
	return common.BytesToHash(indexKey[startIdx : startIdx+common.HashLength])
}

func fetchPriceKeyFromOrdersIndexStore(
	ordersIndexStore prefix.Store,
	marketID common.Hash,
	isHigherOrBuyDirection *bool,
	subaccountID common.Hash,
	orderHash common.Hash,
) (priceKey []byte, direction bool) {
	if isHigherOrBuyDirection != nil {
		subaccountKey := types.GetLimitOrderIndexKey(marketID, *isHigherOrBuyDirection, subaccountID, orderHash)
		return ordersIndexStore.Get(subaccountKey), *isHigherOrBuyDirection
	}

	direction = true
	subaccountKey := types.GetLimitOrderIndexKey(marketID, direction, subaccountID, orderHash)
	priceKey = ordersIndexStore.Get(subaccountKey)

	if priceKey == nil {
		direction = false
		subaccountKey = types.GetLimitOrderIndexKey(marketID, direction, subaccountID, orderHash)
		priceKey = ordersIndexStore.Get(subaccountKey)
	}

	return priceKey, direction
}

func (k *BaseKeeper) SetNewTransientDerivativeLimitOrder(
	ctx sdk.Context,
	order *v2.DerivativeLimitOrder,
	marketID common.Hash,
	isBuy bool,
	orderHash common.Hash,
) {
	defer k.Meter(ctx).FuncTiming(&ctx, "SetNewTransientDerivativeLimitOrder")()

	subaccountID := order.SubaccountID()

	// use transient store key
	tStore := k.getTransientStore(ctx)

	// set main derivative order transient store
	ordersStore := prefix.NewStore(tStore, types.DerivativeLimitOrdersPrefix)
	// marketID common.Hash, isBuy bool, price math.LegacyDec, orderHash
	key := types.GetLimitOrderByPriceKeyPrefix(marketID, isBuy, order.OrderInfo.Price, orderHash)
	bz := k.cdc.MustMarshal(order)
	ordersStore.Set(key, bz)

	ordersIndexStore := prefix.NewStore(tStore, types.DerivativeLimitOrdersIndexPrefix)
	subaccountKey := types.GetLimitOrderIndexKey(marketID, isBuy, subaccountID, orderHash)
	ordersIndexStore.Set(subaccountKey, key)
}

func (k *BaseKeeper) SetTransientDerivativeLimitOrderIndicator(
	ctx sdk.Context,
	marketID common.Hash,
	isBuy bool,
) {
	defer k.Meter(ctx).FuncTiming(&ctx, "SetTransientDerivativeLimitOrderIndicator")()
	// use transient store key
	tStore := k.getTransientStore(ctx)

	// set derivative order markets indicator store
	key := types.GetDerivativeLimitTransientMarketsKeyPrefix(marketID, isBuy)
	if !tStore.Has(key) {
		tStore.Set(key, []byte{})
	}
}


func (k *BaseKeeper) IterateTransientDerivativeLimitOrdersByMarketDirectionBySubaccountID(
	ctx sdk.Context,
	marketID common.Hash,
	isBuy bool,
	process func(o *v2.DerivativeLimitOrder) (stop bool),
) {
	defer k.Meter(ctx).FuncTiming(&ctx, "IterateTransientDerivativeLimitOrdersByMarketDirectionBySubaccountID")()

	store := k.getTransientStore(ctx)
	prefixKey := types.DerivativeLimitOrdersPrefix
	prefixKey = append(prefixKey, types.MarketDirectionPrefix(marketID, isBuy)...)
	ordersStore := prefix.NewStore(store, prefixKey)

	orders := []*v2.DerivativeLimitOrder{}

	var iterator storetypes.Iterator
	if isBuy {
		// iterate over limit buy orders from highest (best) to lowest (worst) price
		iterator = ordersStore.ReverseIterator(nil, nil)
	} else {
		iterator = ordersStore.Iterator(nil, nil)
	}

	iterateSafe(iterator, func(_, v []byte) bool {
		var order v2.DerivativeLimitOrder
		k.cdc.MustUnmarshal(v, &order)
		orders = append(orders, &order)

		return false
	})
	// iterator is closed at this point

	for _, order := range orders {
		if process(order) {
			return
		}
	}
}

// SetTransientDerivativeMarketOrder stores DerivativeMarketOrder in the transient store.
func (k *BaseKeeper) SetTransientDerivativeMarketOrder(
	ctx sdk.Context,
	marketOrder *v2.DerivativeMarketOrder,
	order *v2.DerivativeOrder,
	orderHash common.Hash,
) {
	defer k.Meter(ctx).FuncTiming(&ctx, "SetTransientDerivativeMarketOrder")()

	// use transient store key
	store := k.getTransientStore(ctx)
	marketID := common.HexToHash(order.MarketId)

	// set main derivative market order state transient store
	ordersStore := prefix.NewStore(store, types.DerivativeMarketOrdersPrefix)
	key := types.GetOrderByPriceKeyPrefix(marketID, order.OrderType.IsBuy(), marketOrder.OrderInfo.Price, orderHash)
	bz := k.cdc.MustMarshal(marketOrder)
	ordersStore.Set(key, bz)

	// set subaccount derivative market order store (for efficient per-subaccount iteration)
	subaccountKey := types.GetSubaccountDerivativeMarketOrderKey(marketID, order.SubaccountID(), order.IsBuy(), marketOrder.OrderInfo.Price, orderHash)
	store.Set(subaccountKey, []byte{})

	// set derivative order markets indicator store
	key = types.GetDerivativeMarketTransientMarketsKey(marketID, order.OrderType.IsBuy())
	if !store.Has(key) {
		store.Set(key, []byte{})
	}

	k.SetCid(ctx, true, order.SubaccountID(), order.Cid(), marketID, order.IsBuy(), orderHash)
}

func (k *BaseKeeper) DeleteDerivativeMarketOrder(
	ctx sdk.Context,
	order *v2.DerivativeMarketOrder,
	marketID common.Hash,
) {
	defer k.Meter(ctx).FuncTiming(&ctx, "DeleteDerivativeMarketOrder")()

	// use transient store key
	store := k.getTransientStore(ctx)

	// set main derivative market order state transient store
	ordersStore := prefix.NewStore(store, types.DerivativeMarketOrdersPrefix)
	orderHash := common.BytesToHash(order.OrderHash)
	key := types.GetOrderByPriceKeyPrefix(marketID, order.OrderType.IsBuy(), order.OrderInfo.Price, orderHash)
	ordersStore.Delete(key)

	subaccountKey := types.GetSubaccountDerivativeMarketOrderKey(marketID, order.SubaccountID(), order.IsBuy(), order.OrderInfo.Price, orderHash)
	store.Delete(subaccountKey)

	k.DeleteCid(ctx, true, order.SubaccountID(), order.Cid())
}

// IterateDerivativeMarketOrdersBySubaccount iterates over the transient derivative market orders for a given
// (marketID, subaccountID, direction).
//
// This avoids scanning all market orders in a market when building per-subaccount cross-margin snapshots.
func (k *BaseKeeper) IterateDerivativeMarketOrdersBySubaccount(
	ctx sdk.Context,
	marketID common.Hash,
	subaccountID common.Hash,
	isBuy bool,
	process func(order *v2.DerivativeMarketOrder) (stop bool),
) {
	defer k.Meter(ctx).FuncTiming(&ctx, "IterateDerivativeMarketOrdersBySubaccount")()

	store := k.getTransientStore(ctx)
	ordersStore := prefix.NewStore(store, types.DerivativeMarketOrdersPrefix)
	orderIndexStore := prefix.NewStore(store, types.GetSubaccountDerivativeMarketOrderPrefixByMarketSubaccountDirection(marketID, subaccountID, isBuy))

	var iter storetypes.Iterator
	if isBuy {
		iter = orderIndexStore.ReverseIterator(nil, nil)
	} else {
		iter = orderIndexStore.Iterator(nil, nil)
	}

	orderKeySuffixes := make([][]byte, 0)
	iterateKeysSafe(iter, func(key []byte) bool {
		// copy since iterator reuses the underlying slice
		orderKeySuffixes = append(orderKeySuffixes, append([]byte(nil), key...))
		return false
	})

	for _, suffix := range orderKeySuffixes {
		if len(suffix) <= common.HashLength {
			continue
		}

		paddedPriceBz := suffix[:len(suffix)-common.HashLength]
		orderHash := common.BytesToHash(suffix[len(suffix)-common.HashLength:])

		orderKey := make([]byte, 0, common.HashLength+1+len(paddedPriceBz)+common.HashLength)
		orderKey = append(orderKey, types.MarketDirectionPrefix(marketID, isBuy)...)
		orderKey = append(orderKey, paddedPriceBz...)
		orderKey = append(orderKey, orderHash.Bytes()...)

		bz := ordersStore.Get(orderKey)
		if bz == nil {
			continue
		}

		var order v2.DerivativeMarketOrder
		k.cdc.MustUnmarshal(bz, &order)
		if process(&order) {
			return
		}
	}
}

// GetTransientDerivativeMarketOrderForSubaccount returns the single transient derivative market order
// for the given (market, subaccount, direction), or nil if none exists. There is at most one
// market order per (subaccount, market) per block.
func (k *BaseKeeper) GetTransientDerivativeMarketOrderForSubaccount(
	ctx sdk.Context,
	marketID common.Hash,
	subaccountID common.Hash,
	isBuy bool,
) *v2.DerivativeMarketOrder {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetTransientDerivativeMarketOrderForSubaccount")()

	store := k.getTransientStore(ctx)
	ordersStore := prefix.NewStore(store, types.DerivativeMarketOrdersPrefix)
	indexStore := prefix.NewStore(store, types.GetSubaccountDerivativeMarketOrderPrefixByMarketSubaccountDirection(marketID, subaccountID, isBuy))

	// Collect the first key suffix via iterateKeysSafe (safe against transient store OOG edge cases).
	var orderKeySuffix []byte
	iterateKeysSafe(indexStore.Iterator(nil, nil), func(key []byte) bool {
		orderKeySuffix = append([]byte(nil), key...) // copy since iterator reuses the slice
		return true                                  // stop after first key
	})

	if len(orderKeySuffix) <= common.HashLength {
		return nil
	}

	paddedPriceBz := orderKeySuffix[:len(orderKeySuffix)-common.HashLength]
	orderHash := common.BytesToHash(orderKeySuffix[len(orderKeySuffix)-common.HashLength:])

	orderKey := make([]byte, 0, common.HashLength+1+len(paddedPriceBz)+common.HashLength)
	orderKey = append(orderKey, types.MarketDirectionPrefix(marketID, isBuy)...)
	orderKey = append(orderKey, paddedPriceBz...)
	orderKey = append(orderKey, orderHash.Bytes()...)

	bz := ordersStore.Get(orderKey)
	if bz == nil {
		return nil
	}

	var order v2.DerivativeMarketOrder
	k.cdc.MustUnmarshal(bz, &order)
	return &order
}

// HasTransientDerivativeMarketOrderForSubaccount returns true if the subaccount has at least one
// transient derivative market order in the given market and direction. This is cheaper than
// IterateDerivativeMarketOrdersBySubaccount when the caller only needs an existence check,
// since it avoids deserialising the order. There is at most one market order per
// (subaccount, market, direction).
func (k *BaseKeeper) HasTransientDerivativeMarketOrderForSubaccount(
	ctx sdk.Context,
	marketID common.Hash,
	subaccountID common.Hash,
	isBuy bool,
) bool {
	defer k.Meter(ctx).FuncTiming(&ctx, "HasTransientDerivativeMarketOrderForSubaccount")()

	store := k.getTransientStore(ctx)
	indexStore := prefix.NewStore(store, types.GetSubaccountDerivativeMarketOrderPrefixByMarketSubaccountDirection(marketID, subaccountID, isBuy))

	found := false
	iterateKeysSafe(indexStore.Iterator(nil, nil), func(_ []byte) bool {
		found = true
		return true // stop after first key
	})
	return found
}

// IterateTransientDerivativeLimitOrdersBySubaccount iterates over the transient derivative limits order index
// for a given subaccountID and marketID and direction
//
//nolint:revive // ok
func (k *BaseKeeper) IterateTransientDerivativeLimitOrdersBySubaccount(
	ctx sdk.Context,
	marketID common.Hash,
	isBuy bool,
	subaccountID common.Hash,
	process func(order *v2.DerivativeLimitOrder) (stop bool),
) {
	defer k.Meter(ctx).FuncTiming(&ctx, "IterateTransientDerivativeLimitOrdersBySubaccount")()

	store := k.getTransientStore(ctx)
	ordersStore := prefix.NewStore(store, types.DerivativeLimitOrdersPrefix)
	orderIndexStore := prefix.NewStore(store, types.GetDerivativeLimitOrderIndexPrefix(marketID, isBuy, subaccountID))

	var iter storetypes.Iterator
	if isBuy {
		iter = orderIndexStore.ReverseIterator(nil, nil)
	} else {
		iter = orderIndexStore.Iterator(nil, nil)
	}

	orderKeys := make([][]byte, 0)
	iterateSafe(iter, func(_, v []byte) bool {
		orderKeys = append(orderKeys, v)

		return false
	})

	// iter is closed at this point

	for _, key := range orderKeys {
		var order v2.DerivativeLimitOrder
		k.cdc.MustUnmarshal(ordersStore.Get(key), &order)

		if process(&order) {
			return
		}
	}
}

// GetTransientDerivativeLimitOrderBySubaccountIDAndHash returns the active derivative limit order from hash and subaccountID.
func (k *BaseKeeper) GetTransientDerivativeLimitOrderBySubaccountIDAndHash(
	ctx sdk.Context,
	marketID common.Hash,
	isBuy *bool,
	subaccountID common.Hash,
	orderHash common.Hash,
) *v2.DerivativeLimitOrder {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetTransientDerivativeLimitOrderBySubaccountIDAndHash")()

	store := k.getTransientStore(ctx)
	ordersIndexStore := prefix.NewStore(store, types.DerivativeLimitOrdersIndexPrefix)

	// Fetch price key from ordersIndexStore

	var priceKey []byte
	if isBuy == nil {
		subaccountKey := types.GetLimitOrderIndexKey(marketID, true, subaccountID, orderHash)
		priceKey = ordersIndexStore.Get(subaccountKey)
		if priceKey == nil {
			subaccountKey = types.GetLimitOrderIndexKey(marketID, false, subaccountID, orderHash)
			priceKey = ordersIndexStore.Get(subaccountKey)
		}
	} else {
		subaccountKey := types.GetLimitOrderIndexKey(marketID, *isBuy, subaccountID, orderHash)
		priceKey = ordersIndexStore.Get(subaccountKey)
	}

	if priceKey == nil {
		return nil
	}

	ordersStore := prefix.NewStore(store, types.DerivativeLimitOrdersPrefix)

	// Fetch LimitOrders from ordersStore
	orderBz := ordersStore.Get(priceKey)
	if orderBz == nil {
		return nil
	}

	var order v2.DerivativeLimitOrder

	k.cdc.MustUnmarshal(orderBz, &order)
	return &order
}

// DeleteTransientDerivativeLimitOrderByFields deletes the DerivativeLimitOrder from the transient store.
func (k *BaseKeeper) DeleteTransientDerivativeLimitOrderByFields(
	ctx sdk.Context,
	marketID common.Hash,
	price math.LegacyDec,
	isBuy bool,
	hash common.Hash,
) *v2.DerivativeLimitOrder {
	defer k.Meter(ctx).FuncTiming(&ctx, "DeleteTransientDerivativeLimitOrderByFields")()

	tStore := k.getTransientStore(ctx)
	// set main derivative order transient store
	ordersStore := prefix.NewStore(tStore, types.DerivativeLimitOrdersPrefix)
	// marketID common.Hash, isBuy bool, price math.LegacyDec, orderHash
	key := types.GetLimitOrderByPriceKeyPrefix(marketID, isBuy, price, hash)
	bz := ordersStore.Get(key)
	if bz == nil {
		return nil
	}

	var order v2.DerivativeLimitOrder
	k.cdc.MustUnmarshal(bz, &order)

	k.DeleteTransientDerivativeLimitOrder(ctx, marketID, &order)

	return &order
}

// DeleteTransientDerivativeLimitOrder deletes the DerivativeLimitOrder from the transient store.
func (k *BaseKeeper) DeleteTransientDerivativeLimitOrder(
	ctx sdk.Context,
	marketID common.Hash,
	order *v2.DerivativeLimitOrder,
) {
	defer k.Meter(ctx).FuncTiming(&ctx, "DeleteTransientDerivativeLimitOrder")()

	tStore := k.getTransientStore(ctx)
	// set main derivative order transient store
	ordersStore := prefix.NewStore(tStore, types.DerivativeLimitOrdersPrefix)
	// marketID common.Hash, isBuy bool, price math.LegacyDec, orderHash
	orderHash := common.BytesToHash(order.OrderHash)
	key := types.GetLimitOrderByPriceKeyPrefix(marketID, order.IsBuy(), order.OrderInfo.Price, orderHash)
	ordersStore.Delete(key)

	ordersIndexStore := prefix.NewStore(tStore, types.DerivativeLimitOrdersIndexPrefix)
	subaccountKey := types.GetLimitOrderIndexKey(marketID, order.IsBuy(), order.SubaccountID(), orderHash)
	ordersIndexStore.Delete(subaccountKey)

	subaccountOrderKey := types.GetSubaccountOrderKey(marketID, order.SubaccountID(), order.IsBuy(), order.Price(), orderHash)

	// delete from normal subaccount order store as well
	store := k.getStore(ctx)
	store.Delete(subaccountOrderKey)

	k.DeleteCid(ctx, true, order.SubaccountID(), order.Cid())
}

// IterateDerivativeMarketOrders iterates over the derivative market orders calling process on each one.
//
//nolint:revive // ok
func (k *BaseKeeper) IterateDerivativeMarketOrders(
	ctx sdk.Context,
	marketID common.Hash,
	isBuy bool,
	process func(order *v2.DerivativeMarketOrder) (stop bool),
) {
	defer k.Meter(ctx).FuncTiming(&ctx, "IterateDerivativeMarketOrders")()

	// use transient store key
	store := k.getTransientStore(ctx)

	prefixKey := types.DerivativeMarketOrdersPrefix
	prefixKey = append(prefixKey, types.MarketDirectionPrefix(marketID, isBuy)...)
	ordersStore := prefix.NewStore(store, prefixKey)

	orders := []*v2.DerivativeMarketOrder{}

	var iterator storetypes.Iterator
	if isBuy {
		// iterate over market buy orders from highest to lowest price
		iterator = ordersStore.ReverseIterator(nil, nil)
	} else {
		iterator = ordersStore.Iterator(nil, nil)
	}

	iterateSafe(iterator, func(_, v []byte) bool {
		var order v2.DerivativeMarketOrder
		k.cdc.MustUnmarshal(v, &order)
		orders = append(orders, &order)
		return false
	})

	// iterator is closed at this point

	for _, order := range orders {
		if process(order) {
			return
		}
	}
}

// GetAllTransientDerivativeMarketDirections iterates over all of a derivative market's marketID
// directions for this block.
//
//nolint:revive // ok
func (k *BaseKeeper) GetAllTransientDerivativeMarketDirections(
	ctx sdk.Context,
	isLimit bool,
) []*types.MatchedMarketDirection {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetAllTransientDerivativeMarketDirections")()

	store := k.getTransientStore(ctx)

	var keyPrefix []byte

	if isLimit {
		keyPrefix = types.DerivativeLimitOrderIndicatorPrefix
	} else {
		keyPrefix = types.DerivativeMarketOrderIndicatorPrefix
	}
	marketIndicatorStore := prefix.NewStore(store, keyPrefix)

	matchedMarketDirections := make([]*types.MatchedMarketDirection, 0)

	marketIDs := make([]common.Hash, 0)
	marketDirectionMap := make(map[common.Hash]*types.MatchedMarketDirection)

	iterateSafe(marketIndicatorStore.Iterator(nil, nil), func(key, _ []byte) bool {
		marketId, isBuy := types.GetMarketIdDirectionFromTransientKey(key)
		if marketDirectionMap[marketId] == nil {
			marketIDs = append(marketIDs, marketId)
			matchedMarketDirection := types.MatchedMarketDirection{
				MarketId: marketId,
			}
			marketDirectionMap[marketId] = &matchedMarketDirection
		}
		marketDirectionMap[marketId].BuysExists = isBuy
		marketDirectionMap[marketId].SellsExists = !isBuy
		return false
	})

	for _, marketId := range marketIDs {
		matchedMarketDirections = append(matchedMarketDirections, marketDirectionMap[marketId])
	}

	return matchedMarketDirections
}
