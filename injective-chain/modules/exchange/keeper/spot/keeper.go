package spot

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	"github.com/ethereum/go-ethereum/common"

	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/keeper/base"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/keeper/feediscounts"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/keeper/rewards"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/keeper/subaccount"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/risk"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/types/v2"
)

//nolint:revive // ok
type SpotKeeper struct {
	*base.BaseKeeper

	subaccount     *subaccount.SubaccountKeeper
	bank           bankkeeper.Keeper
	tradingRewards *rewards.TradingKeeper
	feeDiscounts   *feediscounts.FeeDiscountsKeeper
	riskEngine     *risk.Engine
}

func New(
	b *base.BaseKeeper,
	re *risk.Engine,
	bk bankkeeper.Keeper,
	sa *subaccount.SubaccountKeeper,
	tk *rewards.TradingKeeper,
	fd *feediscounts.FeeDiscountsKeeper,
) *SpotKeeper {
	return &SpotKeeper{
		BaseKeeper:     b,
		bank:           bk,
		subaccount:     sa,
		tradingRewards: tk,
		feeDiscounts:   fd,
		riskEngine:     re,
	}
}

// GetFeeDiscountConfigForMarket returns the fee discount configuration for a market.
// This is used by the FBA package to process spot matching results.
func (k SpotKeeper) GetFeeDiscountConfigForMarket(ctx sdk.Context, marketID common.Hash, stakingInfo *v2.FeeDiscountStakingInfo) *v2.FeeDiscountConfig {
	return k.feeDiscounts.GetFeeDiscountConfigForMarket(ctx, marketID, stakingInfo)
}

func (k SpotKeeper) RiskEngine() risk.ReadOnlyEngine {
	return k.riskEngine
}
