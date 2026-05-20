package provider

import (
	"context"

	"cosmossdk.io/errors"
	"cosmossdk.io/math"
	"github.com/InjectiveLabs/metrics/v2"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/InjectiveLabs/injective-core/injective-chain/modules/oracle/assistant/shared"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/oracle/types"
)

// RelayKeeper is the subset of keeper methods used by the Provider assistant.
type RelayKeeper interface {
	Meter(ctx context.Context) metrics.Meter
	IsProviderRelayer(ctx sdk.Context, provider string, relayer sdk.AccAddress) bool
	GetProviderPriceState(ctx sdk.Context, provider, symbol string) *types.ProviderPriceState
	SetProviderPriceState(ctx sdk.Context, provider string, priceState *types.ProviderPriceState)
}

// Key returns the compound oracle key for provider prices, matching keeper price-record encoding.
func Key(provider, symbol string) string {
	return types.JoinProviderCompoundKey(provider, symbol)
}

// ParseProviderKey splits a compound key from Key into provider and symbol.
func ParseProviderKey(key string) (provider, symbol string, ok bool) {
	return types.ParseProviderCompoundKey(key)
}

// Assistant implements OracleAssistant for Provider.
type Assistant struct {
	keeper RelayKeeper
}

// NewAssistant constructs a Provider assistant backed by the given keeper.
func NewAssistant(k RelayKeeper) *Assistant {
	return &Assistant{keeper: k}
}

func (*Assistant) OracleType() types.OracleType {
	return types.OracleType_Provider
}

func (a *Assistant) ProcessRelay(ctx sdk.Context, msg sdk.Msg) (err error) {
	defer a.keeper.Meter(ctx).FuncTiming(&ctx, "Assistant.ProcessRelay")(&err)
	m, ok := msg.(*types.MsgRelayProviderPrices)
	if !ok {
		return errors.Wrap(types.ErrInvalidOracleRequest, "expected MsgRelayProviderPrices")
	}
	relayer, _ := sdk.AccAddressFromBech32(m.Sender)
	if !a.keeper.IsProviderRelayer(ctx, m.Provider, relayer) {
		return errors.Wrapf(types.ErrRelayerNotAuthorized, "relayer %s not an authorized provider for %s", relayer.String(), m.Provider)
	}
	a.ProcessProviderPrices(ctx, m)
	return nil
}

// ProcessProviderPrices applies relayed provider prices without relayer authorization (for keeper/tests).
func (a *Assistant) ProcessProviderPrices(ctx sdk.Context, msg *types.MsgRelayProviderPrices) {
	defer a.keeper.Meter(ctx).FuncTiming(&ctx, "Assistant.ProcessProviderPrices")()

	for idx := range msg.Prices {
		price := msg.Prices[idx]
		symbol := msg.Symbols[idx]

		providerPriceState := a.keeper.GetProviderPriceState(ctx, msg.Provider, symbol)

		blockTime := ctx.BlockTime().Unix()
		if providerPriceState == nil || providerPriceState.State == nil {
			providerPriceState = types.NewProviderPriceState(symbol, price, blockTime)
		} else {
			if types.CheckPriceFeedThreshold(providerPriceState.State.Price, price) {
				continue
			}
			providerPriceState.State.UpdatePrice(price, blockTime)
		}

		a.keeper.SetProviderPriceState(ctx, msg.Provider, providerPriceState)

		// nolint:errcheck //ignored on purpose
		ctx.EventManager().EmitTypedEvent(&types.SetProviderPriceEvent{
			Provider: msg.Provider,
			Relayer:  msg.Sender,
			Symbol:   symbol,
			Price:    price,
		})
	}
}

func (a *Assistant) PriceState(ctx sdk.Context, compoundKey string) *types.PriceState {
	defer a.keeper.Meter(ctx).FuncTiming(&ctx, "Assistant.PriceState")()

	prov, symbol, ok := ParseProviderKey(compoundKey)
	if !ok {
		return nil
	}
	pps := a.keeper.GetProviderPriceState(ctx, prov, symbol)
	if pps == nil || pps.State == nil {
		return nil
	}
	return pps.State
}

func (a *Assistant) PricePairState(ctx sdk.Context, symbolOrCompoundKey, providerOrQuote string, scaling *types.ScalingOptions) *types.PricePairState {
	defer a.keeper.Meter(ctx).FuncTiming(&ctx, "Assistant.PricePairState")()

	if !shared.PairScalingAllowed(types.OracleType_Provider, scaling, providerOrQuote) {
		return nil
	}
	compoundKey, ok := resolveProviderPriceKey(symbolOrCompoundKey, providerOrQuote)
	if !ok {
		return nil
	}
	ps := a.PriceState(ctx, compoundKey)
	if ps == nil {
		return nil
	}
	symbolPrice := ps.Price
	if symbolPrice.IsNil() || !symbolPrice.IsPositive() {
		return nil
	}
	if providerOrQuote == types.QuoteUSD {
		return shared.PricePairStateForUSD(*ps, symbolPrice)
	}
	return nil
}

func (a *Assistant) ReferencePrice(ctx sdk.Context, symbolOrCompoundKey, providerOrQuote string) *math.LegacyDec {
	defer a.keeper.Meter(ctx).FuncTiming(&ctx, "Assistant.ReferencePrice")()

	compoundKey, ok := resolveProviderPriceKey(symbolOrCompoundKey, providerOrQuote)
	if !ok {
		return nil
	}
	ps := a.PriceState(ctx, compoundKey)
	if ps == nil {
		return nil
	}
	price := ps.Price
	if price.IsNil() || !price.IsPositive() {
		return nil
	}
	return &price
}

// resolveProviderPriceKey maps OracleAssistant-style arguments to a compound storage key:
//   - symbolOrCompoundKey may be a full compound key (from Key / JoinProviderCompoundKey); or
//   - when providerOrQuote is not USD, symbolOrCompoundKey is the asset symbol and providerOrQuote is the
//     provider ID (derivative-market layout; see exchange GetDerivativeMarketPrice for OracleType_Provider).
//
// When symbolOrCompoundKey is compound, providerOrQuote must be QuoteUSD (explicit query path) or match the
// embedded provider id; otherwise resolution fails so oracleQuote cannot be silently ignored.
func resolveProviderPriceKey(symbolOrCompoundKey, providerOrQuote string) (compoundKey string, ok bool) {
	if p, _, isCompound := ParseProviderKey(symbolOrCompoundKey); isCompound {
		if providerOrQuote != types.QuoteUSD && providerOrQuote != p {
			return "", false
		}
		return symbolOrCompoundKey, true
	}
	if providerOrQuote == types.QuoteUSD {
		return "", false
	}
	return Key(providerOrQuote, symbolOrCompoundKey), true
}
