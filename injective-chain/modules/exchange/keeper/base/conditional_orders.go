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

func (k *BaseKeeper) SetConditionalDerivativeMarketOrder(
	ctx sdk.Context,
	order *v2.DerivativeMarketOrder,
	marketID common.Hash,
	markPrice math.LegacyDec,
) {
	defer k.Meter(ctx).FuncTiming(&ctx, "SetConditionalDerivativeMarketOrder")()

	store := k.getStore(ctx)
	ordersStore := prefix.NewStore(store, types.DerivativeConditionalMarketOrdersPrefix)
	ordersIndexStore := prefix.NewStore(store, types.DerivativeConditionalMarketOrdersIndexPrefix)

	isTriggerPriceHigher := order.TriggerPrice.GT(markPrice)
	triggerPrice := *order.TriggerPrice

	priceKey := types.GetConditionalOrderByTriggerPriceKeyPrefix(marketID, isTriggerPriceHigher, triggerPrice, order.Hash())
	subaccountIndexKey := types.GetLimitOrderIndexKey(marketID, isTriggerPriceHigher, order.SubaccountID(), order.Hash())

	orderBz := k.cdc.MustMarshal(order)
	ordersIndexStore.Set(subaccountIndexKey, triggerPrice.BigInt().Bytes())
	ordersStore.Set(priceKey, orderBz)

	k.SetCid(ctx, false, order.SubaccountID(), order.OrderInfo.Cid, marketID, order.IsBuy(), order.Hash())
	k.SetActiveDerivativeOrderMarketForSubaccount(ctx, order.SubaccountID(), marketID)
}

func (k *BaseKeeper) SetConditionalDerivativeLimitOrder(
	ctx sdk.Context,
	order *v2.DerivativeLimitOrder,
	marketID common.Hash,
	markPrice math.LegacyDec,
) {
	defer k.Meter(ctx).FuncTiming(&ctx, "SetConditionalDerivativeLimitOrder")()

	store := k.getStore(ctx)
	ordersStore := prefix.NewStore(store, types.DerivativeConditionalLimitOrdersPrefix)
	ordersIndexStore := prefix.NewStore(store, types.DerivativeConditionalLimitOrdersIndexPrefix)

	isTriggerPriceHigher := order.TriggerPrice.GT(markPrice)
	triggerPrice := *order.TriggerPrice

	priceKey := types.GetConditionalOrderByTriggerPriceKeyPrefix(marketID, isTriggerPriceHigher, triggerPrice, order.Hash())
	subaccountIndexKey := types.GetLimitOrderIndexKey(marketID, isTriggerPriceHigher, order.SubaccountID(), order.Hash())

	orderBz := k.cdc.MustMarshal(order)
	ordersIndexStore.Set(subaccountIndexKey, triggerPrice.BigInt().Bytes())
	ordersStore.Set(priceKey, orderBz)

	k.SetCid(ctx, false, order.SubaccountID(), order.OrderInfo.Cid, marketID, order.IsBuy(), order.Hash())
	k.SetActiveDerivativeOrderMarketForSubaccount(ctx, order.SubaccountID(), marketID)
}

// DeleteConditionalDerivativeOrder deletes the conditional derivative order (market or limit).
func (k *BaseKeeper) DeleteConditionalDerivativeOrder( //nolint:revive // ok
	ctx sdk.Context,
	isLimit bool,
	marketID common.Hash,
	subaccountID common.Hash,
	isTriggerPriceHigher bool,
	triggerPrice math.LegacyDec,
	orderHash common.Hash,
	orderCid string,
) {
	defer k.Meter(ctx).FuncTiming(&ctx, "DeleteConditionalDerivativeOrder")()

	var (
		ordersStore      prefix.Store
		ordersIndexStore prefix.Store
	)

	store := k.getStore(ctx)
	if isLimit {
		ordersStore = prefix.NewStore(store, types.DerivativeConditionalLimitOrdersPrefix)
		ordersIndexStore = prefix.NewStore(store, types.DerivativeConditionalLimitOrdersIndexPrefix)
	} else {
		ordersStore = prefix.NewStore(store, types.DerivativeConditionalMarketOrdersPrefix)
		ordersIndexStore = prefix.NewStore(store, types.DerivativeConditionalMarketOrdersIndexPrefix)
	}

	priceKey := types.GetOrderByPriceKeyPrefix(marketID, isTriggerPriceHigher, triggerPrice, orderHash)
	subaccountIndexKey := types.GetLimitOrderIndexKey(marketID, isTriggerPriceHigher, subaccountID, orderHash)

	// delete main derivative order store
	ordersStore.Delete(priceKey)

	// delete from subaccount index key store
	ordersIndexStore.Delete(subaccountIndexKey)

	k.DeleteCid(ctx, false, subaccountID, orderCid)

	// NOTE: MaybeDeleteActiveDerivativeOrderMarketForSubaccount is NOT called here because
	// SubaccountOrderbookMetadata (used by hasAnyDerivativeOrdersInMarketForSubaccount) may not
	// yet reflect this deletion. Callers must call it after metadata is applied.
}

// GetConditionalDerivativeLimitOrderBySubaccountIDAndHash returns the active conditional derivative limit order from hash and subaccountID.
func (k *BaseKeeper) GetConditionalDerivativeLimitOrderBySubaccountIDAndHash(
	ctx sdk.Context,
	marketID common.Hash,
	isTriggerPriceHigher *bool,
	subaccountID common.Hash,
	orderHash common.Hash,
) (order *v2.DerivativeLimitOrder, direction bool) {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetConditionalDerivativeLimitOrderBySubaccountIDAndHash")()

	store := k.getStore(ctx)
	ordersStore := prefix.NewStore(store, types.DerivativeConditionalLimitOrdersPrefix)
	ordersIndexStore := prefix.NewStore(store, types.DerivativeConditionalLimitOrdersIndexPrefix)

	triggerPriceKey, direction := fetchPriceKeyFromOrdersIndexStore(
		ordersIndexStore,
		marketID,
		isTriggerPriceHigher,
		subaccountID,
		orderHash,
	)

	if triggerPriceKey == nil {
		return nil, false
	}

	// Fetch LimitOrder from ordersStore
	triggerPrice := types.UnsignedDecBytesToDec(triggerPriceKey)

	orderBz := ordersStore.Get(types.GetOrderByStringPriceKeyPrefix(
		marketID,
		direction,
		triggerPrice.String(),
		orderHash,
	))
	if orderBz == nil {
		return nil, false
	}

	var orderObj v2.DerivativeLimitOrder
	k.cdc.MustUnmarshal(orderBz, &orderObj)

	return &orderObj, direction
}

// GetConditionalDerivativeMarketOrderBySubaccountIDAndHash returns the active conditional derivative limit order
// from hash and subaccountID.
func (k *BaseKeeper) GetConditionalDerivativeMarketOrderBySubaccountIDAndHash( //nolint:revive // ok
	ctx sdk.Context,
	marketID common.Hash,
	isTriggerPriceHigher *bool,
	subaccountID common.Hash,
	orderHash common.Hash,
) (order *v2.DerivativeMarketOrder, direction bool) {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetConditionalDerivativeMarketOrderBySubaccountIDAndHash")()

	store := k.getStore(ctx)
	ordersStore := prefix.NewStore(store, types.DerivativeConditionalMarketOrdersPrefix)
	ordersIndexStore := prefix.NewStore(store, types.DerivativeConditionalMarketOrdersIndexPrefix)

	triggerPriceKey, direction := fetchPriceKeyFromOrdersIndexStore(
		ordersIndexStore,
		marketID,
		isTriggerPriceHigher,
		subaccountID,
		orderHash,
	)

	if triggerPriceKey == nil {
		return nil, false
	}

	// Fetch LimitOrder from ordersStore
	triggerPrice := types.UnsignedDecBytesToDec(triggerPriceKey)

	orderBz := ordersStore.Get(types.GetOrderByStringPriceKeyPrefix(marketID, direction, triggerPrice.String(), orderHash))
	if orderBz == nil {
		return nil, false
	}

	var orderObj v2.DerivativeMarketOrder
	k.cdc.MustUnmarshal(orderBz, &orderObj)
	return &orderObj, direction
}

func (k *BaseKeeper) GetAllConditionalDerivativeMarketOrdersInMarketUpToPrice(
	ctx sdk.Context,
	marketID common.Hash,
	triggerPrice *math.LegacyDec,
) (marketBuyOrders, marketSellOrders []*v2.DerivativeMarketOrder) {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetAllConditionalDerivativeMarketOrdersInMarketUpToPrice")()

	marketBuyOrders = make([]*v2.DerivativeMarketOrder, 0)
	marketSellOrders = make([]*v2.DerivativeMarketOrder, 0)

	store := k.getStore(ctx)
	appendMarketOrder := func(orderKey []byte) (stop bool) {
		var order v2.DerivativeMarketOrder
		k.cdc.MustUnmarshal(store.Get(orderKey), &order)

		if order.IsBuy() {
			marketBuyOrders = append(marketBuyOrders, &order)
		} else {
			marketSellOrders = append(marketSellOrders, &order)
		}

		return false
	}

	k.iterateConditionalDerivativeOrders(ctx, marketID, true, true, triggerPrice, appendMarketOrder)
	k.iterateConditionalDerivativeOrders(ctx, marketID, false, true, triggerPrice, appendMarketOrder)

	return marketBuyOrders, marketSellOrders
}

func (k *BaseKeeper) GetAllConditionalDerivativeLimitOrdersInMarketUpToPrice(
	ctx sdk.Context,
	marketID common.Hash,
	triggerPrice *math.LegacyDec,
) (limitBuyOrders, limitSellOrders []*v2.DerivativeLimitOrder) {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetAllConditionalDerivativeLimitOrdersInMarketUpToPrice")()

	limitBuyOrders = make([]*v2.DerivativeLimitOrder, 0)
	limitSellOrders = make([]*v2.DerivativeLimitOrder, 0)

	store := k.getStore(ctx)
	appendLimitOrder := func(orderKey []byte) (stop bool) {
		bz := store.Get(orderKey)
		// Unmarshal order
		var order v2.DerivativeLimitOrder
		k.cdc.MustUnmarshal(bz, &order)

		if order.IsBuy() {
			limitBuyOrders = append(limitBuyOrders, &order)
		} else {
			limitSellOrders = append(limitSellOrders, &order)
		}

		return false
	}

	k.iterateConditionalDerivativeOrders(ctx, marketID, true, false, triggerPrice, appendLimitOrder)
	k.iterateConditionalDerivativeOrders(ctx, marketID, false, false, triggerPrice, appendLimitOrder)

	return limitBuyOrders, limitSellOrders
}

// IterateConditionalOrdersBySubaccount iterates over all placed conditional orders in the given market
// for the subaccount, in 'isTriggerPriceHigher' direction with market / limit order type
//
//nolint:revive // ok
func (k *BaseKeeper) IterateConditionalOrdersBySubaccount(
	ctx sdk.Context,
	marketID,
	subaccountID common.Hash,
	isTriggerPriceHigher bool,
	isMarketOrders bool,
	process func(orderHash common.Hash) (stop bool),
) {
	defer k.Meter(ctx).FuncTiming(&ctx, "IterateConditionalOrdersBySubaccount")()

	var (
		iterator     storetypes.Iterator
		ordersPrefix []byte
	)

	if isMarketOrders {
		ordersPrefix = types.DerivativeConditionalMarketOrdersIndexPrefix
	} else {
		ordersPrefix = types.DerivativeConditionalLimitOrdersIndexPrefix
	}

	ordersPrefix = append(ordersPrefix, types.GetLimitOrderIndexSubaccountPrefix(marketID, isTriggerPriceHigher, subaccountID)...)

	store := k.getStore(ctx)
	orderStore := prefix.NewStore(store, ordersPrefix)

	if isTriggerPriceHigher {
		iterator = orderStore.Iterator(nil, nil)
	} else {
		iterator = orderStore.ReverseIterator(nil, nil)
	}

	orderKeyBz := ordersPrefix

	iterateSafe(iterator, func(key, _ []byte) bool {
		orderKey := make([]byte, 0, len(orderKeyBz)+len(key))
		orderKey = append(orderKey, orderKeyBz...)
		orderKey = append(orderKey, key...)
		orderHash := getOrderHashFromDerivativeOrderIndexKey(orderKey)

		return process(orderHash)
	})
}

// markForConditionalOrderInvalidation stores the flag in transient store that this subaccountID has invalid
// RO conditional orders for the market it is supposed to be read in the EndBlocker
func (k *BaseKeeper) markForConditionalOrderInvalidation(ctx sdk.Context, marketID, subaccountID common.Hash, isBuy bool) {
	defer k.Meter(ctx).FuncTiming(&ctx, "markForConditionalOrderInvalidation")()

	store := k.getTransientStore(ctx)
	flagsStore := prefix.NewStore(store, types.ConditionalOrderInvalidationFlagPrefix)
	key := types.GetSubaccountOrderSuffix(marketID, subaccountID, isBuy)
	flagsStore.Set(key, []byte{})
}

func (k *BaseKeeper) removeConditionalOrderInvalidationFlag(ctx sdk.Context, marketID, subaccountID common.Hash, isBuy bool) {
	defer k.Meter(ctx).FuncTiming(&ctx, "removeConditionalOrderInvalidationFlag")()

	store := k.getTransientStore(ctx)
	flagsStore := prefix.NewStore(store, types.ConditionalOrderInvalidationFlagPrefix)
	key := types.GetSubaccountOrderSuffix(marketID, subaccountID, isBuy)
	flagsStore.Delete(key)
}

func (k *BaseKeeper) IterateInvalidConditionalOrderFlags(
	ctx sdk.Context, process func(marketID, subaccountID common.Hash, isBuy bool) (stop bool),
) {
	defer k.Meter(ctx).FuncTiming(&ctx, "IterateInvalidConditionalOrderFlags")()

	store := k.getTransientStore(ctx)
	flagsStore := prefix.NewStore(store, types.ConditionalOrderInvalidationFlagPrefix)

	keys := [][]byte{}

	iterateKeysSafe(flagsStore.Iterator(nil, nil), func(k []byte) bool {
		keys = append(keys, k)
		return false
	})

	for _, key := range keys {
		marketID, subaccountID, isBuy := types.ParseMarketIDSubaccountIDDirectionSuffix(key)
		if process(marketID, subaccountID, isBuy) {
			return
		}
	}
}

// IterateConditionalDerivativeOrders iterates over all placed conditional orders in the given market,
// in 'isTriggerPriceHigher' direction with market / limit order type up to the price of priceRangeEnd
// (exclusive, optional)
//
//nolint:revive // ok
func (k *BaseKeeper) iterateConditionalDerivativeOrders(
	ctx sdk.Context,
	marketID common.Hash,
	isTriggerPriceHigher bool,
	isMarketOrders bool,
	triggerPrice *math.LegacyDec,
	process func(orderKey []byte) (stop bool),
) {
	defer k.Meter(ctx).FuncTiming(&ctx, "iterateConditionalDerivativeOrders")()

	var (
		iterator     storetypes.Iterator
		ordersPrefix []byte
	)

	if isMarketOrders {
		ordersPrefix = types.DerivativeConditionalMarketOrdersPrefix
	} else {
		ordersPrefix = types.DerivativeConditionalLimitOrdersPrefix
	}
	ordersPrefix = append(ordersPrefix, types.MarketDirectionPrefix(marketID, isTriggerPriceHigher)...)

	store := k.getStore(ctx)
	orderStore := prefix.NewStore(store, ordersPrefix)

	if isTriggerPriceHigher {
		var iteratorEnd []byte
		if triggerPrice != nil {
			iteratorEnd = AddBitToPrefix([]byte(types.GetPaddedPrice(*triggerPrice))) // we need inclusive end
		}
		iterator = orderStore.Iterator(nil, iteratorEnd)
	} else {
		var iteratorStart []byte
		if triggerPrice != nil {
			iteratorStart = []byte(types.GetPaddedPrice(*triggerPrice))
		}
		iterator = orderStore.ReverseIterator(iteratorStart, nil)
	}

	orderKeyBz := ordersPrefix

	iterateSafe(iterator, func(key, _ []byte) bool {
		orderKey := make([]byte, 0, len(orderKeyBz)+len(key))
		orderKey = append(orderKey, orderKeyBz...)
		orderKey = append(orderKey, key...)

		return process(orderKey)
	})
}
