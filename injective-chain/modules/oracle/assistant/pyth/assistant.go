package pyth

import (
	"context"

	"cosmossdk.io/errors"
	"cosmossdk.io/log"
	"cosmossdk.io/math"
	"github.com/InjectiveLabs/metrics/v2"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"

	"github.com/InjectiveLabs/injective-core/injective-chain/modules/oracle/assistant/shared"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/oracle/types"
	chaintypes "github.com/InjectiveLabs/injective-core/injective-chain/types"
)

// Keeper is the subset of keeper methods used by the Pyth assistant.
type Keeper interface {
	Logger(ctx sdk.Context) log.Logger
	Meter(ctx context.Context) metrics.Meter
	GetPythPriceState(ctx sdk.Context, priceID common.Hash) *types.PythPriceState
	SetPythPriceState(ctx sdk.Context, priceState *types.PythPriceState)
	EmitPythPricesUpdated(ctx sdk.Context, states []*types.PythPriceState)
}

// Assistant implements OracleAssistant for Pyth.
type Assistant struct {
	keeper Keeper
}

// NewAssistant constructs a Pyth assistant backed by the given keeper.
func NewAssistant(k Keeper) *Assistant {
	return &Assistant{keeper: k}
}

func (*Assistant) OracleType() types.OracleType {
	return types.OracleType_Pyth
}

func (a *Assistant) ProcessRelay(ctx sdk.Context, msg sdk.Msg) (err error) {
	defer a.keeper.Meter(ctx).FuncTiming(&ctx, "Assistant.ProcessRelay")(&err)
	m, ok := msg.(*types.MsgRelayPythPrices)
	if !ok {
		return errors.Wrap(types.ErrInvalidOracleRequest, "expected MsgRelayPythPrices")
	}
	a.ProcessPriceAttestations(ctx, m.PriceAttestations)
	return nil
}

// ProcessPriceAttestations applies attestations and emits price-updated events via the keeper.
func (a *Assistant) ProcessPriceAttestations(ctx sdk.Context, priceAttestations []*types.PriceAttestation) {
	defer a.keeper.Meter(ctx).FuncTiming(&ctx, "Assistant.ProcessPriceAttestations")()
	states := a.collectPythPriceStatesFromAttestations(ctx, priceAttestations)
	a.keeper.EmitPythPricesUpdated(ctx, states)
}

func (a *Assistant) collectPythPriceStatesFromAttestations(ctx sdk.Context, priceAttestations []*types.PriceAttestation) []*types.PythPriceState {
	defer a.keeper.Meter(ctx).FuncTiming(&ctx, "Assistant.collectPythPriceStatesFromAttestations")()
	pythPriceStates := make([]*types.PythPriceState, 0, len(priceAttestations))

	for idx := range priceAttestations {
		attestation := priceAttestations[idx]

		if err := attestation.Validate(); err != nil {
			a.keeper.Logger(ctx).Error("skipping invalid pyth price attestation", attestation.String())
			continue
		}

		priceID := attestation.GetPriceIDHash()
		publishTime := attestation.PublishTime
		publishTimeUint64 := uint64(publishTime)
		price := types.GetExponentiatedDec(attestation.Price, int64(attestation.Expo))
		emaPrice := types.GetExponentiatedDec(attestation.EmaPrice, int64(attestation.EmaExpo))
		confValue, err := chaintypes.SafeInt64(attestation.Conf)
		if err != nil {
			a.keeper.Logger(ctx).Error("skipping pyth attestation with confidence overflow", "price_id", attestation.PriceId, "conf", attestation.Conf, "error", err)
			continue
		}
		emaConfValue, err := chaintypes.SafeInt64(attestation.EmaConf)
		if err != nil {
			a.keeper.Logger(ctx).Error("skipping pyth attestation with ema confidence overflow", "price_id", attestation.PriceId, "ema_conf", attestation.EmaConf, "error", err)
			continue
		}
		conf := types.GetExponentiatedDec(confValue, int64(attestation.Expo))
		emaConf := types.GetExponentiatedDec(emaConfValue, int64(attestation.EmaExpo))

		pythPriceState := a.keeper.GetPythPriceState(ctx, priceID)

		if pythPriceState != nil && pythPriceState.PublishTime > publishTimeUint64 {
			continue
		}

		if pythPriceState != nil && types.CheckPriceFeedThreshold(pythPriceState.PriceState.Price, price) {
			continue
		}

		blockTime := ctx.BlockTime().Unix()

		if pythPriceState == nil {
			pythPriceState = types.NewPythPriceState(priceID, emaPrice, emaConf, conf, publishTime, price, blockTime)
		} else {
			pythPriceState.Update(emaPrice, emaConf, conf, publishTimeUint64, price, blockTime)
		}

		a.keeper.SetPythPriceState(ctx, pythPriceState)

		pythPriceStates = append(pythPriceStates, pythPriceState)
	}

	return pythPriceStates
}

func (a *Assistant) PriceState(ctx sdk.Context, key string) *types.PriceState {
	defer a.keeper.Meter(ctx).FuncTiming(&ctx, "Assistant.PriceState")()
	ps := a.keeper.GetPythPriceState(ctx, common.HexToHash(key))
	if ps == nil {
		return nil
	}
	return &ps.PriceState
}

func (a *Assistant) PricePairState(ctx sdk.Context, base, quote string, scaling *types.ScalingOptions) *types.PricePairState {
	defer a.keeper.Meter(ctx).FuncTiming(&ctx, "Assistant.PricePairState")()
	if !shared.PairScalingAllowed(types.OracleType_Pyth, scaling, quote) {
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
	basePriceState := a.keeper.GetPythPriceState(ctx, common.HexToHash(base))
	if basePriceState == nil {
		return nil
	}

	if quote == types.QuoteUSD {
		return &basePriceState.PriceState.Price
	}

	quotePriceState := a.keeper.GetPythPriceState(ctx, common.HexToHash(quote))
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
