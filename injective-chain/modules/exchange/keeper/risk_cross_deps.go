package keeper

import (
	"cosmossdk.io/math"
	storetypes "cosmossdk.io/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"

	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/keeper/base"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/keeper/derivative"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/risk"
	exchangetypes "github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/types"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/types/v2"
)

// riskCrossDeps wires keeper-level state access into the risk engine without creating
// a package import cycle. It must remain read-only and deterministic.
type riskCrossDeps struct {
	base       *base.BaseKeeper
	derivative *derivative.DerivativeKeeper
}

func (d riskCrossDeps) QuoteBalance(ctx sdk.Context, subaccountID common.Hash, quoteDenom string) math.LegacyDec {
	deposit := d.base.GetDeposit(ctx, subaccountID, quoteDenom)
	// Use AvailableBalance so cross-margin equity reflects spot holds and other commitments.
	// This ensures derivative admission checks "see" collateral already reserved by spot orders.
	//
	// NOTE: Bank balance is intentionally excluded. For default subaccounts, bank funds can be
	// moved out via MsgSend without exchange-module risk checks. Counting them as cross collateral
	// would allow equity to drop below maintenance via unguarded bank sends, creating avoidable
	// bad debt before liquidation. Users must explicitly deposit to the exchange module.
	return deposit.AvailableBalance
}

func (d riskCrossDeps) ActiveDerivativeMarketsBySubaccount(ctx sdk.Context, subaccountID common.Hash) []common.Hash {
	return d.base.GetActiveDerivativeMarketsBySubaccount(ctx, subaccountID)
}

func (d riskCrossDeps) ActiveDerivativeOrderMarketsBySubaccount(ctx sdk.Context, subaccountID common.Hash) []common.Hash {
	return d.base.GetActiveDerivativeOrderMarketsBySubaccount(ctx, subaccountID)
}

func (d riskCrossDeps) TransientDerivativeOrderIndicatorMarketsBySubaccount(ctx sdk.Context, subaccountID common.Hash) []common.Hash {
	return d.base.GetTransientDerivativeOrderIndicatorMarketsBySubaccount(ctx, subaccountID)
}

func (d riskCrossDeps) DerivativeMarketInfo(
	ctx sdk.Context,
	marketID common.Hash,
) (market *v2.DerivativeMarket, markPrice math.LegacyDec, funding *v2.PerpetualMarketFunding) {
	// Try enabled markets first, then fall back to disabled/paused markets.
	// This ensures cross-margin snapshots account for positions/orders in markets that
	// have been disabled but still have outstanding exposure.
	market, markPrice = d.derivative.GetDerivativeMarketWithMarkPrice(ctx, marketID, true)
	if market == nil {
		// Fallback to disabled markets - positions may still exist there.
		market, markPrice = d.derivative.GetDerivativeMarketWithMarkPrice(ctx, marketID, false)
	}
	if market == nil {
		return nil, math.LegacyDec{}, nil
	}

	if market.IsPerpetual {
		funding = d.derivative.GetPerpetualMarketFunding(ctx, marketID)
	}

	return market, markPrice, funding
}

func (d riskCrossDeps) DerivativeMarket(ctx sdk.Context, marketID common.Hash) *v2.DerivativeMarket {
	return d.derivative.GetDerivativeMarketByID(ctx, marketID)
}

func (d riskCrossDeps) Position(ctx sdk.Context, marketID, subaccountID common.Hash) *v2.Position {
	return d.base.GetPosition(ctx, marketID, subaccountID)
}

func (d riskCrossDeps) SubaccountOrderbookMetadata(ctx sdk.Context, marketID, subaccountID common.Hash, isBuy bool) *v2.SubaccountOrderbookMetadata {
	return d.base.GetSubaccountOrderbookMetadata(ctx, marketID, subaccountID, isBuy)
}

func (d riskCrossDeps) IterateSubaccountOrders(
	ctx sdk.Context,
	marketID common.Hash,
	subaccountID common.Hash,
	isBuy bool,
	process func(order *v2.SubaccountOrder) (stop bool),
) {
	d.base.IterateSubaccountOrdersStartingFromOrder(ctx, marketID, subaccountID, isBuy, true, nil, func(order *v2.SubaccountOrder, _ common.Hash) (stop bool) {
		return process(order)
	})
}

func (d riskCrossDeps) GetTransientDerivativeMarketOrderForSubaccount(ctx sdk.Context, marketID, subaccountID common.Hash, isBuy bool) *v2.DerivativeMarketOrder {
	return d.base.GetTransientDerivativeMarketOrderForSubaccount(ctx, marketID, subaccountID, isBuy)
}

func (d riskCrossDeps) HasTransientDerivativeMarketOrderForSubaccount(ctx sdk.Context, marketID, subaccountID common.Hash, isBuy bool) bool {
	return d.base.HasTransientDerivativeMarketOrderForSubaccount(ctx, marketID, subaccountID, isBuy)
}

func (d riskCrossDeps) IterateDerivativeMarketOrdersBySubaccount(
	ctx sdk.Context,
	marketID common.Hash,
	subaccountID common.Hash,
	isBuy bool,
	process func(order *v2.DerivativeMarketOrder) (stop bool),
) {
	d.base.IterateDerivativeMarketOrdersBySubaccount(ctx, marketID, subaccountID, isBuy, process)
}

func (d riskCrossDeps) IterateTransientDerivativeLimitOrdersBySubaccount(
	ctx sdk.Context,
	marketID common.Hash,
	isBuy bool,
	subaccountID common.Hash,
	process func(order *v2.DerivativeLimitOrder) (stop bool),
) {
	d.base.IterateTransientDerivativeLimitOrdersBySubaccount(ctx, marketID, isBuy, subaccountID, process)
}

func (d riskCrossDeps) AtomicMarketOrderFeeMultiplier(ctx sdk.Context, marketID common.Hash, marketType exchangetypes.MarketType) math.LegacyDec {
	return d.base.GetMarketAtomicExecutionFeeMultiplier(ctx, marketID, marketType)
}

func (d riskCrossDeps) ObjectStore(ctx sdk.Context) storetypes.ObjKVStore {
	return ctx.ObjectStore(d.base.GetObjectStoreKey())
}

var _ risk.CrossMarginDeps = riskCrossDeps{}
