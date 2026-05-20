package isolated

import (
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"

	exchangetypes "github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/types"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/types/v2"

	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/risk"
)

// Model implements the existing isolated-margin behaviour.
//
// Today, isolated margin always uses "full hold" (per-order reservation) semantics.
type Model struct{}

// NewModel returns a new isolated-margin model.
func NewModel() *Model {
	return &Model{}
}

func (Model) ReserveSpotLimitOrder(
	ctx sdk.Context,
	funds risk.Funds,
	subaccountID common.Hash,
	order *v2.SpotOrder,
	market *v2.SpotMarket,
) error {
	balanceHoldIncrement, marginDenom := order.GetBalanceHoldAndMarginDenom(market)

	var chainFormattedBalanceHoldIncrement math.LegacyDec
	if order.IsBuy() {
		chainFormattedBalanceHoldIncrement = market.NotionalToChainFormat(balanceHoldIncrement)
	} else {
		chainFormattedBalanceHoldIncrement = market.QuantityToChainFormat(balanceHoldIncrement)
	}

	return funds.ChargeAccount(ctx, subaccountID, marginDenom, chainFormattedBalanceHoldIncrement)
}

func (Model) ReserveSpotMarketOrder(
	ctx sdk.Context,
	funds risk.Funds,
	subaccountID common.Hash,
	order *v2.SpotOrder,
	market *v2.SpotMarket,
	feeRate, bestPrice math.LegacyDec,
) (balanceHold math.LegacyDec, err error) {
	balanceHold = order.GetMarketOrderBalanceHold(feeRate, bestPrice)

	var chainFormattedBalanceHold math.LegacyDec
	marginDenom := order.GetMarginDenom(market)
	if order.IsBuy() {
		chainFormattedBalanceHold = market.NotionalToChainFormat(balanceHold)
	} else {
		chainFormattedBalanceHold = market.QuantityToChainFormat(balanceHold)
	}

	if err := funds.ChargeAccount(ctx, subaccountID, marginDenom, chainFormattedBalanceHold); err != nil {
		return math.LegacyZeroDec(), err
	}

	return balanceHold, nil
}

func (Model) RefundSpotLimitOrderCancel(
	ctx sdk.Context,
	funds risk.Funds,
	subaccountID common.Hash,
	order *v2.SpotLimitOrder,
	market *v2.SpotMarket,
	isTransient bool,
) error {
	marginHold, marginDenom := order.GetUnfilledMarginHoldAndMarginDenom(market, isTransient)

	var chainFormattedMarginHold math.LegacyDec
	if order.IsBuy() {
		chainFormattedMarginHold = market.NotionalToChainFormat(marginHold)
	} else {
		chainFormattedMarginHold = market.QuantityToChainFormat(marginHold)
	}

	funds.IncrementAvailableBalanceOrBank(ctx, subaccountID, marginDenom, chainFormattedMarginHold)
	return nil
}

func (Model) ReserveDerivativeOrderMargin(
	ctx sdk.Context,
	funds risk.Funds,
	subaccountID common.Hash,
	order *v2.DerivativeOrder,
	market v2.DerivativeMarketI,
	markPriceToCheck, tradeFeeRate math.LegacyDec,
) (math.LegacyDec, error) {
	if !order.IsVanilla() {
		return math.LegacyZeroDec(), nil
	}

	marginHold, err := order.CheckMarginAndGetMarginHold(
		market.GetInitialMarginRatio(),
		markPriceToCheck,
		tradeFeeRate,
		market.GetMarketType(),
		market.GetOracleScaleFactor(),
	)
	if err != nil {
		return math.LegacyZeroDec(), err
	}

	chainFormattedMarginHold := market.NotionalToChainFormat(marginHold)
	if err := funds.ChargeAccount(ctx, subaccountID, market.GetQuoteDenom(), chainFormattedMarginHold); err != nil {
		return math.LegacyZeroDec(), err
	}

	return marginHold, nil
}

//nolint:revive // flag-parameter: signature matches Model interface
func (Model) RefundDerivativeLimitOrderCancel(
	ctx sdk.Context,
	funds risk.Funds,
	order *v2.DerivativeLimitOrder,
	market v2.MarketI,
	isTransient bool,
) error {
	if !order.IsVanilla() {
		return nil
	}

	feeRate := market.GetMakerFeeRate()
	if isTransient {
		feeRate = market.GetTakerFeeRate()
	}

	refundAmount := order.GetCancelRefundAmount(feeRate)
	chainFormatRefund := market.NotionalToChainFormat(refundAmount)
	funds.IncrementAvailableBalanceOrBank(ctx, order.SubaccountID(), market.GetQuoteDenom(), chainFormatRefund)
	return nil
}

func (Model) RefundDerivativeMarketOrderCancel(
	ctx sdk.Context,
	funds risk.Funds,
	subaccountID common.Hash,
	market v2.MarketI,
	refundAmount math.LegacyDec,
) {
	chainFormatRefund := market.NotionalToChainFormat(refundAmount)
	funds.IncrementAvailableBalanceOrBank(ctx, subaccountID, market.GetQuoteDenom(), chainFormatRefund)
}

func (Model) ShouldSkipDerivativeOrderForMarginRequirement(
	_ sdk.Context,
	_ common.Hash,
	order risk.DerivativeInitialMarginChecker,
	market v2.DerivativeMarketI,
	markPrice math.LegacyDec,
	_ math.LegacyDec, // remainingQty — unused for isolated margin
) (bool, error) {
	if !order.IsVanilla() || market.GetMarketType() == exchangetypes.MarketType_BinaryOption {
		return false, nil
	}
	return order.CheckInitialMarginRequirementMarkPriceThreshold(market.GetInitialMarginRatio(), markPrice) != nil, nil
}

//nolint:revive // argument-limit: signature matches Model interface
func (Model) CheckValidPositionToReduce(
	_ sdk.Context,
	_ common.Hash,
	position *v2.Position,
	marketType exchangetypes.MarketType,
	orderPrice math.LegacyDec,
	isBuy bool,
	tradeFeeRate math.LegacyDec,
	funding *v2.PerpetualMarketFunding,
	closeExecutionMargin math.LegacyDec,
) error {
	return position.CheckValidPositionToReduce(
		marketType,
		orderPrice,
		isBuy,
		tradeFeeRate,
		funding,
		closeExecutionMargin,
	)
}

func (Model) ValidateDerivativePositionMarginDecrease(
	_ sdk.Context,
	_ common.Hash,
	position *v2.Position,
	market *v2.DerivativeMarket,
	markPrice math.LegacyDec,
) error {
	notional := position.EntryPrice.Mul(position.Quantity)

	if position.Margin.LT(market.ReduceMarginRatio.Mul(notional)) {
		return exchangetypes.ErrInsufficientMargin
	}

	markPriceThreshold := exchangetypes.ComputeMarkPriceThreshold(
		position.IsLong,
		position.EntryPrice,
		position.Quantity,
		position.Margin,
		market.ReduceMarginRatio,
	)
	return exchangetypes.CheckInitialMarginMarkPriceRequirement(position.IsLong, markPriceThreshold, markPrice)
}

func (Model) DerivativePositionLiquidationCheck(
	_ sdk.Context,
	_ common.Hash,
	position *v2.Position,
	market *v2.DerivativeMarket,
	markPrice math.LegacyDec,
	funding *v2.PerpetualMarketFunding,
) (liquidationPrice math.LegacyDec, shouldLiquidate bool, err error) {
	liquidationPrice = position.GetLiquidationPrice(market.MaintenanceMarginRatio, funding)
	shouldLiquidate = (position.IsLong && markPrice.LTE(liquidationPrice)) || (position.IsShort() && markPrice.GTE(liquidationPrice))
	return liquidationPrice, shouldLiquidate, nil
}
