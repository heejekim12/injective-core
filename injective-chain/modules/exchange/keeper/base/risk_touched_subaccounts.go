package base

import (
	"bytes"
	"slices"

	"cosmossdk.io/store/prefix"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"

	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/types"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/types/v2"
)

// extractMarketAndSubaccountFromKey extracts the marketID and subaccountID from a composite indicator key
// with layout [marketID (32 bytes)][subaccountID (32 bytes)].
func extractMarketAndSubaccountFromKey(key []byte) (marketID, subaccountID common.Hash, ok bool) {
	if len(key) < 2*common.HashLength {
		return common.Hash{}, common.Hash{}, false
	}
	return common.BytesToHash(key[:common.HashLength]),
		common.BytesToHash(key[common.HashLength : 2*common.HashLength]),
		true
}

// GetRiskTouchedSubaccounts returns the canonical (sorted) list of subaccounts that the risk pre-pass should
// consider for this block.
//
// The touched set is derived from bounded transient sources (set union):
//  1. Derivative market order placement indicators (including conditional market orders materialized in EndBlocker).
//     - Source: `types.SubaccountMarketOrderIndicatorPrefix`.
//  2. Derivative limit order placement indicators.
//     - Source: `types.SubaccountLimitOrderIndicatorPrefix`.
//  3. Subaccounts with derivative positions modified earlier in the block (recorded per market).
//     - Source: `types.DerivativePositionModifiedSubaccountPrefix`.
//
// This intentionally does not attempt to scan the full global orderbook; it is used to scope best-effort
// caching and cross-margin bookkeeping to current-block activity only.
func (k *BaseKeeper) GetRiskTouchedSubaccounts(ctx sdk.Context) []common.Hash {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetRiskTouchedSubaccounts")()

	touched := make(map[common.Hash]struct{})
	store := k.getTransientStore(ctx)

	marketOrderIndicatorStore := prefix.NewStore(store, types.SubaccountMarketOrderIndicatorPrefix)
	iterateKeysSafe(marketOrderIndicatorStore.Iterator(nil, nil), func(key []byte) (stop bool) {
		if _, subaccountID, ok := extractMarketAndSubaccountFromKey(key); ok {
			touched[subaccountID] = struct{}{}
		}
		return false
	})

	limitOrderIndicatorStore := prefix.NewStore(store, types.SubaccountLimitOrderIndicatorPrefix)
	iterateKeysSafe(limitOrderIndicatorStore.Iterator(nil, nil), func(key []byte) (stop bool) {
		if _, subaccountID, ok := extractMarketAndSubaccountFromKey(key); ok {
			touched[subaccountID] = struct{}{}
		}
		return false
	})

	modifiedPositionsStore := prefix.NewStore(store, types.DerivativePositionModifiedSubaccountPrefix)
	iterateSafe(modifiedPositionsStore.Iterator(nil, nil), func(_, value []byte) (stop bool) {
		var subaccountIDs v2.SubaccountIDs
		k.cdc.MustUnmarshal(value, &subaccountIDs)
		for _, subaccountIDBz := range subaccountIDs.SubaccountIds {
			if len(subaccountIDBz) != common.HashLength {
				continue
			}
			touched[common.BytesToHash(subaccountIDBz)] = struct{}{}
		}
		return false
	})

	ids := make([]common.Hash, 0, len(touched))
	for subaccountID := range touched {
		ids = append(ids, subaccountID)
	}

	slices.SortFunc(ids, func(a, b common.Hash) int {
		return bytes.Compare(a.Bytes(), b.Bytes())
	})

	return ids
}

// GetTransientDerivativeOrderIndicatorMarketsBySubaccount returns the canonical (sorted) set of derivative market IDs
// that the given subaccount has interacted with in the current block via non-atomic order placement.
//
// This is derived from the same transient indicator sources used by GetRiskTouchedSubaccounts, but filtered for a single
// subaccount. It is used by cross-margin liquidations to cancel transient orders across the quote-denom pool without
// scanning global state.
func (k *BaseKeeper) GetTransientDerivativeOrderIndicatorMarketsBySubaccount(ctx sdk.Context, subaccountID common.Hash) []common.Hash {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetTransientDerivativeOrderIndicatorMarketsBySubaccount")()

	marketIDs := make(map[common.Hash]struct{})
	store := k.getTransientStore(ctx)

	addMarketsFromIndicatorStore := func(indicatorPrefix []byte) {
		indicatorStore := prefix.NewStore(store, indicatorPrefix)
		iterateKeysSafe(indicatorStore.Iterator(nil, nil), func(key []byte) (stop bool) {
			marketID, sid, ok := extractMarketAndSubaccountFromKey(key)
			if ok && sid == subaccountID {
				marketIDs[marketID] = struct{}{}
			}
			return false
		})
	}

	addMarketsFromIndicatorStore(types.SubaccountMarketOrderIndicatorPrefix)
	addMarketsFromIndicatorStore(types.SubaccountLimitOrderIndicatorPrefix)

	ids := make([]common.Hash, 0, len(marketIDs))
	for marketID := range marketIDs {
		ids = append(ids, marketID)
	}

	slices.SortFunc(ids, func(a, b common.Hash) int {
		return bytes.Compare(a.Bytes(), b.Bytes())
	})

	return ids
}
