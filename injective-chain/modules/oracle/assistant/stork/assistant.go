package stork

import (
	"context"
	"slices"

	"cosmossdk.io/errors"
	"cosmossdk.io/log"
	"cosmossdk.io/math"
	"github.com/InjectiveLabs/metrics/v2"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"

	"github.com/InjectiveLabs/injective-core/injective-chain/modules/oracle/assistant/shared"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/oracle/types"
)

// Keeper is the subset of keeper methods used by the Stork assistant.
type Keeper interface {
	Logger(ctx sdk.Context) log.Logger
	Meter(ctx context.Context) metrics.Meter
	IsStorkPublisher(ctx sdk.Context, address string) bool
	GetStorkPriceState(ctx sdk.Context, symbol string) *types.StorkPriceState
	SetStorkPriceState(ctx sdk.Context, priceData *types.StorkPriceState)
	EmitStorkPricesUpdated(ctx sdk.Context, states []*types.StorkPriceState)
}

type publisherTimestampKey struct {
	addr common.Address
	ts   uint64
}

// Assistant implements OracleAssistant for Stork.
type Assistant struct {
	keeper Keeper
}

// NewAssistant constructs a Stork assistant backed by the given keeper.
func NewAssistant(k Keeper) *Assistant {
	return &Assistant{keeper: k}
}

func (*Assistant) OracleType() types.OracleType {
	return types.OracleType_Stork
}

func (a *Assistant) ProcessRelay(ctx sdk.Context, msg sdk.Msg) (err error) {
	defer a.keeper.Meter(ctx).FuncTiming(&ctx, "Assistant.ProcessRelay")(&err)
	m, ok := msg.(*types.MsgRelayStorkPrices)
	if !ok {
		return errors.Wrap(types.ErrInvalidOracleRequest, "expected MsgRelayStorkPrices")
	}
	a.ProcessAssetPairsData(ctx, m.AssetPairs)
	return nil
}

// ProcessAssetPairsData applies asset pair updates and emits price-updated events via the keeper.
func (a *Assistant) ProcessAssetPairsData(ctx sdk.Context, assetPairs []*types.AssetPair) {
	defer a.keeper.Meter(ctx).FuncTiming(&ctx, "Assistant.ProcessAssetPairsData")()
	states := a.collectStorkPriceStatesFromAssetPairs(ctx, assetPairs)
	a.keeper.EmitStorkPricesUpdated(ctx, states)
}

func (a *Assistant) collectLegalSignedPrices(ctx sdk.Context, pair *types.AssetPair) ([]*types.SignedPriceOfAssetPair, uint64) {
	defer a.keeper.Meter(ctx).FuncTiming(&ctx, "Assistant.collectLegalSignedPrices")()
	legalSignedPrices := make([]*types.SignedPriceOfAssetPair, 0, len(pair.SignedPrices))
	seen := make(map[publisherTimestampKey]struct{}, len(pair.SignedPrices))
	latestTimestamp := uint64(0)
	for i := range pair.SignedPrices {
		signedPrice := pair.SignedPrices[i]
		timestamp := types.ConvertTimestampToNanoSecond(signedPrice.Timestamp)
		if !a.keeper.IsStorkPublisher(ctx, signedPrice.PublisherKey) {
			continue
		}

		key := publisherTimestampKey{addr: common.HexToAddress(signedPrice.PublisherKey), ts: signedPrice.Timestamp}
		if _, alreadySeen := seen[key]; alreadySeen {
			continue
		}
		seen[key] = struct{}{}

		legalSignedPrices = append(legalSignedPrices, signedPrice)
		if timestamp > latestTimestamp {
			latestTimestamp = timestamp
		}
	}
	return legalSignedPrices, latestTimestamp
}

func (a *Assistant) processOneAssetPair(ctx sdk.Context, pair *types.AssetPair) *types.StorkPriceState {
	defer a.keeper.Meter(ctx).FuncTiming(&ctx, "Assistant.processOneAssetPair")()
	legalSignedPrices, latestTimestamp := a.collectLegalSignedPrices(ctx, pair)
	if len(legalSignedPrices) == 0 {
		a.keeper.Logger(ctx).Error("asset id %s doesn't have at least a valid signed price", pair.AssetId)
		return nil
	}
	storkPriceState := a.keeper.GetStorkPriceState(ctx, pair.AssetId)
	price := getScaledMedianPriceFromValidSignedPrices(legalSignedPrices)

	if storkPriceState != nil && types.ConvertTimestampToNanoSecond(storkPriceState.Timestamp) >= latestTimestamp {
		return nil
	}

	if storkPriceState != nil && types.CheckPriceFeedThreshold(storkPriceState.PriceState.Price, price) {
		return nil
	}

	blockTime := ctx.BlockTime().Unix()

	if storkPriceState == nil {
		storkPriceState = types.NewStorkPriceState(price, latestTimestamp, pair.AssetId, blockTime)
	} else {
		storkPriceState.Update(price, latestTimestamp, blockTime)
	}

	a.keeper.SetStorkPriceState(ctx, storkPriceState)
	return storkPriceState
}

func (a *Assistant) collectStorkPriceStatesFromAssetPairs(ctx sdk.Context, assetPairs []*types.AssetPair) []*types.StorkPriceState {
	defer a.keeper.Meter(ctx).FuncTiming(&ctx, "Assistant.collectStorkPriceStatesFromAssetPairs")()
	storkPriceStates := make([]*types.StorkPriceState, 0, len(assetPairs))
	for idx := range assetPairs {
		if state := a.processOneAssetPair(ctx, assetPairs[idx]); state != nil {
			storkPriceStates = append(storkPriceStates, state)
		}
	}
	return storkPriceStates
}

func getScaledMedianPriceFromValidSignedPrices(legalSigned []*types.SignedPriceOfAssetPair) math.LegacyDec {
	listPrices := make([]math.LegacyDec, 0, len(legalSigned))
	for idx := range legalSigned {
		listPrices = append(listPrices, legalSigned[idx].Price)
	}

	slices.SortStableFunc(listPrices, func(a, b math.LegacyDec) int {
		if a.LT(b) {
			return -1
		}
		if a.GT(b) {
			return 1
		}
		return 0
	})

	unscaledMedianPrice := listPrices[len(listPrices)/2]

	if len(listPrices)%2 == 0 {
		unscaledMedianPrice = unscaledMedianPrice.Add(listPrices[len(listPrices)/2-1]).Quo(math.LegacyNewDec(2))
	}

	scaledPrice := types.ScaleStorkPrice(unscaledMedianPrice)
	return scaledPrice
}

func (a *Assistant) PriceState(ctx sdk.Context, key string) *types.PriceState {
	defer a.keeper.Meter(ctx).FuncTiming(&ctx, "Assistant.PriceState")()
	ps := a.keeper.GetStorkPriceState(ctx, key)
	if ps == nil {
		return nil
	}
	return &ps.PriceState
}

func (a *Assistant) PricePairState(ctx sdk.Context, base, quote string, scaling *types.ScalingOptions) *types.PricePairState {
	defer a.keeper.Meter(ctx).FuncTiming(&ctx, "Assistant.PricePairState")()
	if !shared.PairScalingAllowed(types.OracleType_Stork, scaling, quote) {
		return nil
	}

	basePriceState := a.PriceState(ctx, base)
	if basePriceState == nil {
		return nil
	}

	if quote == types.QuoteUSD {
		baseRate := basePriceState.Price
		if baseRate.IsNil() || !baseRate.IsPositive() {
			return nil
		}
		return shared.PricePairStateForUSD(*basePriceState, baseRate)
	}

	quotePriceState := a.PriceState(ctx, quote)
	return shared.CombinePairPriceState(basePriceState, quotePriceState, quote, scaling)
}

func (a *Assistant) ReferencePrice(ctx sdk.Context, base, quote string) *math.LegacyDec {
	defer a.keeper.Meter(ctx).FuncTiming(&ctx, "Assistant.ReferencePrice")()
	basePriceState := a.keeper.GetStorkPriceState(ctx, base)
	if basePriceState == nil {
		return nil
	}
	if quote == types.QuoteUSD {
		return &basePriceState.PriceState.Price
	}

	quotePriceState := a.keeper.GetStorkPriceState(ctx, quote)
	if quotePriceState == nil {
		return nil
	}

	basePrice := basePriceState.PriceState.Price
	quotePrice := quotePriceState.PriceState.Price

	if basePrice.IsNil() || quotePrice.IsNil() || !basePrice.IsPositive() || !quotePrice.IsPositive() {
		return nil
	}

	price := basePrice.Quo(quotePrice)
	return &price
}
