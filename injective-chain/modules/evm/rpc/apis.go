package rpc

import (
	"fmt"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/server"
	"github.com/ethereum/go-ethereum/rpc"

	appconfig "github.com/InjectiveLabs/injective-core/injective-chain/app/config"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/evm/rpc/backend"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/evm/rpc/namespaces/ethereum/debug"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/evm/rpc/namespaces/ethereum/eth"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/evm/rpc/namespaces/ethereum/eth/filters"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/evm/rpc/namespaces/ethereum/inj"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/evm/rpc/namespaces/ethereum/miner"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/evm/rpc/namespaces/ethereum/net"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/evm/rpc/namespaces/ethereum/personal"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/evm/rpc/namespaces/ethereum/txpool"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/evm/rpc/namespaces/ethereum/web3"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/evm/rpc/stream"
	chaintypes "github.com/InjectiveLabs/injective-core/injective-chain/types"
)

// RPC namespaces and API version
const (
	// Cosmos namespaces

	CosmosNamespace = "cosmos"

	// Ethereum namespaces

	Web3Namespace      = "web3"
	EthNamespace       = "eth"
	PersonalNamespace  = "personal"
	NetNamespace       = "net"
	TxPoolNamespace    = "txpool"
	DebugNamespace     = "debug"
	MinerNamespace     = "miner"
	InjectiveNamespace = "inj"

	apiVersion = "1.0"
)

// APICreator creates the JSON-RPC API implementations.
type APICreator = func(
	ctx *server.Context,
	clientCtx client.Context,
	stream *stream.RPCStream,
	jsonRPCConfig appconfig.JSONRPCConfig,
	indexer chaintypes.EVMTxIndexer,
) []rpc.API

// apiCreators defines the JSON-RPC API namespaces.
var apiCreators map[string]APICreator

func init() {
	apiCreators = map[string]APICreator{
		EthNamespace: func(ctx *server.Context,
			clientCtx client.Context,
			stream *stream.RPCStream,
			jsonRPCConfig appconfig.JSONRPCConfig,
			indexer chaintypes.EVMTxIndexer,
		) []rpc.API {
			evmBackend := backend.NewBackend(ctx, ctx.Logger, clientCtx, jsonRPCConfig, indexer)
			apis := []rpc.API{
				{
					Namespace: EthNamespace,
					Version:   apiVersion,
					Service:   eth.NewPublicAPI(ctx.Logger, evmBackend),
				},
			}

			if stream == nil {
				ctx.Logger.Info("skipping eth filter API registration because RPC stream is unavailable")
				return apis
			}

			apis = append(apis, rpc.API{
				Namespace: EthNamespace,
				Version:   apiVersion,
				Service:   filters.NewPublicAPI(ctx.Logger, clientCtx, stream, evmBackend),
			})

			return apis
		},
		Web3Namespace: func(*server.Context, client.Context, *stream.RPCStream, appconfig.JSONRPCConfig, chaintypes.EVMTxIndexer) []rpc.API {
			return []rpc.API{
				{
					Namespace: Web3Namespace,
					Version:   apiVersion,
					Service:   web3.NewPublicAPI(),
				},
			}
		},
		NetNamespace: func(_ *server.Context, clientCtx client.Context, _ *stream.RPCStream, _ appconfig.JSONRPCConfig, _ chaintypes.EVMTxIndexer) []rpc.API {
			return []rpc.API{
				{
					Namespace: NetNamespace,
					Version:   apiVersion,
					Service:   net.NewPublicAPI(clientCtx),
				},
			}
		},
		PersonalNamespace: func(ctx *server.Context,
			clientCtx client.Context,
			_ *stream.RPCStream,
			jsonRPCConfig appconfig.JSONRPCConfig,
			indexer chaintypes.EVMTxIndexer,
		) []rpc.API {
			evmBackend := backend.NewBackend(ctx, ctx.Logger, clientCtx, jsonRPCConfig, indexer)
			return []rpc.API{
				{
					Namespace: PersonalNamespace,
					Version:   apiVersion,
					Service:   personal.NewAPI(ctx.Logger, evmBackend),
				},
			}
		},
		TxPoolNamespace: func(ctx *server.Context, _ client.Context, _ *stream.RPCStream, _ appconfig.JSONRPCConfig, _ chaintypes.EVMTxIndexer) []rpc.API {
			return []rpc.API{
				{
					Namespace: TxPoolNamespace,
					Version:   apiVersion,
					Service:   txpool.NewPublicAPI(ctx.Logger),
				},
			}
		},
		DebugNamespace: func(ctx *server.Context,
			clientCtx client.Context,
			_ *stream.RPCStream,
			jsonRPCConfig appconfig.JSONRPCConfig,
			indexer chaintypes.EVMTxIndexer,
		) []rpc.API {
			evmBackend := backend.NewBackend(ctx, ctx.Logger, clientCtx, jsonRPCConfig, indexer)
			return []rpc.API{
				{
					Namespace: DebugNamespace,
					Version:   apiVersion,
					Service:   debug.NewAPI(ctx, evmBackend),
				},
			}
		},
		MinerNamespace: func(ctx *server.Context,
			clientCtx client.Context,
			_ *stream.RPCStream,
			jsonRPCConfig appconfig.JSONRPCConfig,
			indexer chaintypes.EVMTxIndexer,
		) []rpc.API {
			evmBackend := backend.NewBackend(ctx, ctx.Logger, clientCtx, jsonRPCConfig, indexer)
			return []rpc.API{
				{
					Namespace: MinerNamespace,
					Version:   apiVersion,
					Service:   miner.NewPrivateAPI(ctx, evmBackend),
				},
			}
		},
		InjectiveNamespace: func(ctx *server.Context,
			clientCtx client.Context,
			_ *stream.RPCStream,
			jsonRPCConfig appconfig.JSONRPCConfig,
			indexer chaintypes.EVMTxIndexer,
		) []rpc.API {
			evmBackend := backend.NewBackend(ctx, ctx.Logger, clientCtx, jsonRPCConfig, indexer)
			return []rpc.API{
				{
					Namespace: InjectiveNamespace,
					Version:   apiVersion,
					Service:   inj.NewInjectiveAPI(ctx, evmBackend),
				},
			}
		},
	}
}

// GetRPCAPIs returns the list of all APIs
func GetRPCAPIs(ctx *server.Context,
	clientCtx client.Context,
	rpcStream *stream.RPCStream,
	jsonRPCConfig appconfig.JSONRPCConfig,
	indexer chaintypes.EVMTxIndexer,
) []rpc.API {
	var apis []rpc.API

	for _, ns := range jsonRPCConfig.API {
		if creator, ok := apiCreators[ns]; ok {
			apis = append(apis, creator(ctx, clientCtx, rpcStream, jsonRPCConfig, indexer)...)
		} else {
			ctx.Logger.Error("invalid namespace value", "namespace", ns)
		}
	}

	return apis
}

// RegisterAPINamespace registers a new API namespace with the API creator.
// This function fails if the namespace is already registered.
func RegisterAPINamespace(ns string, creator APICreator) error {
	if _, ok := apiCreators[ns]; ok {
		return fmt.Errorf("duplicated api namespace %s", ns)
	}
	apiCreators[ns] = creator
	return nil
}
