package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime/pprof"
	"time"

	"cosmossdk.io/log"
	"github.com/InjectiveLabs/metrics/v2"
	"github.com/InjectiveLabs/metrics/v2/flightrecorder"
	cmtconfig "github.com/cometbft/cometbft/config"
	cmtcrypto "github.com/cometbft/cometbft/crypto"
	cmted22519 "github.com/cometbft/cometbft/crypto/ed25519"
	"github.com/cometbft/cometbft/crypto/tmhash"
	"github.com/cometbft/cometbft/node"
	"github.com/cometbft/cometbft/p2p"
	pvm "github.com/cometbft/cometbft/privval"
	"github.com/cometbft/cometbft/proxy"
	rpcclient "github.com/cometbft/cometbft/rpc/client"
	"github.com/cometbft/cometbft/rpc/client/local"
	rpcserver "github.com/cometbft/cometbft/rpc/jsonrpc/server"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/server"
	"github.com/cosmos/cosmos-sdk/server/api"
	serverconfig "github.com/cosmos/cosmos-sdk/server/config"
	servergrpc "github.com/cosmos/cosmos-sdk/server/grpc"
	servercmtlog "github.com/cosmos/cosmos-sdk/server/log"
	"github.com/cosmos/cosmos-sdk/server/types"
	genutiltypes "github.com/cosmos/cosmos-sdk/x/genutil/types"
	"github.com/improbable-eng/grpc-web/go/grpcweb"
	"github.com/rs/cors"
	"github.com/spf13/cobra"
	"github.com/xlab/closer"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	injectivechain "github.com/InjectiveLabs/injective-core/injective-chain/app"
	"github.com/InjectiveLabs/injective-core/injective-chain/app/config"
	ethserver "github.com/InjectiveLabs/injective-core/injective-chain/server"
	ethindexer "github.com/InjectiveLabs/injective-core/injective-chain/server/indexer"
	"github.com/InjectiveLabs/injective-core/injective-chain/server/jsonrpc"
	chaintypes "github.com/InjectiveLabs/injective-core/injective-chain/types"
)

// StartCmd runs the service passed in, either stand-alone or in-process with
// CometBFT.
func StartCmd(appCreator config.InjAppCreator) *cobra.Command {
	return StartCmdWithOptions(appCreator)
}

// StartCmdOptions defines options that can be customized in `StartCmdWithOptions`,
func StartCmdWithOptions(appCreator config.InjAppCreator) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Run the full node",
		Long: `Run the full node application with CometBFT in or out of process. By
default, the application will run with CometBFT in process.

Pruning options can be provided via the '--pruning' flag or alternatively with '--pruning-keep-recent', and
'pruning-interval' together.

For '--pruning' the options are as follows:

default: the last 362880 states are kept, pruning at 10 block intervals
nothing: all historic states will be saved, nothing will be deleted (i.e. archiving node)
everything: 2 latest states will be kept; pruning at 10 block intervals.
custom: allow pruning options to be manually specified through 'pruning-keep-recent', and 'pruning-interval'

Node halting configurations exist in the form of two flags: '--halt-height' and '--halt-time'. During
the ABCI Commit phase, the node will check if the current block height is greater than or equal to
the halt-height or if the current block time is greater than or equal to the halt-time. If so, the
node will attempt to gracefully shutdown and the block will not be committed. In addition, the node
will not be able to commit subsequent blocks.

For profiling and benchmarking purposes, CPU profiling can be enabled via the '--cpu-profile' flag
which accepts a path for the resulting pprof file.

The node may be started in a 'query only' mode where only the gRPC and JSON HTTP
API services are enabled via the 'grpc-only' flag. In this mode, CometBFT is
bypassed and can be used when legacy queries are needed after an on-chain upgrade
is performed. Note, when enabled, gRPC will also be automatically enabled.
`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			serverCtx := server.GetServerContextFromCmd(cmd)
			clientCtx, err := client.GetClientQueryContext(cmd)
			if err != nil {
				return err
			}

			err = wrapCPUProfile(serverCtx, func() error {
				return start(serverCtx, clientCtx, appCreator)
			})

			serverCtx.Logger.Debug("received quit signal")
			graceDuration, _ := cmd.Flags().GetDuration(server.FlagShutdownGrace)
			if graceDuration > 0 {
				serverCtx.Logger.Info("graceful shutdown start", server.FlagShutdownGrace, graceDuration)
				<-time.After(graceDuration)
				serverCtx.Logger.Info("graceful shutdown complete")
			}

			return err
		},
	}

	config.AddStartNodeFlags(cmd)
	config.AddModuleInitFlags(cmd)

	return cmd
}

func start(svrCtx *server.Context, clientCtx client.Context, appCreator config.InjAppCreator) error {
	cfg, err := getAndValidateConfig(svrCtx)
	if err != nil {
		return fmt.Errorf("failed to parse app config: %w", err)
	}

	if cfg.JSONRPC.Enable && cfg.JSONRPCDebug.Enable &&
		cfg.JSONRPC.EnableIndexer && cfg.JSONRPCDebug.EnableIndexer {
		return errors.New("cannot enable indexer on both json-rpc and json-rpc-debug servers simultaneously")
	}

	app, appCleanupFn, err := startApp(cfg, svrCtx.Logger, appCreator)
	if err != nil {
		return err
	}
	defer appCleanupFn()

	if err := startMetrics(svrCtx, app.(*injectivechain.InjectiveApp), cfg.Metrics); err != nil {
		return err
	}

	return startInProcess(svrCtx, cfg, clientCtx, app)
}

func startCmtNode(
	ctx context.Context,
	cfg *cmtconfig.Config,
	app types.Application,
	logger log.Logger,
) (tmNode *node.Node, cleanupFn func(), err error) {
	nodeKey, err := p2p.LoadOrGenNodeKey(cfg.NodeKeyFile())
	if err != nil {
		return nil, cleanupFn, err
	}

	cmtApp := server.NewCometABCIWrapper(app)
	privValidator, err := pvm.LoadOrGenFilePV(
		cfg.PrivValidatorKeyFile(),
		cfg.PrivValidatorStateFile(),
		func() (cmtcrypto.PrivKey, error) { return cmted22519.GenPrivKey(), nil },
	)
	if err != nil {
		return nil, cleanupFn, err
	}

	tmNode, err = node.NewNode(
		ctx,
		cfg,
		privValidator,
		nodeKey,
		proxy.NewLocalClientCreator(cmtApp),
		getGenDocProvider(cfg),
		cmtconfig.DefaultDBProvider,
		node.DefaultMetricsProvider(cfg.Instrumentation),
		servercmtlog.CometLoggerWrapper{Logger: logger},
		getCometMeter(app),
	)
	if err != nil {
		return tmNode, cleanupFn, err
	}

	if err := tmNode.Start(); err != nil {
		return tmNode, cleanupFn, err
	}

	cleanupFn = func() {
		if tmNode != nil && tmNode.IsRunning() {
			_ = tmNode.Stop()
		}
	}

	return tmNode, cleanupFn, nil
}

func getCometMeter(app types.Application) metrics.Meter {
	meteredApp, ok := app.(interface{ Meter() metrics.Meter })
	if !ok {
		return metrics.NewNilMeter()
	}

	appMeter := meteredApp.Meter()
	if appMeter == nil {
		return metrics.NewNilMeter()
	}

	return appMeter.SubMeter("cometbft")
}

func getAndValidateConfig(svrCtx *server.Context) (config.Config, error) {
	cfg, err := config.GetConfig(svrCtx.Viper)
	if err != nil {
		return cfg, err
	}

	if err := cfg.Validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// returns a function which returns the genesis doc from the genesis file.
func getGenDocProvider(cfg *cmtconfig.Config) node.GenesisDocProvider {
	return func() (node.ChecksummedGenesisDoc, error) {
		jsonBlob, err := os.ReadFile(cfg.GenesisFile())
		if err != nil {
			return node.ChecksummedGenesisDoc{}, fmt.Errorf("couldn't read GenesisDoc file: %w", err)
		}

		incomingChecksum := tmhash.Sum(jsonBlob)

		appGenesis, err := genutiltypes.AppGenesisFromFile(cfg.GenesisFile())
		if err != nil {
			return node.ChecksummedGenesisDoc{}, err
		}

		genDoc, err := appGenesis.ToGenesisDoc()
		if err != nil {
			return node.ChecksummedGenesisDoc{}, err
		}

		return node.ChecksummedGenesisDoc{GenesisDoc: genDoc, Sha256Checksum: incomingChecksum}, nil
	}
}

func setupTraceWriter(cfg config.Config, logger log.Logger) (traceWriter io.WriteCloser, cleanup func(), err error) {
	// clean up the traceWriter when the server is shutting down
	cleanup = func() {}

	traceWriterFile := cfg.Get(config.FlagTraceStore).(string) //nolint
	traceWriter, err = openTraceWriter(traceWriterFile)
	if err != nil {
		return traceWriter, cleanup, err
	}

	// if flagTraceStore is not used then traceWriter is nil
	if traceWriter != nil {
		cleanup = func() {
			if err = traceWriter.Close(); err != nil {
				logger.Error("failed to close trace writer", "err", err)
			}
		}
	}

	return traceWriter, cleanup, nil
}

func startGrpcServer(
	ctx context.Context,
	g *errgroup.Group,
	srvConfig serverconfig.GRPCConfig,
	clientCtx client.Context,
	logger log.Logger,
	app types.Application,
) (*grpc.Server, client.Context, error) {
	if !srvConfig.Enable {
		// return grpcServer as nil if gRPC is disabled
		return nil, clientCtx, nil
	}
	_, _, err := net.SplitHostPort(srvConfig.Address)
	if err != nil {
		return nil, clientCtx, err
	}

	maxSendMsgSize := srvConfig.MaxSendMsgSize
	if maxSendMsgSize == 0 {
		maxSendMsgSize = serverconfig.DefaultGRPCMaxSendMsgSize
	}

	maxRecvMsgSize := srvConfig.MaxRecvMsgSize
	if maxRecvMsgSize == 0 {
		maxRecvMsgSize = serverconfig.DefaultGRPCMaxRecvMsgSize
	}

	// if gRPC is enabled, configure gRPC client for gRPC gateway
	grpcClient, err := grpc.NewClient(
		srvConfig.Address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(
			grpc.ForceCodec(codec.NewProtoCodec(clientCtx.InterfaceRegistry).GRPCCodec()),
			grpc.MaxCallRecvMsgSize(maxRecvMsgSize),
			grpc.MaxCallSendMsgSize(maxSendMsgSize),
		),
	)
	if err != nil {
		return nil, clientCtx, err
	}

	clientCtx = clientCtx.WithGRPCClient(grpcClient)
	logger.Debug("gRPC client assigned to client context", "target", srvConfig.Address)

	grpcSrv, err := servergrpc.NewGRPCServer(clientCtx, app, srvConfig)
	if err != nil {
		return nil, clientCtx, err
	}

	// Start the gRPC server in a goroutine. Note, the provided ctx will ensure
	// that the server is gracefully shut down.
	g.Go(func() error {
		return servergrpc.StartGRPCServer(ctx, logger.With("module", "grpc-server"), srvConfig, grpcSrv)
	})

	return grpcSrv, clientCtx, nil
}

func startAPIServer(
	ctx context.Context,
	g *errgroup.Group,
	cfg config.Config,
	clientCtx client.Context,
	logger log.Logger,
	app types.Application,
	home string,
	grpcSrv *grpc.Server,
) {
	if !cfg.API.Enable {
		return
	}

	clientCtx = clientCtx.WithHomeDir(home)

	apiSrv := api.New(clientCtx, logger.With("module", "api-server"), grpcSrv)
	app.RegisterAPIRoutes(apiSrv, cfg.API)

	g.Go(func() error {
		return apiSrv.Start(ctx, cfg.SDKConfig())
	})
}

func startStreamingServers(
	injApp *injectivechain.InjectiveApp,
	cfg config.Config,
	logger log.Logger,
) error {
	if cfg.ChainStream.ServerAddress == "" {
		logger.Info("chainstream server is disabled; not starting chainstream server")
		return nil
	}

	injApp.ChainStreamServer.WithBufferCapacity(cfg.ChainStream.ServerBufferCapacity)
	injApp.EventPublisher.WithBufferCapacity(cfg.ChainStream.PublisherBufferCapacity)
	injApp.EnableStreamer = true

	if err := injApp.EventPublisher.Run(context.Background()); err != nil {
		logger.Error("failed to start event publisher", "error", err)
		return nil
	}

	if err := injApp.ChainStreamServer.Serve(cfg.ChainStream.ServerAddress); err != nil {
		logger.Error("failed to start chainstream server", "error", err)
		return nil
	}

	return startWebsocketServer(injApp, cfg.InjectiveWebsocket, logger)
}

func startWebsocketServer(
	injApp *injectivechain.InjectiveApp,
	wsCfg config.WebsocketConfig,
	logger log.Logger,
) error {
	if wsCfg.Address == "" {
		logger.Info("websocket server is disabled; not starting websocket server")
		return nil
	}

	injApp.WebsocketServer.WithRPCConfig(func(cfg *rpcserver.Config) {
		cfg.MaxOpenConnections = wsCfg.MaxOpenConnections
		cfg.ReadTimeout = wsCfg.ReadTimeout
		cfg.WriteTimeout = wsCfg.WriteTimeout
		cfg.MaxBodyBytes = wsCfg.MaxBodyBytes
		cfg.MaxHeaderBytes = wsCfg.MaxHeaderBytes
		cfg.MaxRequestBatchSize = wsCfg.MaxRequestBatchSize
	})

	if err := injApp.WebsocketServer.Serve(wsCfg.Address); err != nil {
		logger.Error("failed to start websocket server", "error", err)
	}

	return nil
}

func startMetrics(ctx *server.Context, app *injectivechain.InjectiveApp, cfg metrics.Config) error {
	appMetrics, err := metrics.NewMetrics(cfg, metrics.Tag("chain-id", app.ChainID()), metrics.Tag(metrics.ServiceNameKey, "injective-core"))
	if err != nil {
		return err
	}

	appMeter, err := appMetrics.NewMeter("app")
	if err != nil {
		return err
	}

	app.SetMeter(appMeter)
	closer.Bind(func() {
		appMetrics.Shutdown() //nolint:errcheck //ok
	})

	// Trace Flight Recorder
	if cfg.FlightRecorderThreshold > 0 {
		tr := flightrecorder.NewTraceRecorder(time.Minute, cfg.FlightRecorderThreshold, 1024*1024*1024*4)
		if err := tr.Start(); err != nil {
			return err
		}
		ctx.Logger.Info("Started Trace Flight Recorder", "threshold", cfg.FlightRecorderThreshold)
		closer.Bind(func() {
			tr.Stop()
		})

		app.SetTraceFlightRecorder(tr)
	}

	return nil
}

func startInProcess(
	svrCtx *server.Context,
	cfg config.Config,
	clientCtx client.Context,
	app types.Application,
) error {
	closer.Init(closer.Config{
		ExitCodeOK:  closer.ExitCodeOK,
		ExitCodeErr: closer.ExitCodeErr,
		ExitSignals: closer.DebugSignalSet,
	})
	cmtCfg := svrCtx.Config
	g, ctx := getCtx(svrCtx, true)
	svrCtx.Logger.Info("starting node with ABCI CometBFT in-process")
	tmNode, cleanupFn, err := startCmtNode(ctx, cmtCfg, app, svrCtx.Logger)
	if err != nil {
		return err
	}

	defer cleanupFn()

	clientCtx = registerTxServices(tmNode, clientCtx, cfg, app)

	grpcSrv, clientCtx, err := startGrpcServer(ctx, g, cfg.GRPC, clientCtx, svrCtx.Logger, app)
	if err != nil {
		return err
	}

	startAPIServer(ctx, g, cfg, clientCtx, svrCtx.Logger, app, cfg.GetHome(), grpcSrv)

	if injApp, ok := app.(*injectivechain.InjectiveApp); ok {
		if err := startStreamingServers(injApp, cfg, svrCtx.Logger); err != nil {
			return err
		}
	}

	if cfg.JSONRPC.Enable {
		if _, _, _, err := startJSONRPCServer(
			svrCtx,
			clientCtx,
			cfg.JSONRPC,
			cfg.API.EnableUnsafeCORS,
			g,
			false,
		); err != nil {
			return err
		}
	}

	if cfg.JSONRPCDebug.Enable {
		if _, _, _, err := startJSONRPCServer(
			svrCtx,
			clientCtx,
			cfg.JSONRPCDebug,
			cfg.API.EnableUnsafeCORS,
			g,
			true,
		); err != nil {
			return err
		}
	}

	closer.Bind(makeCleanupHandler(tmNode, app, svrCtx))
	closer.Hold()

	return g.Wait()
}

func makeCleanupHandler(tmNode *node.Node, app types.Application, svrCtx *server.Context) func() {
	return func() {
		if tmNode.IsRunning() {
			_ = tmNode.Stop()
		}

		if injApp, ok := app.(*injectivechain.InjectiveApp); ok {
			// Stop websocket server first (if running) to stop accepting new subscriptions
			if injApp.WebsocketServer != nil {
				injApp.WebsocketServer.Stop()
			}

			if injApp.ChainStreamServer != nil {
				injApp.ChainStreamServer.Stop()
			}

			if injApp.EventPublisher != nil {
				if err := injApp.EventPublisher.Stop(); err != nil {
					svrCtx.Logger.Error("failed to stop event publisher", "error", err)
				}
			}
		}

		svrCtx.Logger.Info("Bye!")
	}
}

// registerTxServices adds the tx service to the gRPC router when API or gRPC is enabled.
func registerTxServices(
	tmNode *node.Node,
	clientCtx client.Context,
	cfg config.Config,
	app types.Application,
) client.Context {
	if !cfg.API.Enable && !cfg.GRPC.Enable {
		return clientCtx
	}

	clientCtx = clientCtx.WithClient(local.New(tmNode))
	app.RegisterTxService(clientCtx)
	app.RegisterTendermintService(clientCtx)
	app.RegisterNodeService(clientCtx, cfg.SDKConfig())

	return clientCtx
}

func startJSONRPCServer(
	svrCtx *server.Context,
	clientCtx client.Context,
	jsonRPCConfig config.JSONRPCConfig,
	enableUsafeCors bool,
	g *errgroup.Group,
	isDebug bool,
) (ctx client.Context, httpSrv *http.Server, httpSrvDone chan struct{}, err error) {
	ctx = clientCtx

	genDoc, err := getGenDocProvider(svrCtx.Config)()
	if err != nil {
		return ctx, httpSrv, httpSrvDone, fmt.Errorf("can't get genDocProvider")
	}

	chainId := genDoc.GenesisDoc.ChainID
	home := svrCtx.Config.RootDir
	ctx = ctx.WithChainID(chainId).WithHomeDir(home)

	var idxer chaintypes.EVMTxIndexer
	if !isDebug && jsonRPCConfig.EnableIndexer {
		var idxDB dbm.DB
		idxDB, err = openIndexerDB(home, server.GetAppDBBackend(svrCtx.Viper))
		if err != nil {
			svrCtx.Logger.Error("failed to open EVM indexer DB", "error", err.Error())
			return
		}

		idxLogger := svrCtx.Logger.With("indexer", "evm")
		idxer = ethindexer.NewKVIndexer(idxDB, idxLogger, clientCtx)
		indexerService := ethserver.NewEVMIndexerService(idxer, clientCtx.Client.(rpcclient.Client), jsonRPCConfig.AllowIndexerGap)
		indexerService.SetLogger(servercmtlog.CometLoggerWrapper{Logger: idxLogger})

		g.Go(func() error {
			defer func() {
				if e := recover(); e != nil {
					idxLogger.Error("panic in EVM indexer service", "error", e)
				}
			}()

			defer func() {
				if err := idxDB.Close(); err != nil {
					idxLogger.Error("failed to close EVM indexer DB", "error", err.Error())
				}
			}()

			return indexerService.Start()
		})
	}

	g.Go(func() error {
		defer func() {
			if e := recover(); e != nil {
				svrCtx.Logger.Error("panic in EVM JSON-RPC service", "error", e, "debug", isDebug)
			}
		}()

		httpSrv, httpSrvDone, err = jsonrpc.Start(
			svrCtx,
			ctx,
			g,
			jsonRPCConfig,
			enableUsafeCors,
			idxer,
			isDebug,
		)

		return err
	})

	return
}

// StartGRPCWeb starts a gRPC-Web server on the given address.
func StartGRPCWeb(ctx *server.Context, grpcSrv *grpc.Server, parsedConfig serverconfig.Config) (*http.Server, error) {
	wrappedServer := grpcweb.WrapServer(grpcSrv)
	handler := func(resp http.ResponseWriter, req *http.Request) {
		wrappedServer.ServeHTTP(resp, req)
	}

	handlerWithCors := cors.New(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{
			http.MethodHead,
			http.MethodGet,
			http.MethodPost,
			http.MethodPut,
			http.MethodPatch,
			http.MethodDelete,
		},
		AllowedHeaders:     []string{"*"},
		AllowCredentials:   false,
		OptionsPassthrough: false,
	})

	grpcWebSrv := &http.Server{
		Addr:    parsedConfig.GRPC.Address,
		Handler: handlerWithCors.Handler(http.HandlerFunc(handler)),
	}

	errCh := make(chan error)
	go func() {
		ctx.Logger.Info("Starting GRPC Web server on", "address", parsedConfig.GRPC.Address)
		if err := grpcWebSrv.ListenAndServe(); err != nil {
			if errors.Is(err, http.ErrServerClosed) {
				return
			}

			ctx.Logger.Error("failed to start GRPC Web server", "error", err)
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		ctx.Logger.Error("failed to boot GRPC Web server", "error", err)
		return nil, err
	case <-time.After(1 * time.Second):
	}

	return grpcWebSrv, nil
}

func openDB(rootDir string, backendType dbm.BackendType) (dbm.DB, error) {
	dataDir := filepath.Join(rootDir, "data")
	return dbm.NewDB("application", backendType, dataDir)
}

// openIndexerDB opens the custom eth indexer db, using the same db backend as the main app
func openIndexerDB(rootDir string, backendType dbm.BackendType) (dbm.DB, error) {
	dataDir := filepath.Join(rootDir, "data")
	return dbm.NewDB("evmindexer", backendType, dataDir)
}

func openTraceWriter(traceWriterFile string) (w io.WriteCloser, err error) {
	if traceWriterFile == "" {
		return
	}
	return os.OpenFile(
		traceWriterFile,
		os.O_WRONLY|os.O_APPEND|os.O_CREATE,
		0o666,
	)
}

func getCtx(svrCtx *server.Context, block bool) (*errgroup.Group, context.Context) {
	ctx, cancelFn := context.WithCancel(context.Background())
	g, ctx := errgroup.WithContext(ctx)
	// listen for quit signals so the calling parent process can gracefully exit
	server.ListenForQuitSignals(g, block, cancelFn, svrCtx.Logger)
	return g, ctx
}

func startApp(cfg config.Config, logger log.Logger, appCreator config.InjAppCreator,
) (app types.Application, cleanupFn func(), err error) {
	traceWriter, traceCleanupFn, err := setupTraceWriter(cfg, logger)
	if err != nil {
		return app, traceCleanupFn, err
	}

	db, err := openDB(cfg.GetHome(), cfg.GetDBBackend())
	if err != nil {
		return app, traceCleanupFn, err
	}

	app = appCreator(logger, db, traceWriter, cfg)

	cleanupFn = func() {
		traceCleanupFn()
		if localErr := app.Close(); localErr != nil {
			logger.Error(localErr.Error())
		}
	}
	return app, cleanupFn, nil
}

// wrapCPUProfile starts CPU profiling, if enabled, and executes the provided
// callbackFn in a separate goroutine, then will wait for that callback to
// return.
//
// NOTE: We expect the caller to handle graceful shutdown and signal handling.
func wrapCPUProfile(svrCtx *server.Context, callbackFn func() error) error {
	if cpuProfile := svrCtx.Viper.GetString(config.FlagCPUProfile); cpuProfile != "" {
		f, err := os.Create(cpuProfile)
		if err != nil {
			return err
		}

		svrCtx.Logger.Info("starting CPU profiler", "profile", cpuProfile)

		if err := pprof.StartCPUProfile(f); err != nil {
			return err
		}

		defer func() {
			svrCtx.Logger.Info("stopping CPU profiler", "profile", cpuProfile)
			pprof.StopCPUProfile()

			if err := f.Close(); err != nil {
				svrCtx.Logger.Info("failed to close cpu-profile file", "profile", cpuProfile, "err", err.Error())
			}
		}()
	}

	return callbackFn()
}
