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

func (k *BaseKeeper) GetTransientStoreKey() storetypes.StoreKey {
	return k.tStoreKey
}

func (k *BaseKeeper) GetObjectStoreKey() storetypes.StoreKey {
	return k.objectStoreKey
}

func (k *BaseKeeper) UnmarshalSpotLimitOrder(bz []byte) v2.SpotLimitOrder {
	var order v2.SpotLimitOrder
	k.cdc.MustUnmarshal(bz, &order)
	return order
}

//nolint:revive // ok
func (k *BaseKeeper) SpotLimitOrderbookIterator(
	ctx sdk.Context,
	marketID common.Hash,
	isBuy bool,
) storetypes.Iterator {
	defer k.Meter(ctx).FuncTiming(&ctx, "SpotLimitOrderbookIterator")()

	store := k.getStore(ctx)
	prefixKey := types.SpotLimitOrdersPrefix
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

func (k *BaseKeeper) SetSpotLimitOrder(
	ctx sdk.Context,
	order *v2.SpotLimitOrder,
	marketID common.Hash,
	isBuy bool,
	orderHash common.Hash,
) {
	defer k.Meter(ctx).FuncTiming(&ctx, "SetSpotLimitOrder")()

	store := k.getStore(ctx)
	ordersStore := prefix.NewStore(store, types.SpotLimitOrdersPrefix)
	ordersIndexStore := prefix.NewStore(store, types.SpotLimitOrdersIndexPrefix)

	priceKey := types.GetLimitOrderByPriceKeyPrefix(marketID, isBuy, order.OrderInfo.Price, orderHash)
	bz := k.cdc.MustMarshal(order)
	ordersStore.Set(priceKey, bz)

	// set subaccount index key store
	subaccountKey := types.GetLimitOrderIndexKey(marketID, isBuy, order.SubaccountID(), orderHash)
	ordersIndexStore.Set(subaccountKey, priceKey)
}

// IterateSpotLimitOrdersBySubaccount iterates over the spot limits order index for a given subaccountID and marketID and direction
//
//nolint:revive // ok
func (k *BaseKeeper) IterateSpotLimitOrdersBySubaccount(
	ctx sdk.Context,
	marketID common.Hash,
	isBuy bool,
	subaccountID common.Hash,
	process func(order v2.SpotLimitOrder) (stop bool),
) {
	defer k.Meter(ctx).FuncTiming(&ctx, "IterateSpotLimitOrdersBySubaccount")()

	store := k.getStore(ctx)
	ordersStore := prefix.NewStore(store, types.SpotLimitOrdersPrefix)
	orderIndexStore := prefix.NewStore(store, types.GetSpotLimitOrderIndexPrefix(marketID, isBuy, subaccountID))
	var iterator storetypes.Iterator
	if isBuy {
		iterator = orderIndexStore.ReverseIterator(nil, nil)
	} else {
		iterator = orderIndexStore.Iterator(nil, nil)
	}

	iterateSafe(iterator, func(_, v []byte) bool {
		var order v2.SpotLimitOrder
		k.cdc.MustUnmarshal(ordersStore.Get(v), &order)
		return process(order)
	})
}

// IterateSpotLimitOrdersByAccountAddress iterates over the spot limits order index for a given account address and marketID and direction
//
//nolint:revive // ok
func (k *BaseKeeper) IterateSpotLimitOrdersByAccountAddress(
	ctx sdk.Context,
	marketID common.Hash,
	isBuy bool,
	accountAddress sdk.AccAddress,
	process func(order v2.SpotLimitOrder) (stop bool),
) {
	defer k.Meter(ctx).FuncTiming(&ctx, "IterateSpotLimitOrdersByAccountAddress")()

	store := k.getStore(ctx)
	orderStore := prefix.NewStore(store, types.SpotLimitOrdersPrefix)
	orderIndexStore := prefix.NewStore(store, types.GetSpotLimitOrderIndexByAccountAddressPrefix(marketID, isBuy, accountAddress))
	var iterator storetypes.Iterator
	if isBuy {
		iterator = orderIndexStore.ReverseIterator(nil, nil)
	} else {
		iterator = orderIndexStore.Iterator(nil, nil)
	}

	iterateSafe(iterator, func(_, v []byte) bool {
		var order v2.SpotLimitOrder
		k.cdc.MustUnmarshal(orderStore.Get(v), &order)
		return process(order)
	})
}

// GetSpotOrderSubaccountIDsByAccountAddress returns the unique subaccount IDs that have spot limit orders
// in the given market for the given account address.
// It skip-scans the order index keys by nonce, reading only O(subaccounts) keys instead of O(orders).
func (k *BaseKeeper) GetSpotOrderSubaccountIDsByAccountAddress(
	ctx sdk.Context,
	marketID common.Hash,
	accountAddress sdk.AccAddress,
) []common.Hash {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetSpotOrderSubaccountIDsByAccountAddress")()

	store := k.getStore(ctx)

	// Index key structure after AccountAddress prefix: nonce (12 bytes) + orderHash (32 bytes)
	// SubaccountID = accountAddress (20 bytes) + nonce (12 bytes)
	const nonceLen = common.HashLength - common.AddressLength

	var result []common.Hash
	seen := make(map[common.Hash]struct{})

	for _, isBuy := range []bool{true, false} {
		orderIndexStore := prefix.NewStore(store, types.GetSpotLimitOrderIndexByAccountAddressPrefix(marketID, isBuy, accountAddress))
		result = collectSubaccountsBySkipScan(orderIndexStore, nonceLen, accountAddress, result, seen)
	}

	return result
}

// GetTransientSpotOrderSubaccountIDsByAccountAddress returns the unique subaccount IDs that have transient
// spot limit orders in the given market for the given account address.
// It uses a ranged iterator scoped to the address prefix of the transient order index.
func (k *BaseKeeper) GetTransientSpotOrderSubaccountIDsByAccountAddress(
	ctx sdk.Context,
	marketID common.Hash,
	accountAddress sdk.AccAddress,
) []common.Hash {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetTransientSpotOrderSubaccountIDsByAccountAddress")()

	store := k.getTransientStore(ctx)

	// Transient index key (after SpotLimitOrdersIndexPrefix + marketID + isBuy):
	//   subaccountID (32B) + orderHash (32B)
	// subaccountID = address (20B) + nonce (12B)
	// Range by address prefix to only visit this user's entries.
	start := make([]byte, common.HashLength)
	copy(start, accountAddress.Bytes())

	end := incrementBytes(accountAddress.Bytes())
	var endKey []byte
	if end != nil {
		endKey = make([]byte, common.HashLength)
		copy(endKey, end)
	}

	var result []common.Hash
	seen := make(map[common.Hash]struct{})

	for _, isBuy := range []bool{true, false} {
		indexPrefix := append(types.SpotLimitOrdersIndexPrefix, types.MarketDirectionPrefix(marketID, isBuy)...)
		indexStore := prefix.NewStore(store, indexPrefix)

		iterateSafe(indexStore.Iterator(start, endKey), func(key, _ []byte) bool {
			subaccountID := common.BytesToHash(key[:common.HashLength])
			if _, ok := seen[subaccountID]; !ok {
				seen[subaccountID] = struct{}{}
				result = append(result, subaccountID)
			}
			return false
		})
	}

	return result
}

// collectSubaccountsBySkipScan reads only the first key per nonce prefix from the
// given store, building unique subaccount IDs without iterating all orders.
// The seen map is used to deduplicate across multiple calls (e.g. buy + sell sides).
func collectSubaccountsBySkipScan(
	store storetypes.KVStore,
	nonceLen int,
	accountAddress sdk.AccAddress,
	result []common.Hash,
	seen map[common.Hash]struct{},
) []common.Hash {
	var start []byte // nil is beginning

	for {
		iter := store.Iterator(start, nil)
		if !iter.Valid() {
			iter.Close()
			return result
		}

		key := iter.Key()
		iter.Close()

		// extract the nonce and build the subaccount ID
		nonce := key[:nonceLen]

		var subaccountID common.Hash
		copy(subaccountID[:common.AddressLength], accountAddress)
		copy(subaccountID[common.AddressLength:], nonce)

		if _, ok := seen[subaccountID]; !ok {
			seen[subaccountID] = struct{}{}
			result = append(result, subaccountID)
		}

		// skip to the next nonce: increment the nonce bytes to form the exclusive start key
		start = incrementBytes(nonce)
		if start == nil {
			return result // nonce overflow, no more nonces possible
		}
	}
}

// incrementBytes returns the lexicographically next byte slice of the same length,
// or nil if it overflows (all 0xFF).
func incrementBytes(b []byte) []byte {
	result := make([]byte, len(b))
	copy(result, b)
	for i := len(result) - 1; i >= 0; i-- {
		result[i]++
		if result[i] != 0 {
			return result
		}
	}
	return nil // overflow
}

// GetSpotLimitOrderByPrice returns active spot limit Order from hash and price.
func (k *BaseKeeper) GetSpotLimitOrderByPrice(
	ctx sdk.Context,
	marketID common.Hash,
	isBuy bool,
	price math.LegacyDec,
	orderHash common.Hash,
) *v2.SpotLimitOrder {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetSpotLimitOrderByPrice")()

	store := k.getStore(ctx)

	ordersStore := prefix.NewStore(store, types.SpotLimitOrdersPrefix)
	key := types.SpotMarketDirectionPriceHashPrefix(marketID, isBuy, price, orderHash)
	bz := ordersStore.Get(key)
	if bz == nil {
		return nil
	}

	var order v2.SpotLimitOrder
	k.cdc.MustUnmarshal(bz, &order)
	return &order
}

// GetSpotLimitOrderBySubaccountID returns active spot limit Order from hash and subaccountID.
func (k *BaseKeeper) GetSpotLimitOrderBySubaccountID(
	ctx sdk.Context,
	marketID common.Hash,
	isBuy *bool,
	subaccountID common.Hash,
	orderHash common.Hash,
) *v2.SpotLimitOrder {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetSpotLimitOrderBySubaccountID")()

	store := k.getStore(ctx)
	ordersStore := prefix.NewStore(store, types.SpotLimitOrdersPrefix)
	ordersIndexStore := prefix.NewStore(store, types.SpotLimitOrdersIndexPrefix)

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

	// Fetch LimitOrder from ordersStore
	bz := ordersStore.Get(priceKey)
	if bz == nil {
		return nil
	}

	var order v2.SpotLimitOrder
	k.cdc.MustUnmarshal(bz, &order)

	return &order
}

// DeleteSpotLimitOrder deletes the SpotLimitOrder.
func (k *BaseKeeper) DeleteSpotLimitOrder(
	ctx sdk.Context,
	marketID common.Hash,
	isBuy bool,
	order *v2.SpotLimitOrder,
) {
	defer k.Meter(ctx).FuncTiming(&ctx, "DeleteSpotLimitOrder")()

	store := k.getStore(ctx)
	ordersIndexStore := prefix.NewStore(store, types.SpotLimitOrdersIndexPrefix)
	subaccountKey := types.GetLimitOrderIndexKey(marketID, isBuy, order.SubaccountID(), common.BytesToHash(order.OrderHash))

	priceKey := ordersIndexStore.Get(subaccountKey)

	// delete main spot order store
	ordersStore := prefix.NewStore(store, types.SpotLimitOrdersPrefix)
	ordersStore.Delete(priceKey)

	// delete from subaccount index key store
	ordersIndexStore.Delete(subaccountKey)
}

// IterateSpotLimitOrdersByMarketDirection iterates over spot limits for a given marketID and direction.
// For buy limit orders, starts iteration over the highest price spot limit orders.
// For sell limit orders, starts iteration over the lowest price spot limit orders.
//
//nolint:revive // ok
func (k *BaseKeeper) IterateSpotLimitOrdersByMarketDirection(
	ctx sdk.Context,
	marketID common.Hash,
	isBuy bool,
	process func(order *v2.SpotLimitOrder) (stop bool),
) {
	defer k.Meter(ctx).FuncTiming(&ctx, "IterateSpotLimitOrdersByMarketDirection")()

	store := k.getStore(ctx)
	prefixKey := types.SpotLimitOrdersPrefix
	prefixKey = append(prefixKey, types.MarketDirectionPrefix(marketID, isBuy)...)
	ordersStore := prefix.NewStore(store, prefixKey)

	var iter storetypes.Iterator
	if isBuy {
		iter = ordersStore.ReverseIterator(nil, nil)
	} else {
		iter = ordersStore.Iterator(nil, nil)
	}

	iterateSafe(iter, func(_, v []byte) bool {
		var order v2.SpotLimitOrder
		k.cdc.MustUnmarshal(v, &order)
		return process(&order)
	})
}

// SetTransientSpotLimitOrder stores SpotLimitOrder in the transient store.
func (k *BaseKeeper) SetTransientSpotLimitOrder(
	ctx sdk.Context,
	order *v2.SpotLimitOrder,
	marketID common.Hash,
	isBuy bool,
	orderHash common.Hash,
) {
	defer k.Meter(ctx).FuncTiming(&ctx, "SetTransientSpotLimitOrder")()

	// use transient store key
	store := k.getTransientStore(ctx)

	// set main spot order transient store
	ordersStore := prefix.NewStore(store, types.SpotLimitOrdersPrefix)
	key := types.GetLimitOrderByPriceKeyPrefix(marketID, isBuy, order.OrderInfo.Price, orderHash)
	bz := k.cdc.MustMarshal(order)
	ordersStore.Set(key, bz)

	// set subaccount index key store
	ordersIndexStore := prefix.NewStore(store, types.SpotLimitOrdersIndexPrefix)
	subaccountKey := types.GetLimitOrderIndexKey(marketID, isBuy, order.SubaccountID(), orderHash)
	bz = key
	ordersIndexStore.Set(subaccountKey, bz)

	// set spot order markets indicator store
	key = types.GetSpotMarketTransientMarketsKey(marketID, isBuy)
	if !store.Has(key) {
		store.Set(key, []byte{})
	}

	k.SetCid(ctx, true, order.SubaccountID(), order.Cid(), marketID, isBuy, orderHash)
}

// IterateTransientSpotLimitOrdersBySubaccount iterates over the transient spot limits orders for a given subaccountID and marketID and direction
//
//nolint:revive // ok
func (k *BaseKeeper) IterateTransientSpotLimitOrdersBySubaccount(
	ctx sdk.Context,
	marketID common.Hash,
	isBuy bool,
	subaccountID common.Hash,
	process func(order *v2.SpotLimitOrder) (stop bool),
) {
	defer k.Meter(ctx).FuncTiming(&ctx, "IterateTransientSpotLimitOrdersBySubaccount")()

	store := k.getTransientStore(ctx)
	ordersStore := prefix.NewStore(store, types.SpotLimitOrdersPrefix)
	orderIndexStore := prefix.NewStore(store, types.GetTransientLimitOrderIndexIteratorPrefix(marketID, isBuy, subaccountID))

	orders := []*v2.SpotLimitOrder{}

	var iterator storetypes.Iterator
	if isBuy {
		iterator = orderIndexStore.ReverseIterator(nil, nil)
	} else {
		iterator = orderIndexStore.Iterator(nil, nil)
	}

	iterateSafe(iterator, func(_, value []byte) bool {
		var order v2.SpotLimitOrder
		k.cdc.MustUnmarshal(ordersStore.Get(value), &order)
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

// GetTransientSpotLimitOrderBySubaccountID returns transient spot limit Order from hash and subaccountID.
func (k *BaseKeeper) GetTransientSpotLimitOrderBySubaccountID(
	ctx sdk.Context,
	marketID common.Hash,
	isBuy *bool,
	subaccountID common.Hash,
	orderHash common.Hash,
) *v2.SpotLimitOrder {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetTransientSpotLimitOrderBySubaccountID")()
	// use transient store key
	store := k.getTransientStore(ctx)

	ordersStore := prefix.NewStore(store, types.SpotLimitOrdersPrefix)
	ordersIndexStore := prefix.NewStore(store, types.SpotLimitOrdersIndexPrefix)

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

	// Fetch LimitOrder from ordersStore
	bz := ordersStore.Get(priceKey)
	if bz == nil {
		return nil
	}

	var order v2.SpotLimitOrder
	k.cdc.MustUnmarshal(bz, &order)

	return &order
}

// DeleteTransientSpotLimitOrder deletes the SpotLimitOrder from the transient store.
func (k *BaseKeeper) DeleteTransientSpotLimitOrder(
	ctx sdk.Context,
	marketID common.Hash,
	order *v2.SpotLimitOrder,
) {
	defer k.Meter(ctx).FuncTiming(&ctx, "DeleteTransientSpotLimitOrder")()

	store := k.getTransientStore(ctx)

	ordersStore := prefix.NewStore(store, types.SpotLimitOrdersPrefix)
	ordersIndexStore := prefix.NewStore(store, types.SpotLimitOrdersIndexPrefix)

	priceKey := types.GetLimitOrderByPriceKeyPrefix(marketID, order.IsBuy(), order.OrderInfo.Price, order.Hash())

	// delete from main spot order store
	ordersStore.Delete(priceKey)

	// delete from subaccount index key store
	subaccountKey := types.GetLimitOrderIndexKey(marketID, order.IsBuy(), order.SubaccountID(), order.Hash())
	ordersIndexStore.Delete(subaccountKey)

	k.DeleteCid(ctx, true, order.SubaccountID(), order.Cid())
}

// GetAllTransientMatchedSpotLimitOrderMarkets retrieves all markets referenced by this block's transient SpotLimitOrders.
func (k *BaseKeeper) GetAllTransientMatchedSpotLimitOrderMarkets(
	ctx sdk.Context,
) []*types.MatchedMarketDirection {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetAllTransientMatchedSpotLimitOrderMarkets")()

	// use transient store key
	store := k.getTransientStore(ctx)

	// set spot order markets indicator store
	marketIndicatorStore := prefix.NewStore(store, types.SpotMarketsPrefix)

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

// IterateSpotMarketOrders iterates over the spot market orders calling process on each one.
//
//nolint:revive // ok
func (k *BaseKeeper) IterateSpotMarketOrders(
	ctx sdk.Context,
	marketID common.Hash,
	isBuy bool,
	process func(order *v2.SpotMarketOrder) (stop bool),
) {
	defer k.Meter(ctx).FuncTiming(&ctx, "IterateSpotMarketOrders")()

	// use transient store key
	store := k.getTransientStore(ctx)

	prefixKey := types.SpotMarketOrdersPrefix
	prefixKey = append(prefixKey, types.MarketDirectionPrefix(marketID, isBuy)...)
	ordersStore := prefix.NewStore(store, prefixKey)

	orders := []*v2.SpotMarketOrder{}

	var iterator storetypes.Iterator
	if isBuy {
		// iterate over market buy orders from highest to lowest price
		iterator = ordersStore.ReverseIterator(nil, nil)
	} else {
		iterator = ordersStore.Iterator(nil, nil)
	}

	iterateSafe(iterator, func(_, v []byte) bool {
		var order v2.SpotMarketOrder
		k.cdc.MustUnmarshal(v, &order)
		orders = append(orders, &order)
		return false
	})

	for _, order := range orders {
		if process(order) {
			return
		}
	}
}

// GetAllTransientSpotLimitOrdersByMarketDirection retrieves all transient SpotLimitOrders for
// a given market and direction.
//
//nolint:revive // ok
func (k *BaseKeeper) GetAllTransientSpotLimitOrdersByMarketDirection(
	ctx sdk.Context,
	marketID common.Hash,
	isBuy bool,
) []*v2.SpotLimitOrder {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetAllTransientSpotLimitOrdersByMarketDirection")()

	// use transient store key
	store := k.getTransientStore(ctx)

	prefixKey := types.SpotLimitOrdersPrefix
	prefixKey = append(prefixKey, types.MarketDirectionPrefix(marketID, isBuy)...)
	ordersStore := prefix.NewStore(store, prefixKey)
	var iterator storetypes.Iterator

	if isBuy {
		// iterate over market buy orders from highest to lowest price
		iterator = ordersStore.ReverseIterator(nil, nil)
	} else {
		iterator = ordersStore.Iterator(nil, nil)
	}

	orders := make([]*v2.SpotLimitOrder, 0)

	iterateSafe(iterator, func(_, v []byte) bool {
		var order v2.SpotLimitOrder
		k.cdc.MustUnmarshal(v, &order)
		orders = append(orders, &order)
		return false
	})

	return orders
}

// SetTransientSpotMarketOrder stores SpotMarketOrder in the transient store.
func (k *BaseKeeper) SetTransientSpotMarketOrder(
	ctx sdk.Context,
	marketOrder *v2.SpotMarketOrder,
	order *v2.SpotOrder,
	orderHash common.Hash,
) {
	defer k.Meter(ctx).FuncTiming(&ctx, "SetTransientSpotMarketOrder")()

	// use transient store key
	store := k.getTransientStore(ctx)

	marketId := common.HexToHash(order.MarketId)

	// set main spot market order state transient store
	ordersStore := prefix.NewStore(store, types.SpotMarketOrdersPrefix)
	key := types.GetOrderByPriceKeyPrefix(marketId, order.IsBuy(), marketOrder.OrderInfo.Price, orderHash)
	bz := k.cdc.MustMarshal(marketOrder)
	ordersStore.Set(key, bz)
}

// DeleteTransientSpotMarketOrder removes a transient spot market order from the transient store.
func (k *BaseKeeper) DeleteTransientSpotMarketOrder(
	ctx sdk.Context,
	marketID common.Hash,
	isBuy bool,
	order *v2.SpotMarketOrder,
) {
	defer k.Meter(ctx).FuncTiming(&ctx, "DeleteTransientSpotMarketOrder")()

	store := k.getTransientStore(ctx)
	ordersStore := prefix.NewStore(store, types.SpotMarketOrdersPrefix)
	key := types.GetOrderByPriceKeyPrefix(marketID, isBuy, order.OrderInfo.Price, common.BytesToHash(order.OrderHash))
	ordersStore.Delete(key)

	k.DeleteCid(ctx, true, order.SubaccountID(), order.Cid())
}

// GetAllTransientSpotMarketOrders iterates over spot market exchange over a given direction.
//
//nolint:revive // ok
func (k *BaseKeeper) GetAllTransientSpotMarketOrders(
	ctx sdk.Context,
	marketID common.Hash,
	isBuy bool,
) []*v2.SpotMarketOrder {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetAllTransientSpotMarketOrders")()

	store := k.getTransientStore(ctx)

	// set main spot market order state transient store
	prefixKey := types.SpotMarketOrdersPrefix
	prefixKey = append(prefixKey, types.MarketDirectionPrefix(marketID, isBuy)...)
	ordersStore := prefix.NewStore(store, prefixKey)
	var iterator storetypes.Iterator
	if isBuy {
		// iterate over market buy orders from highest to lowest price
		iterator = ordersStore.ReverseIterator(nil, nil)
	} else {
		iterator = ordersStore.Iterator(nil, nil)
	}

	spotMarketOrders := make([]*v2.SpotMarketOrder, 0)

	iterateSafe(iterator, func(_, v []byte) bool {
		var order v2.SpotMarketOrder
		k.cdc.MustUnmarshal(v, &order)
		spotMarketOrders = append(spotMarketOrders, &order)
		return false
	})

	return spotMarketOrders
}

// GetTransientMarketOrderIndicator gets the transient market order indicator in the transient store.
func (k *BaseKeeper) GetTransientMarketOrderIndicator(
	ctx sdk.Context,
	marketID common.Hash,
	isBuy bool,
) *v2.MarketOrderIndicator {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetTransientMarketOrderIndicator")()

	store := k.getTransientStore(ctx)
	marketQuantityStore := prefix.NewStore(store, types.SpotMarketOrderIndicatorPrefix)
	quantityKey := types.MarketDirectionPrefix(marketID, isBuy)
	bz := marketQuantityStore.Get(quantityKey)
	if bz == nil {
		return &v2.MarketOrderIndicator{
			MarketId: marketID.Hex(),
			IsBuy:    isBuy,
		}
	}
	var marketQuantity v2.MarketOrderIndicator
	k.cdc.MustUnmarshal(bz, &marketQuantity)

	return &marketQuantity
}

// GetTransientMarketOrderIndicator sets the transient market order indicator in the transient store.
func (k *BaseKeeper) SetTransientMarketOrderIndicator(
	ctx sdk.Context,
	marketID common.Hash,
	isBuy bool,
) {
	defer k.Meter(ctx).FuncTiming(&ctx, "SetTransientMarketOrderIndicator")()

	store := k.getTransientStore(ctx)
	marketIndicatorStore := prefix.NewStore(store, types.SpotMarketOrderIndicatorPrefix)
	quantityKey := types.MarketDirectionPrefix(marketID, isBuy)
	marketOrderIndicator := &v2.MarketOrderIndicator{
		MarketId: marketID.Hex(),
		IsBuy:    isBuy,
	}
	bz := k.cdc.MustMarshal(marketOrderIndicator)
	marketIndicatorStore.Set(quantityKey, bz)
}

// GetAllTransientSpotMarketOrderIndicators iterates over all of a spot market's marketID directions for this block.
func (k *BaseKeeper) GetAllTransientSpotMarketOrderIndicators(
	ctx sdk.Context,
) []*v2.MarketOrderIndicator {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetAllTransientSpotMarketOrderIndicators")()

	store := k.getTransientStore(ctx)
	marketQuantityStore := prefix.NewStore(store, types.SpotMarketOrderIndicatorPrefix)

	marketQuantities := make([]*v2.MarketOrderIndicator, 0)

	iterateSafe(marketQuantityStore.Iterator(nil, nil), func(_, v []byte) bool {
		var marketQuantity v2.MarketOrderIndicator
		k.cdc.MustUnmarshal(v, &marketQuantity)
		marketQuantities = append(marketQuantities, &marketQuantity)
		return false
	})

	return marketQuantities
}

func (k *BaseKeeper) UpdateSpotLimitOrderWithDelta(
	ctx sdk.Context,
	marketID common.Hash,
	orderDelta *v2.SpotLimitOrderDelta,
) {
	defer k.Meter(ctx).FuncTiming(&ctx, "UpdateSpotLimitOrderWithDelta")()

	store := k.getStore(ctx)
	ordersStore := prefix.NewStore(store, types.SpotLimitOrdersPrefix)
	priceKey := types.GetLimitOrderByPriceKeyPrefix(
		marketID,
		orderDelta.Order.IsBuy(),
		orderDelta.Order.GetPrice(),
		orderDelta.Order.Hash(),
	)

	orderBz := k.cdc.MustMarshal(orderDelta.Order)
	ordersStore.Set(priceKey, orderBz)
}
