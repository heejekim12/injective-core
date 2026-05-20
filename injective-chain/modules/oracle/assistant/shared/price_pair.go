package shared

import (
	"cosmossdk.io/math"

	"github.com/InjectiveLabs/injective-core/injective-chain/modules/oracle/types"
)

// PricePairStateForUSD builds a PricePairState when the quote is USD (single-leg oracle).
func PricePairStateForUSD(basePriceState types.PriceState, baseRate math.LegacyDec) *types.PricePairState {
	return &types.PricePairState{
		PairPrice:            baseRate,
		BasePrice:            baseRate,
		QuotePrice:           math.LegacyDec{},
		BaseCumulativePrice:  basePriceState.CumulativePrice,
		QuoteCumulativePrice: math.LegacyDec{},
		BaseTimestamp:        basePriceState.Timestamp,
		QuoteTimestamp:       basePriceState.Timestamp,
	}
}

// CombinePairPriceState derives a cross pair price state from base and quote price states.
func CombinePairPriceState(
	basePriceState *types.PriceState,
	quotePriceState *types.PriceState,
	quote string,
	scalingOptions *types.ScalingOptions,
) *types.PricePairState {
	baseRate := basePriceState.Price
	if baseRate.IsNil() || !baseRate.IsPositive() {
		return nil
	}

	if quote == types.QuoteUSD {
		return PricePairStateForUSD(*basePriceState, baseRate)
	}

	if quotePriceState == nil {
		return nil
	}

	quoteRate := quotePriceState.Price
	if quoteRate.IsNil() || !quoteRate.IsPositive() {
		return nil
	}

	var pairPrice math.LegacyDec
	if scalingOptions != nil {
		pairPrice = baseRate.Mul(math.LegacyNewDec(10).Power(uint64(scalingOptions.QuoteDecimals))).Quo(
			quoteRate.Mul(math.LegacyNewDec(10).Power(uint64(scalingOptions.BaseDecimals))),
		)
	} else {
		pairPrice = baseRate.Quo(quoteRate)
	}

	return &types.PricePairState{
		PairPrice:            pairPrice,
		BasePrice:            baseRate,
		QuotePrice:           quoteRate,
		BaseCumulativePrice:  basePriceState.CumulativePrice,
		QuoteCumulativePrice: quotePriceState.CumulativePrice,
		BaseTimestamp:        basePriceState.Timestamp,
		QuoteTimestamp:       quotePriceState.Timestamp,
	}
}

// PairScalingAllowed returns whether scaling options may be applied for the given oracle type and quote.
func PairScalingAllowed(oracleType types.OracleType, scaling *types.ScalingOptions, quote string) bool {
	if scaling == nil {
		return true
	}
	return oracleType != types.OracleType_PriceFeed && quote != types.QuoteUSD
}
