//go:build !injective_release

package exchange

// Test-only exports for `abci.go` internals. Excluded from the production
// binary by the `injective_release` build tag set by the `install-injectived`
// Makefile target. `go test ./...` continues to work without flags because
// the default build does not set `injective_release`.

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"

	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/types/v2"
)

// CleanupTransientSpotMarketOrdersForTest exposes the recovered-panic spot
// market-order cleanup so tests can pin side-specific refund behavior.
func (h *BlockHandler) CleanupTransientSpotMarketOrdersForTest(
	ctx sdk.Context,
	batchData []*v2.SpotBatchExecutionData,
	indicators []*v2.MarketOrderIndicator,
) {
	h.cleanupTransientSpotMarketOrders(ctx, batchData, indicators)
}

func (h *BlockHandler) CancelConditionalOrdersForMarketForTest(
	ctx sdk.Context,
	triggeredMarket *v2.TriggeredOrdersInMarket,
) {
	h.cancelConditionalOrdersForMarket(ctx, triggeredMarket)
}

func (h *BlockHandler) CancelConditionalMarketOrdersForTest(
	ctx sdk.Context,
	triggeredMarket *v2.TriggeredOrdersInMarket,
) {
	h.cancelTriggeredMarketOrdersForMarket(ctx, triggeredMarket)
}

func (h *BlockHandler) InvalidateConditionalOrdersIfNoMarginLockedForTest(
	ctx sdk.Context,
	marketID, subaccountID common.Hash,
	isBuy bool,
	marketCache map[common.Hash]*v2.DerivativeMarket,
) {
	h.invalidateConditionalOrdersIfNoMarginLocked(ctx, marketID, subaccountID, isBuy, marketCache)
}
