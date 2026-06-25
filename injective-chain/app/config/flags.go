package config

import (
	"os"
	"path/filepath"
	"time"

	"github.com/CosmWasm/wasmd/x/wasm"
	cmtcmd "github.com/cometbft/cometbft/cmd/cometbft/commands"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/cosmos/cosmos-sdk/server"
	"github.com/cosmos/cosmos-sdk/x/crisis"
	"github.com/spf13/cobra"

	chainstreamtypes "github.com/InjectiveLabs/injective-core/injective-chain/stream/types"
)

const (
	FlagTraceStore                 = "trace-store"
	FlagCPUProfile                 = "cpu-profile"
	FlagOptimisticExecutionEnabled = "optimistic-execution-enabled"
	FlagAppDBBackend               = "app-db-backend"
	FlagSkipAnteHandlers           = "SkipAnteHandlers" // to be used in tests

	FlagMempoolRecheckEnabled = "mempool.recheck" // controls Comet mempool

	FlagUnsafeConsensusTimeoutPropose        = "unsafe-consensus-timeout-propose"
	FlagUnsafeConsensusTimeoutProposeDelta   = "unsafe-consensus-timeout-propose-delta"
	FlagUnsafeConsensusTimeoutPrevote        = "unsafe-consensus-timeout-prevote"
	FlagUnsafeConsensusTimeoutPrevoteDelta   = "unsafe-consensus-timeout-prevote-delta"
	FlagUnsafeConsensusTimeoutPrecommit      = "unsafe-consensus-timeout-precommit"
	FlagUnsafeConsensusTimeoutPrecommitDelta = "unsafe-consensus-timeout-precommit-delta"
	FlagUnsafeConsensusTimeoutCommit         = "unsafe-consensus-timeout-commit"

	FlagGRPCOnly      = "grpc-only"
	FlagGRPCEnable    = "grpc.enable"
	FlagGRPCAddress   = "grpc.address"
	FlagGRPCWebEnable = "grpc-web.enable"

	FlagLogLevel   = "log-level"
	FlagLogFormat  = "log-format"
	FlagLogNoColor = "log-no-color"

	FlagMetricsEnableMetrics           = "metrics.metrics-enabled"
	FlagMetricsEnableTracing           = "metrics.tracing-enabled"
	FlagMetricsEndpoint                = "metrics.endpoint"
	FlagMetricsInsecure                = "metrics.insecure-endpoint"
	FlagMetricsExportInterval          = "metrics.export-interval"
	FlagMetricsStuckFuncTimeout        = "metrics.stuck-func-timeout"
	FlagMetricsFlightRecorderThreshold = "metrics.flight-recorder-threshold"

	FlagJSONRPCEnable              = "json-rpc.enable"
	FlagJSONRPCAPI                 = "json-rpc.api"
	FlagJSONRPCAddress             = "json-rpc.address"
	FlagJSONWsAddress              = "json-rpc.ws-address"
	FlagJSONRPCGasCap              = "json-rpc.gas-cap"
	FlagJSONRPCEVMTimeout          = "json-rpc.evm-timeout"
	FlagJSONRPCTxFeeCap            = "json-rpc.txfee-cap"
	FlagJSONRPCFilterCap           = "json-rpc.filter-cap"
	FlagJSONRPCFeeHistoryCap       = "json-rpc.feehistory-cap"
	FlagJSONRPCLogsCap             = "json-rpc.logs-cap"
	FlagJSONRPCBlockRangeCap       = "json-rpc.block-range-cap"
	FlagJSONRPCHTTPTimeout         = "json-rpc.http-timeout"
	FlagJSONRPCHTTPIdleTimeout     = "json-rpc.http-idle-timeout"
	FlagJSONRPCAllowUnprotectedTxs = "json-rpc.allow-unprotected-txs"
	FlagJSONRPCMaxOpenConnections  = "json-rpc.max-open-connections"
	FlagJSONRPCEnableIndexer       = "json-rpc.enable-indexer"
	FlagJSONRPCAllowIndexerGap     = "json-rpc.allow-indexer-gap"
	FlagJSONRPCEnableMetrics       = "json-rpc.metrics"
	FlagJSONRPCMetricsAddress      = "json-rpc.metrics-address"
	FlagJSONRPCReturnDataLimit     = "json-rpc.return-data-limit"

	FlagJSONRPCDebugEnable             = "json-rpc-debug.enable"
	FlagJSONRPCDebugAPI                = "json-rpc-debug.api"
	FlagJSONRPCDebugAddress            = "json-rpc-debug.address"
	FlagJSONRPCDebugGasCap             = "json-rpc-debug.gas-cap"
	FlagJSONRPCDebugEVMTimeout         = "json-rpc-debug.evm-timeout"
	FlagJSONRPCDebugTxFeeCap           = "json-rpc-debug.txfee-cap"
	FlagJSONRPCDebugFilterCap          = "json-rpc-debug.filter-cap"
	FlagJSONRPCDebugFeeHistoryCap      = "json-rpc-debug.feehistory-cap"
	FlagJSONRPCDebugLogsCap            = "json-rpc-debug.logs-cap"
	FlagJSONRPCDebugBlockRangeCap      = "json-rpc-debug.block-range-cap"
	FlagJSONRPCDebugHTTPTimeout        = "json-rpc-debug.http-timeout"
	FlagJSONRPCDebugHTTPIdleTimeout    = "json-rpc-debug.http-idle-timeout"
	FlagJSONRPCDebugMaxOpenConnections = "json-rpc-debug.max-open-connections"
	FlagJSONRPCDebugReturnDataLimit    = "json-rpc-debug.return-data-limit"

	FlagEVMTracer            = "evm.tracer"
	FlagEVMMaxTxGasWanted    = "evm.max-tx-gas-wanted"
	FlagEVMEnableGRPCTracing = "evm.enable-grpc-tracing"

	FlagInjWebsocketAddress             = "injective-websocket.address"
	FlagInjWebsocketMaxOpenConnections  = "injective-websocket.max-open-connections"
	FlagInjWebsocketReadTimeout         = "injective-websocket.read-timeout"
	FlagInjWebsocketWriteTimeout        = "injective-websocket.write-timeout"
	FlagInjWebsocketMaxBodyBytes        = "injective-websocket.max-body-bytes"
	FlagInjWebsocketMaxHeaderBytes      = "injective-websocket.max-header-bytes"
	FlagInjWebsocketMaxRequestBatchSize = "injective-websocket.max-request-batch-size"
)

var (
	defaultCfg      = DefaultConfig()
	DefaultNodeHome = "" // set in init()
	DefaultChainID  = "injective-1"
)

func init() {
	userHomeDir, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}

	DefaultNodeHome = filepath.Join(userHomeDir, ".injectived")
}

// addStartNodeFlags should be added to any CLI commands that start the network.
func AddStartNodeFlags(cmd *cobra.Command) {
	AddChainIDFlag(cmd)
	addExecutionControlFlags(cmd)
	addIAVLFlags(cmd)
	addBaseFlags(cmd)
	addAPIFlags(cmd)
	addGRPCFlags(cmd)
	addMempoolFlags(cmd)
	addUnsafeConsensusTimeoutFlags(cmd)
	addStateSyncFlags(cmd)
	chainstreamtypes.AddCmdFlags(cmd)
	addJSONRPCFlags(cmd)
	addJSONRPCDebugFlags(cmd)
	addEVMFlags(cmd)
	addInjWebsocketFlags(cmd)
	addMetricsFlags(cmd)
	addClientFlags(cmd)
	// add support for all CometBFT-specific command line options
	cmtcmd.AddNodeFlags(cmd)
}

func addExecutionControlFlags(cmd *cobra.Command) {
	cmd.Flags().String(flags.FlagHome, DefaultNodeHome, "The application home directory")
	cmd.Flags().String(FlagTraceStore, "", "Enable KVStore tracing to an output file")
	cmd.Flags().IntSlice(server.FlagUnsafeSkipUpgrades, []int{}, "Skip a set of upgrade heights to continue the old binary")
	cmd.Flags().String(FlagCPUProfile, "", "Enable CPU profiling and write to the provided file")
	cmd.Flags().Bool(server.FlagTrace, false, "Provide full stack traces for errors in ABCI Log")
	cmd.Flags().Uint(server.FlagInvCheckPeriod, 0, "Assert registered invariants every N blocks")
	cmd.Flags().Bool(FlagGRPCOnly, false, "Start the node in gRPC query only mode (no CometBFT process is started)")
	cmd.Flags().Duration(server.FlagShutdownGrace, 0*time.Second, "On Shutdown, duration to wait for resource clean up")
	cmd.Flags().Bool(FlagOptimisticExecutionEnabled, false, "Enable optimistic execution (true|false)")
	cmd.Flags().String(FlagAppDBBackend, defaultCfg.AppDBBackend, "Defines the database backend type to use for the application and snapshots DBs.")
}

func addIAVLFlags(cmd *cobra.Command) {
	cmd.Flags().Uint64(server.FlagIAVLCacheSize, defaultCfg.IAVLCacheSize, "Configure IAVL cache size for app")
	cmd.Flags().Bool(server.FlagIAVLSyncPruning, true, "Define if IAVL pruning should use sync mode (true|false)")
}

func addBaseFlags(cmd *cobra.Command) {
	cmd.Flags().String(
		server.FlagMinGasPrices,
		defaultCfg.MinGasPrices,
		"Minimum gas prices to accept for transactions; Any fee in a tx must meet this minimum (e.g. 0.01photino;0.0001stake)",
	)
	cmd.Flags().Uint64(server.FlagQueryGasLimit, defaultCfg.QueryGasLimit, "Maximum gas a Rest/Grpc query can consume. Blank and 0 imply unbounded.")
	cmd.Flags().Uint64(server.FlagHaltHeight, defaultCfg.HaltHeight, "Block height at which to gracefully halt the chain and shutdown the node")
	cmd.Flags().Uint64(server.FlagHaltTime, defaultCfg.HaltTime, "Minimum block time (in Unix seconds) at which to gracefully halt the chain and shutdown the node")
	cmd.Flags().Bool(server.FlagInterBlockCache, defaultCfg.InterBlockCache, "Enable inter-block caching")
	cmd.Flags().String(server.FlagPruning, defaultCfg.Pruning, "Pruning strategy (default|nothing|everything|custom)")
	cmd.Flags().String(server.FlagPruningKeepRecent, defaultCfg.PruningKeepRecent, "Number of recent heights to keep on disk (ignored if pruning is not 'custom')")
	cmd.Flags().String(
		server.FlagPruningInterval,
		defaultCfg.PruningInterval,
		"Height interval at which pruned heights are removed from disk (ignored if pruning is not 'custom')",
	)
	cmd.Flags().Uint64(server.FlagMinRetainBlocks, defaultCfg.MinRetainBlocks, "Minimum block height offset during ABCI commit to prune Tendermint blocks")
	cmd.Flags().Bool(server.FlagDisableIAVLFastNode, defaultCfg.IAVLDisableFastNode, "Define if fast node IAVL should be disabled (default true)")
	cmd.Flags().StringSlice(server.FlagIndexEvents, defaultCfg.IndexEvents,
		"defines the set of events in the form {eventType}.{attributeKey}, which informs CometBFT what to index. If empty, all events will be indexed.")
}

func addAPIFlags(cmd *cobra.Command) {
	cmd.Flags().Bool(server.FlagAPIEnable, defaultCfg.API.Enable, "Define if the API server should be enabled")
	cmd.Flags().Bool(
		server.FlagAPISwagger,
		defaultCfg.API.Swagger,
		"Define if swagger documentation should automatically be registered (Note: the API must also be enabled)",
	)
	cmd.Flags().String(server.FlagAPIAddress, defaultCfg.API.Address, "the API server address to listen on")
	cmd.Flags().Uint(server.FlagAPIMaxOpenConnections, defaultCfg.API.MaxOpenConnections, "Define the number of maximum open connections")
	cmd.Flags().Uint(server.FlagRPCReadTimeout, defaultCfg.API.RPCReadTimeout, "Define the CometBFT RPC read timeout (in seconds)")
	cmd.Flags().Uint(server.FlagRPCWriteTimeout, defaultCfg.API.RPCWriteTimeout, "Define the CometBFT RPC write timeout (in seconds)")
	cmd.Flags().Uint(server.FlagRPCMaxBodyBytes, defaultCfg.API.RPCMaxBodyBytes, "Define the CometBFT maximum request body (in bytes)")
	cmd.Flags().Bool(server.FlagAPIEnableUnsafeCORS, defaultCfg.API.EnableUnsafeCORS, "Define if CORS should be enabled (unsafe - use it at your own risk)")
}

func addGRPCFlags(cmd *cobra.Command) {
	cmd.Flags().Bool(FlagGRPCEnable, defaultCfg.GRPC.Enable, "Define if the gRPC server should be enabled")
	cmd.Flags().String(FlagGRPCAddress, defaultCfg.GRPC.Address, "the gRPC server address to listen on")
	cmd.Flags().Bool(FlagGRPCWebEnable, defaultCfg.GRPCWeb.Enable, "Define if the gRPC-Web server should be enabled. (Note: gRPC must also be enabled.)")
}

func addMempoolFlags(cmd *cobra.Command) {
	cmd.Flags().Int(server.FlagMempoolMaxTxs, defaultCfg.Mempool.MaxTxs, "Sets MaxTx value for the app-side mempool")
	// CometBFT mempool flags
	cmd.Flags().Bool(FlagMempoolRecheckEnabled, true, "Enable rechecking of transactions in the mempool (disable for a faster sync)")
}

func addStateSyncFlags(cmd *cobra.Command) {
	cmd.Flags().Uint64(server.FlagStateSyncSnapshotInterval, defaultCfg.StateSync.SnapshotInterval, "State sync snapshot interval")
	cmd.Flags().Uint32(server.FlagStateSyncSnapshotKeepRecent, defaultCfg.StateSync.SnapshotKeepRecent, "State sync snapshot to keep")
}

func addJSONRPCFlags(cmd *cobra.Command) {
	cmd.Flags().Bool(FlagJSONRPCEnable, defaultCfg.JSONRPC.Enable, "Define if the JSON-RPC server should be enabled")
	cmd.Flags().StringSlice(FlagJSONRPCAPI, defaultCfg.JSONRPC.API, "Defines a list of JSON-RPC namespaces that should be enabled")
	cmd.Flags().String(FlagJSONRPCAddress, defaultCfg.JSONRPC.Address, "The JSON-RPC server address to listen on")
	cmd.Flags().String(FlagJSONWsAddress, defaultCfg.JSONRPC.WsAddress, "The JSON-RPC WS server address to listen on")
	cmd.Flags().Uint64(FlagJSONRPCGasCap, defaultCfg.JSONRPC.GasCap, "Sets a cap on gas that can be used in eth_call/estimateGas (0=infinite)")
	cmd.Flags().Float64(FlagJSONRPCTxFeeCap, defaultCfg.JSONRPC.TxFeeCap, "Sets a cap on transaction fee that can be sent via the RPC APIs (1 = default 1 photon)")
	cmd.Flags().Int32(FlagJSONRPCFilterCap, defaultCfg.JSONRPC.FilterCap, "Sets the global cap for total number of filters that can be created")
	cmd.Flags().Int32(FlagJSONRPCFeeHistoryCap, defaultCfg.JSONRPC.FeeHistoryCap, "Sets the global cap for fee history")
	cmd.Flags().Duration(FlagJSONRPCEVMTimeout, defaultCfg.JSONRPC.EVMTimeout, "Sets a timeout used for eth_call (0=infinite)")
	cmd.Flags().Duration(FlagJSONRPCHTTPTimeout, defaultCfg.JSONRPC.HTTPTimeout, "Sets a read/write timeout for JSON-RPC HTTP server (0=infinite)")
	cmd.Flags().Duration(FlagJSONRPCHTTPIdleTimeout, defaultCfg.JSONRPC.HTTPIdleTimeout, "Sets a idle timeout for JSON-RPC HTTP server (0=infinite)")
	cmd.Flags().Bool(FlagJSONRPCAllowUnprotectedTxs, defaultCfg.JSONRPC.AllowUnprotectedTxs,
		"Allow for unprotected (non EIP155 signed) transactions to be submitted via the node's RPC when the global parameter is disabled")
	cmd.Flags().Int32(FlagJSONRPCLogsCap, defaultCfg.JSONRPC.LogsCap, "Sets the max number of results can be returned from single `eth_getLogs` query")
	cmd.Flags().Int32(FlagJSONRPCBlockRangeCap, defaultCfg.JSONRPC.BlockRangeCap, "Sets the max block range allowed for `eth_getLogs` query")
	cmd.Flags().Int(FlagJSONRPCMaxOpenConnections, defaultCfg.JSONRPC.MaxOpenConnections, "Sets the maximum number of simultaneous connections for the server listener")
	cmd.Flags().Bool(FlagJSONRPCEnableIndexer, defaultCfg.JSONRPC.EnableIndexer, "Enable the custom tx indexer for JSON-RPC")
	cmd.Flags().Bool(FlagJSONRPCAllowIndexerGap, defaultCfg.JSONRPC.AllowIndexerGap, "Allow block gap for the custom tx indexer for JSON-RPC")
	cmd.Flags().Bool(FlagJSONRPCEnableMetrics, defaultCfg.JSONRPC.Metrics, "Define if JSON-RPC rpc metrics server should be enabled")
	cmd.Flags().String(FlagJSONRPCMetricsAddress, defaultCfg.JSONRPC.MetricsAddress, "Set the address for the JSON-RPC metrics server")
	cmd.Flags().Int64(FlagJSONRPCReturnDataLimit, defaultCfg.JSONRPC.ReturnDataLimit, "Set the maximum number of bytes returned from `eth_call` or similar invocations")
}

func addJSONRPCDebugFlags(cmd *cobra.Command) {
	cmd.Flags().Bool(FlagJSONRPCDebugEnable, defaultCfg.JSONRPCDebug.Enable, "Define if the dedicated debug JSON-RPC server should be enabled")
	cmd.Flags().StringSlice(FlagJSONRPCDebugAPI, defaultCfg.JSONRPCDebug.API, "Defines a list of JSON-RPC namespaces that should be enabled on the debug JSON-RPC server")
	cmd.Flags().String(FlagJSONRPCDebugAddress, defaultCfg.JSONRPCDebug.Address, "The debug JSON-RPC server address to listen on")
	cmd.Flags().Uint64(FlagJSONRPCDebugGasCap, defaultCfg.JSONRPCDebug.GasCap, "Sets a cap on gas that can be used in eth_call/estimateGas on debug JSON-RPC (0=infinite)")
	cmd.Flags().Float64(FlagJSONRPCDebugTxFeeCap, defaultCfg.JSONRPCDebug.TxFeeCap,
		"Sets a cap on transaction fee that can be sent via the debug JSON-RPC APIs (1 = default 1 photon)")
	cmd.Flags().Int32(FlagJSONRPCDebugFilterCap, defaultCfg.JSONRPCDebug.FilterCap, "Sets the global cap for total number of filters that can be created on debug JSON-RPC")
	cmd.Flags().Int32(FlagJSONRPCDebugFeeHistoryCap, defaultCfg.JSONRPCDebug.FeeHistoryCap, "Sets the global cap for fee history on debug JSON-RPC")
	cmd.Flags().Duration(FlagJSONRPCDebugEVMTimeout, defaultCfg.JSONRPCDebug.EVMTimeout, "Sets a timeout used for eth_call on debug JSON-RPC (0=infinite)")
	cmd.Flags().Duration(FlagJSONRPCDebugHTTPTimeout, defaultCfg.JSONRPCDebug.HTTPTimeout, "Sets a read/write timeout for debug JSON-RPC HTTP server (0=infinite)")
	cmd.Flags().Duration(FlagJSONRPCDebugHTTPIdleTimeout, defaultCfg.JSONRPCDebug.HTTPIdleTimeout, "Sets an idle timeout for debug JSON-RPC HTTP server (0=infinite)")
	cmd.Flags().Int32(FlagJSONRPCDebugLogsCap, defaultCfg.JSONRPCDebug.LogsCap, "Sets the max number of results returned from a single `eth_getLogs` query on debug JSON-RPC")
	cmd.Flags().Int32(FlagJSONRPCDebugBlockRangeCap, defaultCfg.JSONRPCDebug.BlockRangeCap, "Sets the max block range allowed for `eth_getLogs` query on debug JSON-RPC")
	cmd.Flags().Int(
		FlagJSONRPCDebugMaxOpenConnections,
		defaultCfg.JSONRPCDebug.MaxOpenConnections,
		"Sets the maximum number of simultaneous connections for the debug JSON-RPC server listener",
	)
	cmd.Flags().Int64(
		FlagJSONRPCDebugReturnDataLimit,
		defaultCfg.JSONRPCDebug.ReturnDataLimit,
		"Set the maximum number of bytes returned from `eth_call` or similar invocations on debug JSON-RPC",
	)
}

func addEVMFlags(cmd *cobra.Command) {
	cmd.Flags().String(FlagEVMTracer, defaultCfg.EVM.Tracer,
		"The EVM tracer type to collect execution traces from the EVM transaction execution (json|struct|access_list|markdown)")
	cmd.Flags().Uint64(FlagEVMMaxTxGasWanted, defaultCfg.EVM.MaxTxGasWanted, "The gas wanted for each eth tx returned in ante handler in check tx mode")
	cmd.Flags().Bool(FlagEVMEnableGRPCTracing, defaultCfg.EVM.EnableGRPCTracing, "Enabled or disable TraceTx/TraceBlock/TraceCall gRPC queries")
}

func addInjWebsocketFlags(cmd *cobra.Command) {
	cmd.Flags().String(FlagInjWebsocketAddress, defaultCfg.InjectiveWebsocket.Address, "Address defines the websocket server address to bind to.")
	cmd.Flags().Int(FlagInjWebsocketMaxOpenConnections, defaultCfg.InjectiveWebsocket.MaxOpenConnections,
		"MaxOpenConnections sets the maximum number of simultaneous connections.")
	cmd.Flags().Duration(FlagInjWebsocketReadTimeout, defaultCfg.InjectiveWebsocket.ReadTimeout, "ReadTimeout defines the HTTP read timeout.")
	cmd.Flags().Duration(FlagInjWebsocketWriteTimeout, defaultCfg.InjectiveWebsocket.WriteTimeout, "WriteTimeout defines the HTTP write timeout.")
	cmd.Flags().Int64(FlagInjWebsocketMaxBodyBytes, defaultCfg.InjectiveWebsocket.MaxBodyBytes, "MaxBodyBytes defines the maximum allowed HTTP body size (in bytes).")
	cmd.Flags().Int(FlagInjWebsocketMaxHeaderBytes, defaultCfg.InjectiveWebsocket.MaxHeaderBytes, "MaxHeaderBytes defines the maximum allowed HTTP header size (in bytes).")
	cmd.Flags().Int(FlagInjWebsocketMaxRequestBatchSize, defaultCfg.InjectiveWebsocket.MaxRequestBatchSize,
		"MaxRequestBatchSize defines the maximum number of RPC calls per batch request.")
}

func addUnsafeConsensusTimeoutFlags(cmd *cobra.Command) {
	cmd.Flags().String(FlagUnsafeConsensusTimeoutPropose, "", "Unsafe emergency value for CometBFT consensus timeout_propose. Empty uses Injective's hardcoded value.")
	cmd.Flags().String(FlagUnsafeConsensusTimeoutProposeDelta, "", "Unsafe emergency value for CometBFT consensus timeout_propose_delta. Empty uses Injective's hardcoded value.")
	cmd.Flags().String(FlagUnsafeConsensusTimeoutPrevote, "", "Unsafe emergency value for CometBFT consensus timeout_prevote. Empty uses Injective's hardcoded value.")
	cmd.Flags().String(FlagUnsafeConsensusTimeoutPrevoteDelta, "", "Unsafe emergency value for CometBFT consensus timeout_prevote_delta. Empty uses Injective's hardcoded value.")
	cmd.Flags().String(FlagUnsafeConsensusTimeoutPrecommit, "", "Unsafe emergency value for CometBFT consensus timeout_precommit. Empty uses Injective's hardcoded value.")
	cmd.Flags().String(FlagUnsafeConsensusTimeoutPrecommitDelta, "", "Unsafe emergency value for CometBFT consensus timeout_precommit_delta. Empty uses Injective's hardcoded value.")
	cmd.Flags().String(FlagUnsafeConsensusTimeoutCommit, "", "Unsafe emergency value for CometBFT consensus timeout_commit. Empty uses Injective's hardcoded value.")
}

func addMetricsFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().Bool(FlagMetricsEnableMetrics, defaultCfg.Metrics.MetricsEnabled, "Enable OpenTelemetry metrics")
	cmd.PersistentFlags().Bool(FlagMetricsEnableTracing, defaultCfg.Metrics.TracingEnabled, "Enable OpenTelemetry tracing")
	cmd.PersistentFlags().String(FlagMetricsEndpoint, defaultCfg.Metrics.Endpoint, "OpenTelemetry collector gRPC address")
	cmd.PersistentFlags().Bool(FlagMetricsInsecure, defaultCfg.Metrics.InsecureEndpoint, "Disables TLS encryption for gRPC metrics endpoint communication")
	cmd.PersistentFlags().Duration(FlagMetricsExportInterval, defaultCfg.Metrics.ExportInterval, "time interval between metric exports")
	cmd.PersistentFlags().Duration(FlagMetricsStuckFuncTimeout, defaultCfg.Metrics.StuckFuncTimeout,
		"Sets a duration to consider a function to be stuck to mark in metrics (e.g. in deadlock). 0 disables timeouts.")
	cmd.PersistentFlags().Duration(FlagMetricsFlightRecorderThreshold, defaultCfg.Metrics.FlightRecorderThreshold,
		"Block time threshold after which block is considered slow and flight recorder should dump the profile to disk. 0 = flight recorder disabled")
}

func addClientFlags(cmd *cobra.Command) {
	cmd.Flags().String(flags.FlagKeyringBackend, keyring.BackendFile, "Select keyring's backend (os|file|kwallet|pass|test)")
}

func AddLogFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().String(FlagLogLevel, "", "The logging level (trace|debug|info|warn|error|fatal|panic)")
	cmd.PersistentFlags().String(FlagLogFormat, "", "The logging format (json|plain)")
	cmd.PersistentFlags().Bool(FlagLogNoColor, false, "Disable log coloring")
}

func AddModuleInitFlags(cmd *cobra.Command) {
	crisis.AddModuleInitFlags(cmd)
	wasm.AddModuleInitFlags(cmd)
}

func AddChainIDFlag(cmd *cobra.Command) {
	cmd.PersistentFlags().String(flags.FlagChainID, DefaultChainID, "The network chain ID")
}
