package keeper

import (
	"bytes"
	"sort"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"

	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/types/v2"
)

// IncrementSequenceAndEmitAllTransientOrderbookUpdates increments each orderbook sequence and emits an
// EventOrderbookUpdate event for all the modified orderbooks in all markets.
func (k *Keeper) IncrementSequenceAndEmitAllTransientOrderbookUpdates(ctx sdk.Context) {
	defer k.Meter(ctx).FuncTiming(&ctx, "IncrementSequenceAndEmitAllTransientOrderbookUpdates")()

	spotOrderbooks := k.GetAllTransientOrderbookUpdates(ctx, true)
	derivativeOrderbooks := k.GetAllTransientOrderbookUpdates(ctx, false)

	if len(spotOrderbooks) == 0 && len(derivativeOrderbooks) == 0 {
		return
	}

	spotUpdates := make([]*v2.OrderbookUpdate, 0, len(spotOrderbooks))
	derivativeUpdates := make([]*v2.OrderbookUpdate, 0, len(derivativeOrderbooks))

	for _, orderbook := range spotOrderbooks {
		sequence := k.IncrementOrderbookSequence(ctx, common.BytesToHash(orderbook.MarketId))
		spotUpdates = append(spotUpdates, &v2.OrderbookUpdate{
			Seq:       sequence,
			Orderbook: orderbook,
		})
	}

	for _, orderbook := range derivativeOrderbooks {
		sequence := k.IncrementOrderbookSequence(ctx, common.BytesToHash(orderbook.MarketId))
		derivativeUpdates = append(derivativeUpdates, &v2.OrderbookUpdate{
			Seq:       sequence,
			Orderbook: orderbook,
		})
	}

	k.EmitEvent(ctx, &v2.EventOrderbookUpdate{
		SpotUpdates:       spotUpdates,
		DerivativeUpdates: derivativeUpdates,
	})
}

// GetAllTransientOrderbookUpdates gets all the transient orderbook updates
func (k *Keeper) GetAllTransientOrderbookUpdates(ctx sdk.Context, isSpot bool) []*v2.Orderbook {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetAllTransientOrderbookUpdates")()

	orderbookMap := make(map[common.Hash]*v2.Orderbook)

	appendPriceLevel := func(marketID common.Hash, isBuy bool, priceLevel *v2.Level) (stop bool) {
		if _, ok := orderbookMap[marketID]; !ok {
			orderbookMap[marketID] = v2.NewOrderbook(marketID)
		}

		orderbookMap[marketID].AppendLevel(isBuy, priceLevel)
		return false
	}

	k.IterateTransientOrderbookPriceLevels(ctx, isSpot, appendPriceLevel)

	orderbooks := make([]*v2.Orderbook, 0, len(orderbookMap))
	for _, orderbook := range orderbookMap {
		orderbooks = append(orderbooks, orderbook)
	}

	sort.SliceStable(orderbooks, func(i, j int) bool {
		return bytes.Compare(orderbooks[i].MarketId, orderbooks[j].MarketId) < 1
	})

	return orderbooks
}

func (k *Keeper) GetAllBalancesWithBalanceHolds(ctx sdk.Context) []*v2.BalanceWithMarginHold {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetAllBalancesWithBalanceHolds")()

	var (
		balanceHolds            = make(map[string]map[string]math.LegacyDec)
		balances                = k.GetAllExchangeBalances(ctx)
		restingSpotOrders       = k.GetAllSpotLimitOrderbook(ctx)
		restingDerivativeOrders = k.GetAllDerivativeAndBinaryOptionsLimitOrderbook(ctx)
	)

	var safeUpdateBalanceHolds = func(subaccountId, denom string, amount math.LegacyDec) {
		if _, ok := balanceHolds[subaccountId]; !ok {
			balanceHolds[subaccountId] = make(map[string]math.LegacyDec)
		}

		if balanceHolds[subaccountId][denom].IsNil() {
			balanceHolds[subaccountId][denom] = math.LegacyZeroDec()
		}

		balanceHolds[subaccountId][denom] = balanceHolds[subaccountId][denom].Add(amount)
	}

	processSpotOrders(ctx, k, restingSpotOrders, safeUpdateBalanceHolds)
	processDerivativeOrders(ctx, k, restingDerivativeOrders, safeUpdateBalanceHolds)

	return createBalanceWithMarginHolds(balances, balanceHolds)
}

func processSpotOrders(
	ctx sdk.Context,
	k *Keeper,
	restingSpotOrders []v2.SpotOrderBook,
	safeUpdateBalanceHolds func(subaccountId, denom string, amount math.LegacyDec),
) {
	for _, orderbook := range restingSpotOrders {
		market := k.GetSpotMarketByID(ctx, common.HexToHash(orderbook.MarketId))

		for _, order := range orderbook.Orders {
			var chainFormatBalanceHold math.LegacyDec
			balanceHold, denom := order.GetUnfilledMarginHoldAndMarginDenom(market, false)
			if denom == market.BaseDenom {
				chainFormatBalanceHold = market.QuantityToChainFormat(balanceHold)
			} else {
				chainFormatBalanceHold = market.NotionalToChainFormat(balanceHold)
			}
			safeUpdateBalanceHolds(order.SubaccountID().Hex(), denom, chainFormatBalanceHold)
		}
	}
}

func processDerivativeOrders(
	ctx sdk.Context,
	k *Keeper,
	restingDerivativeOrders []v2.DerivativeOrderBook,
	safeUpdateBalanceHolds func(subaccountId, denom string, amount math.LegacyDec),
) {
	// Cross-margin derivatives use pool-level order locking (OLR) and do not reserve
	// per-order deposit holds, so BalanceWithBalanceHolds should only include
	// derivative order holds for isolated-mode subaccounts.
	isCrossBySubaccount := make(map[string]bool)

	for _, orderbook := range restingDerivativeOrders {
		market := k.GetDerivativeOrBinaryOptionsMarket(ctx, common.HexToHash(orderbook.MarketId), nil)

		for _, order := range orderbook.Orders {
			subaccountIDHex := order.SubaccountID().Hex()
			isCross, ok := isCrossBySubaccount[subaccountIDHex]
			if !ok {
				profile, _ := k.GetEffectiveSubaccountRiskProfile(ctx, order.SubaccountID())
				isCross = profile != nil && profile.Mode == v2.RiskMode_RISK_MODE_CROSS
				isCrossBySubaccount[subaccountIDHex] = isCross
			}
			if isCross {
				continue
			}

			balanceHold := order.GetCancelDepositDelta(market.GetMakerFeeRate()).AvailableBalanceDelta
			chainFormatBalanceHold := market.NotionalToChainFormat(balanceHold)
			safeUpdateBalanceHolds(subaccountIDHex, market.GetQuoteDenom(), chainFormatBalanceHold)
		}
	}
}

func createBalanceWithMarginHolds(
	balances []v2.Balance,
	balanceHolds map[string]map[string]math.LegacyDec,
) []*v2.BalanceWithMarginHold {
	balanceWithBalanceHolds := make([]*v2.BalanceWithMarginHold, 0, len(balances))

	for _, balance := range balances {
		balanceHold := balanceHolds[balance.SubaccountId][balance.Denom]

		if balanceHold.IsNil() {
			balanceHold = math.LegacyZeroDec()
		}

		balanceWithBalanceHolds = append(balanceWithBalanceHolds, &v2.BalanceWithMarginHold{
			SubaccountId: balance.SubaccountId,
			Denom:        balance.Denom,
			Available:    balance.Deposits.AvailableBalance,
			Total:        balance.Deposits.TotalBalance,
			BalanceHold:  balanceHold,
		})
	}

	return balanceWithBalanceHolds
}
