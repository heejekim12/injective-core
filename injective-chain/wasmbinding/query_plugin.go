package wasmbinding

import (
	"encoding/json"
	"fmt"

	errorsmod "cosmossdk.io/errors"
	wasmkeeper "github.com/CosmWasm/wasmd/x/wasm/keeper"
	wasmvmtypes "github.com/CosmWasm/wasmvm/v2/types"
	abci "github.com/cometbft/cometbft/abci/types"
	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/cosmos/cosmos-sdk/x/authz"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/golang/protobuf/proto" //nolint:staticcheck // this dependency is still used in cosmos sdk

	auctiontypes "github.com/InjectiveLabs/injective-core/injective-chain/modules/auction/types"
	exchangetypes "github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/types"
	exchangev2 "github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/types/v2"
	oracletypes "github.com/InjectiveLabs/injective-core/injective-chain/modules/oracle/types"
)

const (
	AuthzRoute        = "authz"
	StakingRoute      = "staking"
	AuctionRoute      = "auction"
	OracleRoute       = "oracle"
	ExchangeRoute     = "exchange"
	TokenFactoryRoute = "tokenfactory"
	WasmxRoute        = "wasmx"
	FeeGrant          = "feegrant"
)

type InjectiveQueryWrapper struct {
	// specifies which module handler should handle the query
	Route string `json:"route,omitempty"`
	// The query data that should be parsed into the module query
	QueryData json.RawMessage `json:"query_data,omitempty"`
}

// CustomQuerier dispatches custom CosmWasm bindings queries.
func CustomQuerier(qp *QueryPlugin) wasmkeeper.CustomQuerier {
	// Create a map of route to handler function
	handlers := map[string]func(sdk.Context, json.RawMessage) ([]byte, error){
		AuthzRoute:        qp.HandleAuthzQuery,
		StakingRoute:      qp.HandleStakingQuery,
		AuctionRoute:      qp.HandleAuctionQuery,
		OracleRoute:       qp.HandleOracleQuery,
		ExchangeRoute:     qp.HandleExchangeQuery,
		TokenFactoryRoute: qp.HandleTokenFactoryQuery,
		WasmxRoute:        qp.HandleWasmxQuery,
		FeeGrant:          qp.HandleFeeGrantQuery,
	}

	return func(ctx sdk.Context, request json.RawMessage) ([]byte, error) {
		var contractQuery InjectiveQueryWrapper
		if err := json.Unmarshal(request, &contractQuery); err != nil {
			return nil, errorsmod.Wrap(err, "Error parsing request data")
		}

		handler, exists := handlers[contractQuery.Route]
		if !exists {
			return nil, wasmvmtypes.UnsupportedRequest{Kind: "Unknown Injective Query Route"}
		}

		return handler(ctx, contractQuery.QueryData)
	}
}

type AcceptedStargateQueries map[string]proto.Message

func getWhitelistedQueries() map[string]func() proto.Message {
	return map[string]func() proto.Message{
		// auth
		"/cosmos.auth.v1beta1.Query/Account": func() proto.Message { return &authtypes.QueryAccountResponse{} },
		"/cosmos.auth.v1beta1.Query/Params":  func() proto.Message { return &authtypes.QueryParamsResponse{} },

		// bank
		"/cosmos.bank.v1beta1.Query/Balance":       func() proto.Message { return &banktypes.QueryBalanceResponse{} },
		"/cosmos.bank.v1beta1.Query/DenomMetadata": func() proto.Message { return &banktypes.QueryDenomsMetadataResponse{} },
		"/cosmos.bank.v1beta1.Query/Params":        func() proto.Message { return &banktypes.QueryParamsResponse{} },
		"/cosmos.bank.v1beta1.Query/SupplyOf":      func() proto.Message { return &banktypes.QuerySupplyOfResponse{} },

		// Injective queries
		// Exchange
		"/injective.exchange.v1beta1.Query/QueryExchangeParams":                 func() proto.Message { return &exchangetypes.QueryExchangeParamsResponse{} },
		"/injective.exchange.v1beta1.Query/SubaccountDeposit":                   func() proto.Message { return &exchangetypes.QuerySubaccountDepositResponse{} },
		"/injective.exchange.v1beta1.Query/DerivativeMarket":                    func() proto.Message { return &exchangetypes.QueryDerivativeMarketResponse{} },
		"/injective.exchange.v1beta1.Query/SpotMarket":                          func() proto.Message { return &exchangetypes.QuerySpotMarketResponse{} },
		"/injective.exchange.v1beta1.Query/SubaccountEffectivePositionInMarket": func() proto.Message { return &exchangetypes.QuerySubaccountEffectivePositionInMarketResponse{} },
		"/injective.exchange.v1beta1.Query/SubaccountPositionInMarket":          func() proto.Message { return &exchangetypes.QuerySubaccountPositionInMarketResponse{} },
		"/injective.exchange.v1beta1.Query/TraderDerivativeOrders":              func() proto.Message { return &exchangetypes.QueryTraderDerivativeOrdersResponse{} },
		"/injective.exchange.v1beta1.Query/TraderDerivativeTransientOrders":     func() proto.Message { return &exchangetypes.QueryTraderDerivativeOrdersResponse{} },
		"/injective.exchange.v1beta1.Query/TraderSpotTransientOrders":           func() proto.Message { return &exchangetypes.QueryTraderSpotOrdersResponse{} },
		"/injective.exchange.v1beta1.Query/TraderSpotOrders":                    func() proto.Message { return &exchangetypes.QueryTraderSpotOrdersResponse{} },
		"/injective.exchange.v1beta1.Query/PerpetualMarketInfo":                 func() proto.Message { return &exchangetypes.QueryPerpetualMarketInfoResponse{} },
		"/injective.exchange.v1beta1.Query/PerpetualMarketFunding":              func() proto.Message { return &exchangetypes.QueryPerpetualMarketFundingResponse{} },
		"/injective.exchange.v1beta1.Query/MarketVolatility":                    func() proto.Message { return &exchangetypes.QueryMarketVolatilityResponse{} },
		"/injective.exchange.v1beta1.Query/SpotMidPriceAndTOB":                  func() proto.Message { return &exchangetypes.QuerySpotMidPriceAndTOBResponse{} },
		"/injective.exchange.v1beta1.Query/DerivativeMidPriceAndTOB":            func() proto.Message { return &exchangetypes.QueryDerivativeMidPriceAndTOBResponse{} },
		"/injective.exchange.v1beta1.Query/AggregateMarketVolume":               func() proto.Message { return &exchangetypes.QueryAggregateMarketVolumeResponse{} },
		"/injective.exchange.v1beta1.Query/SpotOrderbook":                       func() proto.Message { return &exchangetypes.QuerySpotOrderbookResponse{} },
		"/injective.exchange.v1beta1.Query/DerivativeOrderbook":                 func() proto.Message { return &exchangetypes.QueryDerivativeOrderbookResponse{} },
		"/injective.exchange.v1beta1.Query/MarketAtomicExecutionFeeMultiplier":  func() proto.Message { return &exchangetypes.QueryMarketAtomicExecutionFeeMultiplierResponse{} },
		// ExchangeV2
		"/injective.exchange.v2.Query/QueryExchangeParams":                 func() proto.Message { return &exchangev2.QueryExchangeParamsResponse{} },
		"/injective.exchange.v2.Query/SubaccountDeposit":                   func() proto.Message { return &exchangev2.QuerySubaccountDepositResponse{} },
		"/injective.exchange.v2.Query/DerivativeMarket":                    func() proto.Message { return &exchangev2.QueryDerivativeMarketResponse{} },
		"/injective.exchange.v2.Query/SpotMarket":                          func() proto.Message { return &exchangev2.QuerySpotMarketResponse{} },
		"/injective.exchange.v2.Query/SubaccountEffectivePositionInMarket": func() proto.Message { return &exchangev2.QuerySubaccountEffectivePositionInMarketResponse{} },
		"/injective.exchange.v2.Query/SubaccountPositionInMarket":          func() proto.Message { return &exchangev2.QuerySubaccountPositionInMarketResponse{} },
		"/injective.exchange.v2.Query/TraderDerivativeOrders":              func() proto.Message { return &exchangev2.QueryTraderDerivativeOrdersResponse{} },
		"/injective.exchange.v2.Query/TraderDerivativeTransientOrders":     func() proto.Message { return &exchangev2.QueryTraderDerivativeOrdersResponse{} },
		"/injective.exchange.v2.Query/TraderSpotTransientOrders":           func() proto.Message { return &exchangev2.QueryTraderSpotOrdersResponse{} },
		"/injective.exchange.v2.Query/TraderSpotOrders":                    func() proto.Message { return &exchangev2.QueryTraderSpotOrdersResponse{} },
		"/injective.exchange.v2.Query/PerpetualMarketInfo":                 func() proto.Message { return &exchangev2.QueryPerpetualMarketInfoResponse{} },
		"/injective.exchange.v2.Query/PerpetualMarketFunding":              func() proto.Message { return &exchangev2.QueryPerpetualMarketFundingResponse{} },
		"/injective.exchange.v2.Query/MarketVolatility":                    func() proto.Message { return &exchangev2.QueryMarketVolatilityResponse{} },
		"/injective.exchange.v2.Query/SpotMidPriceAndTOB":                  func() proto.Message { return &exchangev2.QuerySpotMidPriceAndTOBResponse{} },
		"/injective.exchange.v2.Query/DerivativeMidPriceAndTOB":            func() proto.Message { return &exchangev2.QueryDerivativeMidPriceAndTOBResponse{} },
		"/injective.exchange.v2.Query/AggregateMarketVolume":               func() proto.Message { return &exchangev2.QueryAggregateMarketVolumeResponse{} },
		"/injective.exchange.v2.Query/SpotOrderbook":                       func() proto.Message { return &exchangev2.QuerySpotOrderbookResponse{} },
		"/injective.exchange.v2.Query/DerivativeOrderbook":                 func() proto.Message { return &exchangev2.QueryDerivativeOrderbookResponse{} },
		"/injective.exchange.v2.Query/MarketAtomicExecutionFeeMultiplier":  func() proto.Message { return &exchangev2.QueryMarketAtomicExecutionFeeMultiplierResponse{} },
		// Oracle
		"/injective.oracle.v1beta1.Query/OracleVolatility": func() proto.Message { return &oracletypes.QueryOracleVolatilityResponse{} },
		"/injective.oracle.v1beta1.Query/OraclePrice":      func() proto.Message { return &oracletypes.QueryOraclePriceResponse{} },
		"/injective.oracle.v1beta1.Query/PythPrice":        func() proto.Message { return &oracletypes.QueryPythPriceResponse{} },
		// Auction
		"/injective.auction.v1beta1.Query/LastAuctionResult":    func() proto.Message { return &auctiontypes.QueryLastAuctionResultResponse{} },
		"/injective.auction.v1beta1.Query/AuctionParams":        func() proto.Message { return &auctiontypes.QueryAuctionParamsResponse{} },
		"/injective.auction.v1beta1.Query/CurrentAuctionBasket": func() proto.Message { return &auctiontypes.QueryCurrentAuctionBasketResponse{} },
		// Authz
		"/cosmos.authz.v1beta1.Query/GranteeGrants": func() proto.Message { return &authz.QueryGranteeGrantsResponse{} },
		"/cosmos.authz.v1beta1.Query/GranterGrants": func() proto.Message { return &authz.QueryGranterGrantsResponse{} },
		"/cosmos.authz.v1beta1.Query/Grants":        func() proto.Message { return &authz.QueryGrantsResponse{} },
	}
}

// StargateQuerier dispatches whitelisted stargate queries
func StargateQuerier(
	queryRouter baseapp.GRPCQueryRouter, codecInterface codec.Codec,
) func(ctx sdk.Context, request *wasmvmtypes.StargateQuery) ([]byte, error) {
	acceptList := getWhitelistedQueries()
	return func(ctx sdk.Context, request *wasmvmtypes.StargateQuery) ([]byte, error) {
		newResponse, accepted := acceptList[request.Path]
		if !accepted {
			return nil, wasmvmtypes.UnsupportedRequest{Kind: fmt.Sprintf("'%s' path is not allowed from the contract", request.Path)}
		}

		// Construct a fresh response per call. The accept list holds constructors, not shared
		// instances, so a concurrent off-consensus contract query (gRPC SmartContractState / tx
		// Simulate) on the same path cannot overwrite this query's response object.
		protoResponse := newResponse()

		route := queryRouter.Route(request.Path)
		if route == nil {
			return nil, wasmvmtypes.UnsupportedRequest{Kind: fmt.Sprintf("No route to query '%s'", request.Path)}
		}

		res, err := route(ctx, &abci.QueryRequest{
			Data: request.Data,
			Path: request.Path,
		})
		if err != nil {
			return nil, err
		}

		return ConvertProtoToJSONMarshal(codecInterface, protoResponse, res.Value)
	}
}

// ConvertProtoToJsonMarshal  unmarshals the given bytes into a proto message and then marshals it to json.
// This is done so that clients calling stargate queries do not need to define their own proto unmarshalers,
// being able to use response directly by json marshalling, which is supported in cosmwasm.
func ConvertProtoToJSONMarshal(cdc codec.Codec, protoResponse proto.Message, bz []byte) ([]byte, error) {
	// unmarshal binary into stargate response data structure
	err := cdc.Unmarshal(bz, protoResponse)
	if err != nil {
		return nil, errorsmod.Wrap(err, "to proto")
	}

	bz, err = cdc.MarshalJSON(protoResponse)
	if err != nil {
		return nil, errorsmod.Wrap(err, "to json")
	}

	// gogoproto Unmarshal merges into protoResponse rather than replacing it, so leave the
	// message reset on return: callers that reuse a response object must not see stale fields
	// from a prior call. (StargateQuerier already passes a fresh response per call.)
	protoResponse.Reset()
	return bz, nil
}
