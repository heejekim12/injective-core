package assistant

import (
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/InjectiveLabs/injective-core/injective-chain/modules/oracle/assistant/chainlink"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/oracle/assistant/coinbase"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/oracle/assistant/pricefeed"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/oracle/assistant/provider"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/oracle/assistant/pyth"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/oracle/assistant/pythpro"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/oracle/assistant/sedafast"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/oracle/assistant/stork"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/oracle/types"
)

// OracleAssistant encapsulates relay handling and price derivation for a single oracle type.
type OracleAssistant interface {
	OracleType() types.OracleType
	ProcessRelay(ctx sdk.Context, msg sdk.Msg) error
	PriceState(ctx sdk.Context, key string) *types.PriceState
	PricePairState(ctx sdk.Context, base, quote string, scaling *types.ScalingOptions) *types.PricePairState
	ReferencePrice(ctx sdk.Context, base, quote string) *math.LegacyDec
}

// OracleKeeper combines the per-oracle interfaces for NewOracleAssistant.
type OracleKeeper interface {
	pyth.Keeper
	stork.Keeper
	chainlink.DataStreamsKeeper
	pricefeed.Keeper
	coinbase.Keeper
	provider.RelayKeeper
	pythpro.Keeper
	sedafast.Keeper
}

// NewOracleAssistant returns the assistant for oracle types that support this refactor phase.
func NewOracleAssistant(k OracleKeeper, oracleType types.OracleType) (OracleAssistant, error) {
	switch oracleType {
	case types.OracleType_Pyth:
		return pyth.NewAssistant(k), nil
	case types.OracleType_Stork:
		return stork.NewAssistant(k), nil
	case types.OracleType_ChainlinkDataStreams:
		return chainlink.NewAssistant(k), nil
	case types.OracleType_PriceFeed:
		return pricefeed.NewAssistant(k), nil
	case types.OracleType_Coinbase:
		return coinbase.NewAssistant(k), nil
	case types.OracleType_Provider:
		return provider.NewAssistant(k), nil
	case types.OracleType_PythPro:
		return pythpro.NewAssistant(k), nil
	case types.OracleType_SedaFast:
		return sedafast.NewAssistant(k), nil
	default:
		return nil, types.ErrUnsupportedOracleType
	}
}
