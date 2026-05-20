package base

import (
	"cosmossdk.io/store/prefix"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"

	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/risk"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/types"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/types/v2"
)

// Exposure indexes are derived state to avoid scanning all markets when building
// per-subaccount cross-margin snapshots and when cancelling orders across a subaccount's
// quote-denom pool during liquidations.

// activeDerivativeMarketsKey builds a safe prefix key for the subaccount's active derivative markets index.
func activeDerivativeMarketsKey(subaccountID common.Hash) []byte {
	key := make([]byte, 0, len(types.ActiveDerivativeMarketsBySubaccountPrefix)+common.HashLength)
	key = append(key, types.ActiveDerivativeMarketsBySubaccountPrefix...)
	key = append(key, subaccountID.Bytes()...)
	return key
}

// activeDerivativeOrderMarketsKey builds a safe prefix key for the subaccount's active derivative order markets index.
func activeDerivativeOrderMarketsKey(subaccountID common.Hash) []byte {
	key := make([]byte, 0, len(types.ActiveDerivativeOrderMarketsBySubaccountPrefix)+common.HashLength)
	key = append(key, types.ActiveDerivativeOrderMarketsBySubaccountPrefix...)
	key = append(key, subaccountID.Bytes()...)
	return key
}

func (k *BaseKeeper) SetActiveDerivativeMarketForSubaccount(ctx sdk.Context, subaccountID, marketID common.Hash) {
	defer k.Meter(ctx).FuncTiming(&ctx, "SetActiveDerivativeMarketForSubaccount")()

	store := k.getStore(ctx)
	subStore := prefix.NewStore(store, activeDerivativeMarketsKey(subaccountID))
	subStore.Set(marketID.Bytes(), []byte{})
}

func (k *BaseKeeper) DeleteActiveDerivativeMarketForSubaccount(ctx sdk.Context, subaccountID, marketID common.Hash) {
	defer k.Meter(ctx).FuncTiming(&ctx, "DeleteActiveDerivativeMarketForSubaccount")()

	store := k.getStore(ctx)
	subStore := prefix.NewStore(store, activeDerivativeMarketsKey(subaccountID))
	subStore.Delete(marketID.Bytes())
}

func (k *BaseKeeper) GetActiveDerivativeMarketsBySubaccount(ctx sdk.Context, subaccountID common.Hash) []common.Hash {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetActiveDerivativeMarketsBySubaccount")()

	store := k.getStore(ctx)
	subStore := prefix.NewStore(store, activeDerivativeMarketsKey(subaccountID))

	marketIDs := make([]common.Hash, 0, 4)
	iterateKeysSafe(subStore.Iterator(nil, nil), func(key []byte) bool {
		marketIDs = append(marketIDs, common.BytesToHash(key))
		return false
	})

	return marketIDs
}

func (k *BaseKeeper) SetActiveDerivativeOrderMarketForSubaccount(ctx sdk.Context, subaccountID, marketID common.Hash) {
	defer k.Meter(ctx).FuncTiming(&ctx, "SetActiveDerivativeOrderMarketForSubaccount")()

	store := k.getStore(ctx)
	subStore := prefix.NewStore(store, activeDerivativeOrderMarketsKey(subaccountID))
	subStore.Set(marketID.Bytes(), []byte{})
}

func (k *BaseKeeper) DeleteActiveDerivativeOrderMarketForSubaccount(ctx sdk.Context, subaccountID, marketID common.Hash) {
	defer k.Meter(ctx).FuncTiming(&ctx, "DeleteActiveDerivativeOrderMarketForSubaccount")()

	store := k.getStore(ctx)
	subStore := prefix.NewStore(store, activeDerivativeOrderMarketsKey(subaccountID))
	subStore.Delete(marketID.Bytes())
}

func (k *BaseKeeper) GetActiveDerivativeOrderMarketsBySubaccount(ctx sdk.Context, subaccountID common.Hash) []common.Hash {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetActiveDerivativeOrderMarketsBySubaccount")()

	store := k.getStore(ctx)
	subStore := prefix.NewStore(store, activeDerivativeOrderMarketsKey(subaccountID))

	marketIDs := make([]common.Hash, 0, 4)
	iterateKeysSafe(subStore.Iterator(nil, nil), func(key []byte) bool {
		marketIDs = append(marketIDs, common.BytesToHash(key))
		return false
	})

	return marketIDs
}

// GetAllActiveDerivativeMarketIDsForSubaccount returns the deduplicated, sorted union of market IDs
// where the subaccount has positions, persistent derivative orders, or transient derivative order indicators.
func (k *BaseKeeper) GetAllActiveDerivativeMarketIDsForSubaccount(ctx sdk.Context, subaccountID common.Hash) []common.Hash {
	return risk.MergeAndSortMarketIDs(
		k.GetActiveDerivativeMarketsBySubaccount(ctx, subaccountID),
		k.GetActiveDerivativeOrderMarketsBySubaccount(ctx, subaccountID),
		k.GetTransientDerivativeOrderIndicatorMarketsBySubaccount(ctx, subaccountID),
	)
}

// MaybeDeleteActiveDerivativeOrderMarketForSubaccount removes the market from the subaccount's
// active derivative order markets index if the subaccount has no remaining orders in that market.
// This should be called after an order is cancelled or fully filled.
func (k *BaseKeeper) MaybeDeleteActiveDerivativeOrderMarketForSubaccount(ctx sdk.Context, subaccountID, marketID common.Hash) {
	defer k.Meter(ctx).FuncTiming(&ctx, "MaybeDeleteActiveDerivativeOrderMarketForSubaccount")()

	if k.hasAnyDerivativeOrdersInMarketForSubaccount(ctx, marketID, subaccountID) {
		return
	}

	store := k.getStore(ctx)
	subStore := prefix.NewStore(store, activeDerivativeOrderMarketsKey(subaccountID))
	subStore.Delete(marketID.Bytes())
}

func (k *BaseKeeper) hasAnyDerivativeOrdersInMarketForSubaccount(ctx sdk.Context, marketID, subaccountID common.Hash) bool {
	// Orderbook metadata tracks counts for all order types (vanilla limit, reduce-only limit,
	// vanilla conditional, reduce-only conditional) per (market, subaccount, direction).
	// Two key reads instead of iterating 8 index stores.
	buyMeta := k.GetSubaccountOrderbookMetadata(ctx, marketID, subaccountID, true)
	if buyMeta.GetOrderSideCount() > 0 {
		return true
	}

	sellMeta := k.GetSubaccountOrderbookMetadata(ctx, marketID, subaccountID, false)
	return sellMeta.GetOrderSideCount() > 0
}

// BackfillActiveDerivativeMarketIndexes populates the ActiveDerivativeMarketsBySubaccount index
// for all existing derivative positions. This is intended to be called during chain upgrades to
// ensure pre-existing positions are tracked in the index for cross-margin eligibility and snapshots.
//
// This function iterates ALL derivative positions and sets the index for each non-zero position.
// It is idempotent - calling it multiple times has no adverse effect.
func (k *BaseKeeper) BackfillActiveDerivativeMarketIndexes(ctx sdk.Context) {
	defer k.Meter(ctx).FuncTiming(&ctx, "BackfillActiveDerivativeMarketIndexes")()

	store := k.getStore(ctx)
	positionStore := prefix.NewStore(store, types.DerivativePositionsPrefix)

	iterateSafe(positionStore.Iterator(nil, nil), func(key, value []byte) bool {
		// Key structure: marketID (32 bytes) + subaccountID (32 bytes)
		if len(key) < 64 {
			return false
		}
		marketID := common.BytesToHash(key[:32])
		subaccountID := common.BytesToHash(key[32:64])

		var position v2.Position
		k.cdc.MustUnmarshal(value, &position)

		if !position.Quantity.IsZero() {
			k.SetActiveDerivativeMarketForSubaccount(ctx, subaccountID, marketID)
		}
		return false
	})
}

// BackfillActiveDerivativeOrderMarketIndexes populates the ActiveDerivativeOrderMarketsBySubaccount index
// for all existing derivative orders (resting limit orders and conditional orders). This is intended to
// be called during chain upgrades to ensure pre-existing orders are tracked in the index for cross-margin
// eligibility and snapshots.
//
// This function iterates the existing order subaccount indexes (DerivativeLimitOrdersIndexPrefix,
// DerivativeConditionalMarketOrdersIndexPrefix, DerivativeConditionalLimitOrdersIndexPrefix) and sets
// the active order market index for each unique (subaccountID, marketID) pair.
// It is idempotent - calling it multiple times has no adverse effect.
//
// Note: Transient orders (market orders) don't persist across blocks and are not backfilled.
func (k *BaseKeeper) BackfillActiveDerivativeOrderMarketIndexes(ctx sdk.Context) {
	defer k.Meter(ctx).FuncTiming(&ctx, "BackfillActiveDerivativeOrderMarketIndexes")()

	store := k.getStore(ctx)

	// Track which (subaccount, market) pairs we've already set to avoid redundant writes.
	seen := make(map[[64]byte]struct{})

	// indexOrderMarket extracts and indexes (subaccountID, marketID) pairs from order index keys.
	// Key structure: marketID (32) + isBuy/direction (1) + subaccountID (32) + orderHash (32)
	indexOrderMarket := func(key []byte) {
		if len(key) < 65 { // marketID + direction + subaccountID minimum
			return
		}
		marketID := common.BytesToHash(key[:32])
		// key[32] is direction (1 byte)
		subaccountID := common.BytesToHash(key[33:65])

		var cacheKey [64]byte
		copy(cacheKey[:32], subaccountID.Bytes())
		copy(cacheKey[32:], marketID.Bytes())

		if _, ok := seen[cacheKey]; !ok {
			seen[cacheKey] = struct{}{}
			k.SetActiveDerivativeOrderMarketForSubaccount(ctx, subaccountID, marketID)
		}
	}

	// Backfill from resting derivative limit orders.
	limitOrdersStore := prefix.NewStore(store, types.DerivativeLimitOrdersIndexPrefix)
	iterateSafe(limitOrdersStore.Iterator(nil, nil), func(key, _ []byte) bool {
		indexOrderMarket(key)
		return false
	})

	// Backfill from conditional derivative market orders.
	condMarketOrdersStore := prefix.NewStore(store, types.DerivativeConditionalMarketOrdersIndexPrefix)
	iterateSafe(condMarketOrdersStore.Iterator(nil, nil), func(key, _ []byte) bool {
		indexOrderMarket(key)
		return false
	})

	// Backfill from conditional derivative limit orders.
	condLimitOrdersStore := prefix.NewStore(store, types.DerivativeConditionalLimitOrdersIndexPrefix)
	iterateSafe(condLimitOrdersStore.Iterator(nil, nil), func(key, _ []byte) bool {
		indexOrderMarket(key)
		return false
	})
}
