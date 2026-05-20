package risk

import (
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
)

// Cross-margin last-look pruning needs to account for cancellations done during orderbook traversal.
// The persistent order state is not mutated until the post-matching persistence step, so within a
// matching pass we maintain an in-memory, stage-local view of "remaining" order-lock requirement.
//
// IMPORTANT: this cache must be scoped to a single market matching goroutine to avoid cross-market
// coordination and concurrency races. Derivative matching entrypoints wrap ctx with this cache.
type CrossMarginLastLookCache struct {
	pools map[common.Hash]map[string]*CrossMarginLastLookPoolState
}

type CrossMarginLastLookPoolState struct {
	EquityAdmission      math.LegacyDec
	OrderLockRequirement math.LegacyDec
	MarketLockStateByID  map[common.Hash]*CrossMarginLastLookMarketState
}

type CrossMarginLastLookMarketState struct {
	SignedPosQty math.LegacyDec
	BuyQty       math.LegacyDec
	SellQty      math.LegacyDec
}

type CrossMarginLastLookContextKey struct{}

var ctxKeyCrossMarginLastLook = CrossMarginLastLookContextKey{}

// WithCrossMarginLastLookCache attaches a stage-local cross-margin last-look cache to ctx.
// Callers should create a fresh cache per market matching execution (per goroutine).
func WithCrossMarginLastLookCache(ctx sdk.Context) sdk.Context {
	return ctx.WithValue(ctxKeyCrossMarginLastLook, &CrossMarginLastLookCache{
		pools: make(map[common.Hash]map[string]*CrossMarginLastLookPoolState),
	})
}

func GetCrossMarginLastLookCache(ctx sdk.Context) (*CrossMarginLastLookCache, bool) {
	v := ctx.Value(ctxKeyCrossMarginLastLook)
	if v == nil {
		return nil, false
	}
	cache, ok := v.(*CrossMarginLastLookCache)
	return cache, ok
}

// GetLastLookPoolState retrieves an existing pool state for a (subaccount, quoteDenom) pair
// without creating one. Returns nil and false if the cache is inactive or the pool doesn't exist yet.
func GetLastLookPoolState(
	ctx sdk.Context,
	subaccountID common.Hash,
	quoteDenom string,
) (*CrossMarginLastLookPoolState, bool) {
	llCache, ok := GetCrossMarginLastLookCache(ctx)
	if !ok || llCache == nil {
		return nil, false
	}

	byDenom, ok := llCache.pools[subaccountID]
	if !ok {
		return nil, false
	}

	pool := byDenom[quoteDenom]
	if pool == nil {
		return nil, false
	}

	return pool, true
}

// GetOrCreateLastLookPoolState retrieves or initialises the pool state for a (subaccount, quoteDenom) pair
// within the last-look cache. Returns the pool state and true if the cache is active, otherwise nil and false.
func GetOrCreateLastLookPoolState(
	ctx sdk.Context,
	subaccountID common.Hash,
	quoteDenom string,
	snapshot *CrossPoolSnapshot,
) (*CrossMarginLastLookPoolState, bool) {
	llCache, ok := GetCrossMarginLastLookCache(ctx)
	if !ok || llCache == nil {
		return nil, false
	}

	byDenom, ok := llCache.pools[subaccountID]
	if !ok {
		byDenom = make(map[string]*CrossMarginLastLookPoolState)
		llCache.pools[subaccountID] = byDenom
	}

	pool := byDenom[quoteDenom]
	if pool == nil {
		pool = &CrossMarginLastLookPoolState{
			EquityAdmission:      snapshot.EquityAdmission,
			OrderLockRequirement: snapshot.OrderLockRequirement,
			MarketLockStateByID:  make(map[common.Hash]*CrossMarginLastLookMarketState),
		}
		byDenom[quoteDenom] = pool
	}

	return pool, true
}
