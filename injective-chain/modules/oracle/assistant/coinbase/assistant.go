package coinbase

import (
	"context"

	"cosmossdk.io/errors"
	"cosmossdk.io/math"
	"github.com/InjectiveLabs/metrics/v2"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/InjectiveLabs/injective-core/injective-chain/modules/oracle/assistant/shared"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/oracle/types"
	chaintypes "github.com/InjectiveLabs/injective-core/injective-chain/types"
)

// Keeper is the subset of keeper methods used by the Coinbase assistant.
type Keeper interface {
	Meter(ctx context.Context) metrics.Meter
	GetCoinbasePriceStates(ctx sdk.Context, key string) []*types.CoinbasePriceState
	GetCoinbasePriceState(ctx sdk.Context, key string) *types.CoinbasePriceState
	SetCoinbasePriceState(ctx sdk.Context, priceData *types.CoinbasePriceState) error
}

// Assistant implements OracleAssistant for Coinbase.
type Assistant struct {
	keeper Keeper
}

// NewAssistant constructs a Coinbase assistant backed by the given keeper.
func NewAssistant(k Keeper) *Assistant {
	return &Assistant{keeper: k}
}

func (*Assistant) OracleType() types.OracleType {
	return types.OracleType_Coinbase
}

func (a *Assistant) ProcessRelay(ctx sdk.Context, msg sdk.Msg) (err error) {
	defer a.keeper.Meter(ctx).FuncTiming(&ctx, "Assistant.ProcessRelay")(&err)
	m, ok := msg.(*types.MsgRelayCoinbaseMessages)
	if !ok {
		return errors.Wrap(types.ErrInvalidOracleRequest, "expected MsgRelayCoinbaseMessages")
	}
	return a.processCoinbaseMessages(ctx, m)
}

func (a *Assistant) processCoinbaseMessages(ctx sdk.Context, msg *types.MsgRelayCoinbaseMessages) (err error) {
	defer a.keeper.Meter(ctx).FuncTiming(&ctx, "Assistant.processCoinbaseMessages")(&err)

	for idx := range msg.Messages {
		if err := types.ValidateCoinbaseSignature(msg.Messages[idx], msg.Signatures[idx]); err != nil {
			return err
		}

		newCoinbasePriceState, err := types.ParseCoinbaseMessage(msg.Messages[idx])
		if err != nil {
			return err
		}

		price := newCoinbasePriceState.GetDecPrice()

		oldCoinbasePriceState := a.keeper.GetCoinbasePriceState(ctx, newCoinbasePriceState.Key)
		blockTime := ctx.BlockTime().Unix()
		if oldCoinbasePriceState == nil {
			newCoinbasePriceState.PriceState = types.PriceState{
				Price:           price,
				CumulativePrice: math.LegacyZeroDec(),
				Timestamp:       blockTime,
			}
		} else {
			oldCoinbasePriceState.PriceState.UpdatePrice(price, blockTime)
			newCoinbasePriceState.PriceState = oldCoinbasePriceState.PriceState
		}

		if err := a.keeper.SetCoinbasePriceState(ctx, newCoinbasePriceState); err != nil {
			return err
		}
	}

	return nil
}

func (a *Assistant) PriceState(ctx sdk.Context, key string) *types.PriceState {
	defer a.keeper.Meter(ctx).FuncTiming(&ctx, "Assistant.PriceState")()

	ps := a.keeper.GetCoinbasePriceState(ctx, key)
	if ps == nil {
		return nil
	}
	return &ps.PriceState
}

func (a *Assistant) ReferencePrice(ctx sdk.Context, base, quote string) *math.LegacyDec {
	defer a.keeper.Meter(ctx).FuncTiming(&ctx, "Assistant.ReferencePrice")()

	basePrice := a.twapForAsset(ctx, base)
	if quote == types.QuoteUSD {
		return basePrice
	}
	quotePrice := a.twapForAsset(ctx, quote)

	if basePrice == nil || basePrice.IsNil() || quotePrice == nil || quotePrice.IsNil() {
		return nil
	}

	if !basePrice.IsPositive() || !quotePrice.IsPositive() {
		return nil
	}

	price := basePrice.Quo(*quotePrice)
	return &price
}

func (a *Assistant) PricePairState(ctx sdk.Context, base, quote string, scaling *types.ScalingOptions) *types.PricePairState {
	defer a.keeper.Meter(ctx).FuncTiming(&ctx, "Assistant.PricePairState")()
	if !shared.PairScalingAllowed(types.OracleType_Coinbase, scaling, quote) {
		return nil
	}

	basePriceState := a.PriceState(ctx, base)
	if basePriceState == nil {
		return nil
	}

	baseRate := basePriceState.Price
	if baseRate.IsNil() || !baseRate.IsPositive() {
		return nil
	}

	if quote == types.QuoteUSD {
		return shared.PricePairStateForUSD(*basePriceState, baseRate)
	}

	quotePriceState := a.PriceState(ctx, quote)
	return shared.CombinePairPriceState(basePriceState, quotePriceState, quote, scaling)
}

func (a *Assistant) twapForAsset(ctx sdk.Context, asset string) *math.LegacyDec {
	defer a.keeper.Meter(ctx).FuncTiming(&ctx, "Assistant.twapForAsset")()

	assetPriceStates := a.keeper.GetCoinbasePriceStates(ctx, asset)
	if len(assetPriceStates) == 0 {
		return nil
	}

	now := ctx.BlockTime().Unix()
	twapWindowEnd := now - types.TwapWindow
	lastSeenTimestamp := now

	priceCumulative := math.LegacyZeroDec()
	validSamples := 0

	for _, priceState := range assetPriceStates {
		priceStateTimestamp, err := chaintypes.SafeInt64(priceState.Timestamp)
		if err != nil {
			continue
		}
		if twapWindowEnd > lastSeenTimestamp {
			break
		}
		var timeDelta int64
		if priceStateTimestamp < twapWindowEnd {
			timeDelta = types.TwapWindow - (now - lastSeenTimestamp)
		} else {
			timeDelta = lastSeenTimestamp - priceStateTimestamp
		}
		priceCumulativeIncrement := math.LegacyNewDec(timeDelta).Mul(priceState.PriceState.Price)
		priceCumulative = priceCumulative.Add(priceCumulativeIncrement)
		lastSeenTimestamp = priceStateTimestamp
		validSamples++
	}

	if validSamples == 0 {
		return nil
	}

	twapPrice := priceCumulative.QuoTruncate(math.LegacyNewDec(types.TwapWindow))
	return &twapPrice
}
