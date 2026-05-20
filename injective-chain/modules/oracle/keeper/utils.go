package keeper

import (
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/InjectiveLabs/injective-core/injective-chain/modules/oracle/assistant"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/oracle/types"
)

func (k *Keeper) GetPricePairStateForUSD(ctx sdk.Context, basePriceState types.PriceState, baseRate math.LegacyDec) *types.PricePairState {
	pricePairState := types.PricePairState{
		PairPrice:            baseRate,
		BasePrice:            baseRate,
		QuotePrice:           math.LegacyDec{},
		BaseCumulativePrice:  basePriceState.CumulativePrice,
		QuoteCumulativePrice: math.LegacyDec{},
		BaseTimestamp:        basePriceState.Timestamp,
		QuoteTimestamp:       basePriceState.Timestamp,
	}
	return &pricePairState
}

func (k *Keeper) GetPriceState(ctx sdk.Context, key string, oracletype types.OracleType) *types.PriceState {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetPriceState")()

	// price feed has no single denom price points
	if oracletype == types.OracleType_PriceFeed {
		return nil
	}

	switch oracletype {
	case types.OracleType_Band:
		priceState := k.GetBandPriceState(ctx, key)
		if priceState == nil {
			return nil
		}
		return &priceState.PriceState
	case types.OracleType_BandIBC:
		priceState := k.GetBandIBCPriceState(ctx, key)
		if priceState == nil {
			return nil
		}
		return &priceState.PriceState
	case types.OracleType_Razor, types.OracleType_Dia, types.OracleType_API3, types.OracleType_Uma:
		return nil
	default:
		a, err := assistant.NewOracleAssistant(k, oracletype)
		if err != nil {
			return nil
		}
		return a.PriceState(ctx, key)
	}
}
