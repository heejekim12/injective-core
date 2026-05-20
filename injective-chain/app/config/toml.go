package config

import (
	"bytes"
	"fmt"
	"os"
	"text/template"
)

const DefaultConfigTemplate = `# This is a TOML config file.
# For more information, see https://github.com/toml-lang/toml

###############################################################################
###                           Base Configuration                            ###
###############################################################################

# The minimum gas prices a validator is willing to accept for processing a
# transaction. A transaction's fees must meet the minimum of any denomination
# specified in this config (e.g. 0.25token1,0.0001token2).
minimum-gas-prices = "{{ .BaseConfig.MinGasPrices }}"

# The maximum gas a query coming over rest/grpc may consume.
# If this is set to zero, the query can consume an unbounded amount of gas.
query-gas-limit = "{{ .BaseConfig.QueryGasLimit }}"

# default: the last 362880 states are kept, pruning at 10 block intervals
# nothing: all historic states will be saved, nothing will be deleted (i.e. archiving node)
# everything: 2 latest states will be kept; pruning at 10 block intervals.
# custom: allow pruning options to be manually specified through 'pruning-keep-recent', and 'pruning-interval'
pruning = "{{ .BaseConfig.Pruning }}"

# These are applied if and only if the pruning strategy is custom.
pruning-keep-recent = "{{ .BaseConfig.PruningKeepRecent }}"
pruning-interval = "{{ .BaseConfig.PruningInterval }}"

# HaltHeight contains a non-zero block height at which a node will gracefully
# halt and shutdown that can be used to assist upgrades and testing.
#
# Note: Commitment of state will be attempted on the corresponding block.
halt-height = {{ .BaseConfig.HaltHeight }}

# HaltTime contains a non-zero minimum block time (in Unix seconds) at which
# a node will gracefully halt and shutdown that can be used to assist upgrades
# and testing.
#
# Note: Commitment of state will be attempted on the corresponding block.
halt-time = {{ .BaseConfig.HaltTime }}

# MinRetainBlocks defines the minimum block height offset from the current
# block being committed, such that all blocks past this offset are pruned
# from CometBFT. It is used as part of the process of determining the
# ResponseCommit.RetainHeight value during ABCI Commit. A value of 0 indicates
# that no blocks should be pruned.
#
# This configuration value is only responsible for pruning CometBFT blocks.
# It has no bearing on application state pruning which is determined by the
# "pruning-*" configurations.
#
# Note: CometBFT block pruning is dependant on this parameter in conjunction
# with the unbonding (safety threshold) period, state pruning and state sync
# snapshot parameters to determine the correct minimum value of
# ResponseCommit.RetainHeight.
min-retain-blocks = {{ .BaseConfig.MinRetainBlocks }}

# InterBlockCache enables inter-block caching.
inter-block-cache = {{ .BaseConfig.InterBlockCache }}

# IndexEvents defines the set of events in the form {eventType}.{attributeKey},
# which informs CometBFT what to index. If empty, all events will be indexed.
#
# Example:
# ["message.sender", "message.recipient"]
index-events = [{{ range .BaseConfig.IndexEvents }}{{ printf "%q, " . }}{{end}}]

# IavlCacheSize set the size of the iavl tree cache (in number of nodes).
iavl-cache-size = {{ .BaseConfig.IAVLCacheSize }}

# IAVLDisableFastNode enables or disables the fast node feature of IAVL. 
# Default is false.
iavl-disable-fastnode = {{ .BaseConfig.IAVLDisableFastNode }}

# AppDBBackend defines the database backend type to use for the application and snapshots DBs.
# An empty string indicates that a fallback will be used.
# The fallback is the db_backend value set in CometBFT's config.toml.
app-db-backend = "{{ .BaseConfig.AppDBBackend }}"

###############################################################################
###                           API Configuration                             ###
###############################################################################

[api]

# Enable defines if the API server should be enabled.
enable = {{ .API.Enable }}

# Swagger defines if swagger documentation should automatically be registered.
swagger = {{ .API.Swagger }}

# Address defines the API server to listen on.
address = "{{ .API.Address }}"

# MaxOpenConnections defines the number of maximum open connections.
max-open-connections = {{ .API.MaxOpenConnections }}

# RPCReadTimeout defines the CometBFT RPC read timeout (in seconds).
rpc-read-timeout = {{ .API.RPCReadTimeout }}

# RPCWriteTimeout defines the CometBFT RPC write timeout (in seconds).
rpc-write-timeout = {{ .API.RPCWriteTimeout }}

# RPCMaxBodyBytes defines the CometBFT maximum request body (in bytes).
rpc-max-body-bytes = {{ .API.RPCMaxBodyBytes }}

# EnableUnsafeCORS defines if CORS should be enabled (unsafe - use it at your own risk).
enabled-unsafe-cors = {{ .API.EnableUnsafeCORS }}

###############################################################################
###                           gRPC Configuration                            ###
###############################################################################

[grpc]

# Enable defines if the gRPC server should be enabled.
enable = {{ .GRPC.Enable }}

# Address defines the gRPC server address to bind to.
address = "{{ .GRPC.Address }}"

# MaxRecvMsgSize defines the max message size in bytes the server can receive.
# The default value is 10MB.
max-recv-msg-size = "{{ .GRPC.MaxRecvMsgSize }}"

# MaxSendMsgSize defines the max message size in bytes the server can send.
# The default value is math.MaxInt32.
max-send-msg-size = "{{ .GRPC.MaxSendMsgSize }}"

###############################################################################
###                        gRPC Web Configuration                           ###
###############################################################################

[grpc-web]

# GRPCWebEnable defines if the gRPC-web should be enabled.
# NOTE: gRPC must also be enabled, otherwise, this configuration is a no-op.
# NOTE: gRPC-Web uses the same address as the API server.
enable = {{ .GRPCWeb.Enable }}

###############################################################################
###                             EVM Configuration                           ###
###############################################################################

[evm]

# Tracer defines the 'vm.Tracer' type that the EVM will use when the node is run in
# debug mode. To enable tracing use the '--evm.tracer' flag when starting your node.
# Valid types are: json|struct|access_list|markdown
tracer = "{{ .EVM.Tracer }}"

# MaxTxGasWanted defines the gas wanted for each eth tx returned in ante handler in check tx mode.
max-tx-gas-wanted = {{ .EVM.MaxTxGasWanted }}

# Enabled or disable TraceTx/TraceBlock/TraceCall gRPC endpoints
enable-grpc-tracing = {{ .EVM.EnableGRPCTracing }}

###############################################################################
###                           JSON RPC Configuration                        ###
###############################################################################

[json-rpc]

# Enable defines if the gRPC server should be enabled.
enable = {{ .JSONRPC.Enable }}

# Address defines the EVM RPC HTTP server address to bind to.
address = "{{ .JSONRPC.Address }}"

# Address defines the EVM WebSocket server address to bind to.
ws-address = "{{ .JSONRPC.WsAddress }}"

# API defines a list of JSON-RPC namespaces that should be enabled
# Example: "eth,txpool,personal,net,debug,web3"
api = "{{range $index, $elmt := .JSONRPC.API}}{{if $index}},{{$elmt}}{{else}}{{$elmt}}{{end}}{{end}}"

# GasCap sets a cap on gas that can be used in eth_call/estimateGas (0=infinite).
gas-cap = {{ .JSONRPC.GasCap }}

# EVMTimeout is the global timeout for eth_call. Default: 5s.
evm-timeout = "{{ .JSONRPC.EVMTimeout }}"

# TxFeeCap is the global tx-fee cap for send transaction. Default: 10eth.
txfee-cap = {{ .JSONRPC.TxFeeCap }}

# FilterCap sets the global cap for total number of filters that can be created
filter-cap = {{ .JSONRPC.FilterCap }}

# FeeHistoryCap sets the global cap for total number of blocks that can be fetched
feehistory-cap = {{ .JSONRPC.FeeHistoryCap }}

# LogsCap defines the max number of results can be returned from single 'eth_getLogs' query.
logs-cap = {{ .JSONRPC.LogsCap }}

# BlockRangeCap defines the max block range allowed for 'eth_getLogs' query.
block-range-cap = {{ .JSONRPC.BlockRangeCap }}

# HTTPTimeout is the read/write timeout of http json-rpc server.
http-timeout = "{{ .JSONRPC.HTTPTimeout }}"

# HTTPIdleTimeout is the idle timeout of http json-rpc server.
http-idle-timeout = "{{ .JSONRPC.HTTPIdleTimeout }}"

# AllowUnprotectedTxs restricts unprotected (non EIP155 signed) transactions to be submitted via
# the node's RPC when the global parameter is disabled.
allow-unprotected-txs = {{ .JSONRPC.AllowUnprotectedTxs }}

# MaxOpenConnections sets the maximum number of simultaneous connections
# for the server listener.
max-open-connections = {{ .JSONRPC.MaxOpenConnections }}

# EnableIndexer enables the custom transaction indexer for the EVM (ethereum transactions).
enable-indexer = {{ .JSONRPC.EnableIndexer }}

# AllowIndexerGap allow block gap for the custom transaction indexer for the EVM (ethereum transactions).
allow-indexer-gap = {{ .JSONRPC.AllowIndexerGap }}

# Define if JSON-RPC metrics server should be enabled.
metrics = {{ .JSONRPC.Metrics }}

# MetricsAddress defines the EVM Metrics server address to bind to.
# Prometheus metrics path: /debug/metrics/prometheus
metrics-address = "{{ .JSONRPC.MetricsAddress }}"

# Maximum number of bytes returned from eth_call or similar invocations.
return-data-limit = {{ .JSONRPC.ReturnDataLimit }}

###############################################################################
###                     Debug JSON RPC Configuration                        ###
###############################################################################

[json-rpc-debug]

# Enable defines if the dedicated debug JSON-RPC server should be enabled.
enable = {{ .JSONRPCDebug.Enable }}

# Address defines the debug JSON-RPC HTTP server address to bind to.
address = "{{ .JSONRPCDebug.Address }}"

# API defines a list of JSON-RPC namespaces that should be enabled
# Example: "eth,debug"
api = "{{range $index, $elmt := .JSONRPCDebug.API}}{{if $index}},{{$elmt}}{{else}}{{$elmt}}{{end}}{{end}}"

# GasCap sets a cap on gas that can be used in eth_call/estimateGas (0=infinite).
gas-cap = {{ .JSONRPCDebug.GasCap }}

# EVMTimeout is the global timeout for eth_call. Default: 5s.
evm-timeout = "{{ .JSONRPCDebug.EVMTimeout }}"

# TxFeeCap is the global tx-fee cap for send transaction. Default: 10eth.
txfee-cap = {{ .JSONRPCDebug.TxFeeCap }}

# FilterCap sets the global cap for total number of filters that can be created
filter-cap = {{ .JSONRPCDebug.FilterCap }}

# FeeHistoryCap sets the global cap for total number of blocks that can be fetched
feehistory-cap = {{ .JSONRPCDebug.FeeHistoryCap }}

# LogsCap defines the max number of results can be returned from single 'eth_getLogs' query.
logs-cap = {{ .JSONRPCDebug.LogsCap }}

# BlockRangeCap defines the max block range allowed for 'eth_getLogs' query.
block-range-cap = {{ .JSONRPCDebug.BlockRangeCap }}

# HTTPTimeout is the read/write timeout of http json-rpc server.
http-timeout = "{{ .JSONRPCDebug.HTTPTimeout }}"

# HTTPIdleTimeout is the idle timeout of http json-rpc server.
http-idle-timeout = "{{ .JSONRPCDebug.HTTPIdleTimeout }}"

# MaxOpenConnections sets the maximum number of simultaneous connections
# for the server listener.
max-open-connections = {{ .JSONRPCDebug.MaxOpenConnections }}

# Maximum number of bytes returned from eth_call or similar invocations.
return-data-limit = {{ .JSONRPCDebug.ReturnDataLimit }}

###############################################################################
###                  Injective Websocket Configuration                      ###
###############################################################################

[injective-websocket]

# Address defines the websocket server address to bind to.
address = "{{ .InjectiveWebsocket.Address }}"

# MaxOpenConnections sets the maximum number of simultaneous connections.
max-open-connections = {{ .InjectiveWebsocket.MaxOpenConnections }}

# ReadTimeout defines the HTTP read timeout.
read-timeout = "{{ .InjectiveWebsocket.ReadTimeout }}"

# WriteTimeout defines the HTTP write timeout.
write-timeout = "{{ .InjectiveWebsocket.WriteTimeout }}"

# MaxBodyBytes defines the maximum allowed HTTP body size (in bytes).
max-body-bytes = {{ .InjectiveWebsocket.MaxBodyBytes }}

# MaxHeaderBytes defines the maximum allowed HTTP header size (in bytes).
max-header-bytes = {{ .InjectiveWebsocket.MaxHeaderBytes }}

# MaxRequestBatchSize defines the maximum number of RPC calls per batch request.
max-request-batch-size = {{ .InjectiveWebsocket.MaxRequestBatchSize }}

###############################################################################
###                        State Sync Configuration                         ###
###############################################################################

# State sync snapshots allow other nodes to rapidly join the network without replaying historical
# blocks, instead downloading and applying a snapshot of the application state at a given height.
[state-sync]

# snapshot-interval specifies the block interval at which local state sync snapshots are
# taken (0 to disable).
snapshot-interval = {{ .StateSync.SnapshotInterval }}

# snapshot-keep-recent specifies the number of recent snapshots to keep and serve (0 to keep all).
snapshot-keep-recent = {{ .StateSync.SnapshotKeepRecent }}

###############################################################################
###                              State Streaming                            ###
###############################################################################

# Streaming allows nodes to stream state to external systems.
[streaming]

# streaming.abci specifies the configuration for the ABCI Listener streaming service.
[streaming.abci]

# List of kv store keys to stream out via gRPC.
# The store key names MUST match the module's StoreKey name.
#
# Example:
# ["acc", "bank", "gov", "staking", "mint"[,...]]
# ["*"] to expose all keys.
keys = [{{ range .Streaming.ABCI.Keys }}{{ printf "%q, " . }}{{end}}]

# The plugin name used for streaming via gRPC.
# Streaming is only enabled if this is set.
# Supported plugins: abci
plugin = "{{ .Streaming.ABCI.Plugin }}"

# stop-node-on-err specifies whether to stop the node on message delivery error.
stop-node-on-err = {{ .Streaming.ABCI.StopNodeOnErr }}

###############################################################################
###                         Mempool                                         ###
###############################################################################

[mempool]
# Setting max-txs to 0 will allow for a unbounded amount of transactions in the mempool.
# Setting max_txs to negative 1 (-1) will disable transactions from being inserted into the mempool (no-op mempool).
# Setting max_txs to a positive number (> 0) will limit the number of transactions in the mempool, by the specified amount.
#
# Note, this configuration only applies to SDK built-in app-side mempool
# implementations.
max-txs = {{ .Mempool.MaxTxs }}

###############################################################################
###                         ChainStream Server                              ###
###############################################################################

[chainstream]

# ChainStream server address to bind to. Empty = disabled.
server = "{{ .ChainStream.ServerAddress }}"

# Configure ChainStream server buffer capacity for each connected client
buffer-cap = "{{ .ChainStream.ServerBufferCapacity }}"

# Configure ChainStream publisher buffer capacity
publisher-buffer-cap = "{{ .ChainStream.PublisherBufferCapacity }}"

# Define if Keepalive configuration params should be applied to chainstream gRPC server
enforce-keepalive = "{{ .ChainStream.EnforceKeepalive }}"

# Amount of time (in seconds) a client should wait before sending a keepalive ping
min-client-ping-interval = "{{ .ChainStream.MinClientPingInterval }}"

# Amount of time in seconds a connection is allowed to stay idle before forcing the disconnection
max-connection-idle = "{{ .ChainStream.MaxConnectionIdle }}"

# Amount of time in seconds after which the server will send a keepalive ping to the client on an idle connection
server-ping-interval = "{{ .ChainStream.ServerPingInterval }}"

# Amount of time in seconds the server waits for the client to respond to a ping message before forcing a disconnection
server-ping-response-timeout = "{{ .ChainStream.ServerPingResponseTimeout }}"

###############################################################################
###                         Metrics Configuration                         ###
###############################################################################

[metrics]

# whether metrics collections should be enabled, default: false
metrics-enabled = {{ .Metrics.MetricsEnabled }}

# whether tracing should be enabled, default: false
tracing-enabled = {{ .Metrics.TracingEnabled }}

# gRPC OTEL receiver endpoint, default: localhost:4317
endpoint = "{{ .Metrics.Endpoint }}"

# whether to use TLS during Endpoint connection
insecure-endpoint = {{ .Metrics.InsecureEndpoint }}

# time interval between metric exports, default: 10s
export-interval = "{{ .Metrics.ExportInterval }}"

# timeout before FuncTiming marks the function as stuck and ends the span.
# Set to 0 (default) to disable timeouts completely.
stuck-func-timeout = "{{ .Metrics.StuckFuncTimeout }}"

# time after which block is considered slow and flight recorder should dump the profile to disk.
# 0 - disables flight recorder.
flight-recorder-threshold = "{{ .Metrics.FlightRecorderThreshold }}"

`

var configTemplate *template.Template

func init() {
	var err error

	tmpl := template.New("appConfigFileTemplate")

	if configTemplate, err = tmpl.Parse(DefaultConfigTemplate); err != nil {
		panic(err)
	}
}

// SetConfigTemplate sets the custom app config template for
// the application
func SetConfigTemplate(customTemplate string) {
	var err error

	tmpl := template.New("appConfigFileTemplate")

	if configTemplate, err = tmpl.Parse(customTemplate); err != nil {
		panic(err)
	}
}

// WriteConfigFile renders config using the template and writes it to
// configFilePath.
func WriteConfigFile(configFilePath string, config interface{}) {
	var buffer bytes.Buffer

	if err := configTemplate.Execute(&buffer, config); err != nil {
		panic(err)
	}

	mustWriteFile(configFilePath, buffer.Bytes(), 0o644)
}

func mustWriteFile(filePath string, contents []byte, mode os.FileMode) {
	if err := os.WriteFile(filePath, contents, mode); err != nil {
		panic(fmt.Errorf("failed to write file: %w", err))
	}
}
