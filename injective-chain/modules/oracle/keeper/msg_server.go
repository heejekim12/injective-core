package keeper

import (
	"context"

	"cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"

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

var _ types.MsgServer = MsgServer{}

type MsgServer struct {
	*Keeper
}

// NewMsgServerImpl returns an implementation of the oracle MsgServer interface
// for the provided Keeper.
func NewMsgServerImpl(keeper Keeper) types.MsgServer {
	return &MsgServer{Keeper: &keeper}
}

func (m MsgServer) RelayPriceFeedPrice(c context.Context, msg *types.MsgRelayPriceFeedPrice) (*types.MsgRelayPriceFeedPriceResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer m.Meter(ctx).FuncTiming(&ctx, "RelayPriceFeedPrice")()
	if err := pricefeed.NewAssistant(m.Keeper).ProcessRelay(ctx, msg); err != nil {
		return nil, err
	}
	return &types.MsgRelayPriceFeedPriceResponse{}, nil
}

func (m MsgServer) RelayCoinbaseMessages(c context.Context, msg *types.MsgRelayCoinbaseMessages) (*types.MsgRelayCoinbaseMessagesResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer m.Meter(ctx).FuncTiming(&ctx, "RelayCoinbaseMessages")()

	if err := coinbase.NewAssistant(m.Keeper).ProcessRelay(ctx, msg); err != nil {
		return nil, err
	}
	return &types.MsgRelayCoinbaseMessagesResponse{}, nil
}

func (m MsgServer) RelayProviderPrices(c context.Context, msg *types.MsgRelayProviderPrices) (*types.MsgRelayProviderPricesResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer m.Meter(ctx).FuncTiming(&ctx, "RelayProviderPrices")()

	if err := provider.NewAssistant(m.Keeper).ProcessRelay(ctx, msg); err != nil {
		return nil, err
	}
	return &types.MsgRelayProviderPricesResponse{}, nil
}

func (m MsgServer) RelayPythPrices(c context.Context, msg *types.MsgRelayPythPrices) (*types.MsgRelayPythPricesResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer m.Meter(ctx).FuncTiming(&ctx, "RelayPythPrices")()

	if len(msg.PriceAttestations) == 0 {
		return &types.MsgRelayPythPricesResponse{}, nil
	}

	pythContract, err := sdk.AccAddressFromBech32(m.GetParams(ctx).PythContract)
	if err != nil {
		return nil, types.ErrPythContractNotFound
	}

	if !pythContract.Equals(sdk.MustAccAddressFromBech32(msg.Sender)) {
		return nil, types.ErrUnauthorizedPythPriceRelay
	}

	if err := pyth.NewAssistant(m.Keeper).ProcessRelay(ctx, msg); err != nil {
		return nil, err
	}
	return &types.MsgRelayPythPricesResponse{}, nil
}

func (m MsgServer) RelayStorkMessage(c context.Context, msg *types.MsgRelayStorkPrices) (*types.MsgRelayStorkPricesResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer m.Meter(ctx).FuncTiming(&ctx, "RelayStorkMessage")()

	if err := stork.NewAssistant(m.Keeper).ProcessRelay(ctx, msg); err != nil {
		return nil, err
	}

	return &types.MsgRelayStorkPricesResponse{}, nil
}

func (m MsgServer) RelayChainlinkPrices(c context.Context, msg *types.MsgRelayChainlinkPrices) (*types.MsgRelayChainlinkPricesResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer m.Meter(ctx).FuncTiming(&ctx, "RelayChainlinkPrices")()

	if len(msg.Reports) == 0 {
		return &types.MsgRelayChainlinkPricesResponse{}, nil
	}

	if err := chainlink.NewAssistant(m.Keeper).ProcessRelay(ctx, msg); err != nil {
		return nil, err
	}

	return &types.MsgRelayChainlinkPricesResponse{}, nil
}

func (m MsgServer) RelayPythProPrices(c context.Context, msg *types.MsgRelayPythProPrices) (*types.MsgRelayPythProPricesResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer m.Meter(ctx).FuncTiming(&ctx, "RelayPythProPrices")()

	if err := pythpro.NewAssistant(m.Keeper).ProcessRelay(ctx, msg); err != nil {
		return nil, err
	}
	return &types.MsgRelayPythProPricesResponse{}, nil
}

func (m MsgServer) RelaySedaFastPrices(c context.Context, msg *types.MsgRelaySedaFastPrices) (*types.MsgRelaySedaFastPricesResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer m.Meter(ctx).FuncTiming(&ctx, "RelaySedaFastPrices")()

	if err := sedafast.NewAssistant(m.Keeper).ProcessRelay(ctx, msg); err != nil {
		return nil, err
	}
	return &types.MsgRelaySedaFastPricesResponse{}, nil
}

func (m MsgServer) UpdateParams(c context.Context, msg *types.MsgUpdateParams) (*types.MsgUpdateParamsResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer m.Meter(ctx).FuncTiming(&ctx, "UpdateParams")()

	if msg.Authority != m.authority {
		return nil, errors.Wrapf(govtypes.ErrInvalidSigner, "invalid authority: expected %s, got %s", m.authority, msg.Authority)
	}

	if err := msg.Params.Validate(); err != nil {
		return nil, err
	}

	m.SetParams(ctx, msg.Params)

	return &types.MsgUpdateParamsResponse{}, nil
}
