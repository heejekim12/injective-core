package oracle

import (
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/InjectiveLabs/injective-core/injective-chain/modules/oracle/keeper"
)

type BlockHandler struct {
	k *keeper.Keeper
}

func NewBlockHandler(k keeper.Keeper) *BlockHandler {
	return &BlockHandler{
		k: &k,
	}
}

func (h *BlockHandler) BeginBlocker(ctx sdk.Context) {
	defer h.k.Meter(ctx).FuncTiming(&ctx, "BeginBlocker")()

	h.k.CleanupHistoricalPriceRecords(ctx)
}
