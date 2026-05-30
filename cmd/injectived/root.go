package main

import (
	"context"
	"io"
	"math/big"
	"os"
	"path/filepath"

	"cosmossdk.io/log"
	"cosmossdk.io/math"
	"cosmossdk.io/store"
	"cosmossdk.io/store/snapshots"
	snapshottypes "cosmossdk.io/store/snapshots/types"
	storetypes "cosmossdk.io/store/types"
	confixcmd "cosmossdk.io/tools/confix/cmd"
	txsigning "cosmossdk.io/x/tx/signing"
	wasmcli "github.com/CosmWasm/wasmd/x/wasm/client/cli"
	tmcmd "github.com/cometbft/cometbft/cmd/cometbft/commands"
	cmcli "github.com/cometbft/cometbft/libs/cli"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/config"
	"github.com/cosmos/cosmos-sdk/client/debug"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/pruning"
	"github.com/cosmos/cosmos-sdk/client/rpc"
	"github.com/cosmos/cosmos-sdk/client/snapshot"
	"github.com/cosmos/cosmos-sdk/codec"
	sdkserver "github.com/cosmos/cosmos-sdk/server"
	servertypes "github.com/cosmos/cosmos-sdk/server/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	authcmd "github.com/cosmos/cosmos-sdk/x/auth/client/cli"
	"github.com/cosmos/cosmos-sdk/x/auth/tx"
	authtxconfig "github.com/cosmos/cosmos-sdk/x/auth/tx/config"
	"github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/cosmos/cosmos-sdk/x/genutil"
	genutilcli "github.com/cosmos/cosmos-sdk/x/genutil/client/cli"
	genutiltypes "github.com/cosmos/cosmos-sdk/x/genutil/types"
	"github.com/pkg/errors"
	"github.com/spf13/cast"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/InjectiveLabs/injective-core/injective-chain/app"
	"github.com/InjectiveLabs/injective-core/injective-chain/app/ante/eip712"
	"github.com/InjectiveLabs/injective-core/injective-chain/app/ante/typeddata"
	appconfig "github.com/InjectiveLabs/injective-core/injective-chain/app/config"
	clientcli "github.com/InjectiveLabs/injective-core/injective-chain/app/config/cli"
	chainclient "github.com/InjectiveLabs/injective-core/injective-chain/client"
	injcodectypes "github.com/InjectiveLabs/injective-core/injective-chain/codec/types"
	"github.com/InjectiveLabs/injective-core/injective-chain/crypto/hd"
	injectivekr "github.com/InjectiveLabs/injective-core/injective-chain/crypto/keyring"
	"github.com/InjectiveLabs/injective-core/version"
)

// NewRootCmd creates a new root command for simd. It is called once in the
// main function.
func NewRootCmd() *cobra.Command {
	// we "pre"-instantiate the application for getting the injected/configured encoding configuration
	// note, this is not necessary when using app wiring, as depinject can be directly used (see root_v2.go)
	cfg := appconfig.DefaultConfig()
	defaultHome := cfg.GetHome()
	cfg.Set(flags.FlagHome, tempDir())
	tempApp := app.NewInjectiveApp(log.NewNopLogger(), dbm.NewMemDB(), nil, true, *cfg)
	encodingConfig := injcodectypes.EncodingConfig{
		InterfaceRegistry: tempApp.InterfaceRegistry(),
		Codec:             tempApp.AppCodec(),
		TxConfig:          tempApp.TxConfig(),
		Amino:             tempApp.LegacyAmino(),
	}

	// set global cdc for typed data (Ledger signing)
	typeddata.SetCodec(encodingConfig.Amino, codec.NewProtoCodec(encodingConfig.InterfaceRegistry))

	initClientCtx := client.Context{}.
		WithCodec(encodingConfig.Codec).
		WithInterfaceRegistry(encodingConfig.InterfaceRegistry).
		WithTxConfig(encodingConfig.TxConfig).
		WithLegacyAmino(encodingConfig.Amino).
		WithInput(os.Stdin).
		WithKeyringOptions(hd.EthSecp256k1Option()).
		WithAccountRetriever(types.AccountRetriever{}).
		WithBroadcastMode(flags.BroadcastSync).
		WithHomeDir(defaultHome).
		WithViper("injectived").
		WithKeyringOptions(injectivekr.EthSecp256k1Option()).
		WithPreprocessTxHook(injectivekr.LedgerPreprocessTxHook)

	rootCmd := &cobra.Command{
		Use:           "injectived",
		Short:         "Injective Daemon",
		SilenceErrors: true,
		SilenceUsage:  true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			// set the default command outputs
			cmd.SetOut(cmd.OutOrStdout())
			cmd.SetErr(cmd.ErrOrStderr())

			initClientCtx = initClientCtx.WithCmdContext(cmd.Context())
			initClientCtx, err := client.ReadPersistentCommandFlags(initClientCtx, cmd.Flags())
			if err != nil {
				return err
			}

			initClientCtx, err = clientcli.ReadFromClientConfig(initClientCtx)
			if err != nil {
				return err
			}

			// This needs to go after ReadFromClientConfig, as that function
			// sets the RPC client needed for SIGN_MODE_TEXTUAL. This sign mode
			// is only available if the client is online.
			if !initClientCtx.Offline {
				txConfigOpts := tx.ConfigOptions{
					EnabledSignModes:           append(tx.DefaultSignModes, signing.SignMode_SIGN_MODE_TEXTUAL),
					CustomSignModes:            []txsigning.SignModeHandler{eip712.NewSignModeHandler(initClientCtx.Codec)},
					TextualCoinMetadataQueryFn: authtxconfig.NewGRPCCoinMetadataQueryFn(initClientCtx),
				}

				txConfig, err := tx.NewTxConfigWithOptions(initClientCtx.Codec, txConfigOpts)
				if err != nil {
					return err
				}

				initClientCtx = initClientCtx.WithTxConfig(txConfig)
			}

			if err := client.SetCmdClientContextHandler(initClientCtx, cmd); err != nil {
				return err
			}

			return InterceptConfigsPreRunHandler(cmd)
		},
	}

	initRootCmd(rootCmd, encodingConfig.TxConfig, tempApp.BasicModuleManager, encodingConfig)

	autoCliOpts := tempApp.AutoCliOpts()
	initClientCtx, _ = config.ReadDefaultValuesFromDefaultClientConfig(initClientCtx)
	autoCliOpts.ClientCtx = initClientCtx

	if err := autoCliOpts.EnhanceRootCommand(rootCmd); err != nil {
		panic(err)
	}

	return rootCmd
}

// Execute executes the root command.
func Execute(rootCmd *cobra.Command) error {
	// Create and set a client.Context on the command's Context. During the pre-run
	// of the root command, a default initialized client.Context is provided to
	// seed child command execution with values such as AccountRetriver, Keyring,
	// and a Tendermint RPC. This requires the use of a pointer reference when
	// getting and setting the client.Context. Ideally, we utilize
	// https://github.com/spf13/cobra/pull/1118.
	ctx := context.Background()
	ctx = context.WithValue(ctx, client.ClientContextKey, &client.Context{})
	ctx = context.WithValue(ctx, sdkserver.ServerContextKey, sdkserver.NewDefaultContext())

	appconfig.AddLogFlags(rootCmd)

	executor := cmcli.PrepareBaseCmd(rootCmd, "", appconfig.DefaultNodeHome)
	return executor.ExecuteContext(ctx)
}

func initRootCmd(
	rootCmd *cobra.Command,
	txConfig client.TxConfig,
	basicManager module.BasicManager,
	ec injcodectypes.EncodingConfig,
) {
	sdk.DefaultPowerReduction = math.NewIntFromBigInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))

	cfg := sdk.GetConfig()
	cfg.Seal()

	rootCmd.AddCommand(
		genutilcli.InitCmd(basicManager, appconfig.DefaultNodeHome),
		debug.Cmd(),
		confixcmd.ConfigCommand(),
		pruning.Cmd(newApp, appconfig.DefaultNodeHome),
		snapshot.Cmd(newApp),
		AddGenesisAccountCmd(appconfig.DefaultNodeHome),
	)

	cometCmd := &cobra.Command{
		Use:     "comet",
		Aliases: []string{"cometbft", "tendermint"},
		Short:   "CometBFT subcommands",
	}

	cometCmd.AddCommand(
		sdkserver.ShowNodeIDCmd(),
		sdkserver.ShowValidatorCmd(),
		sdkserver.ShowAddressCmd(),
		sdkserver.VersionCmd(),
		tmcmd.ResetAllCmd,
		tmcmd.ResetStateCmd,
		sdkserver.BootstrapStateCmd(newApp),
	)

	startCmd := StartCmd(newInjApp)

	rootCmd.AddCommand(
		startCmd,
		cometCmd,
		sdkserver.ExportCmd(appExport, appconfig.DefaultNodeHome),
		version.NewVersionCommand(),
		sdkserver.NewRollbackCmd(newApp, appconfig.DefaultNodeHome),
	)

	rootCmd.AddCommand(devnetifyCmd(app.NewDevnetApp))

	wasmcli.ExtendUnsafeResetAllCmd(rootCmd)

	// add keybase, auxiliary RPC, query, genesis, and tx child commands
	rootCmd.AddCommand(
		sdkserver.StatusCommand(),
		genesisCommand(txConfig, basicManager),
		queryCommand(),
		txCommand(),
		chainclient.KeyCommands(appconfig.DefaultNodeHome),
	)
}

// genesisCommand builds genesis-related `simd genesis` command. Users may provide application specific commands as a parameter
func genesisCommand(txConfig client.TxConfig, basicManager module.BasicManager, cmds ...*cobra.Command) *cobra.Command {
	cmd := &cobra.Command{
		Use:                        "genesis",
		Short:                      "Application's genesis-related subcommands",
		DisableFlagParsing:         false,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}
	gentxModule := basicManager[genutiltypes.ModuleName].(genutil.AppModuleBasic)

	cmd.AddCommand(
		genutilcli.InitCmd(basicManager, appconfig.DefaultNodeHome),
		genutilcli.CollectGenTxsCmd(banktypes.GenesisBalancesIterator{}, appconfig.DefaultNodeHome, gentxModule.GenTxValidator, txConfig.SigningContext().ValidatorAddressCodec()),
		genutilcli.MigrateGenesisCmd(genutilcli.MigrationMap),
		genutilcli.GenTxCmd(app.ModuleBasics, txConfig, banktypes.GenesisBalancesIterator{}, appconfig.DefaultNodeHome, txConfig.SigningContext().ValidatorAddressCodec()),
		genutilcli.ValidateGenesisCmd(app.ModuleBasics),
		AddGenesisAccountCmd(appconfig.DefaultNodeHome),
	)

	for _, subCmd := range cmds {
		cmd.AddCommand(subCmd)
	}

	return cmd
}

func queryCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        "query",
		Aliases:                    []string{"q"},
		Short:                      "Querying subcommands",
		DisableFlagParsing:         true,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}

	cmd.AddCommand(
		rpc.ValidatorCommand(),
		rpc.QueryEventForTxCmd(),
		sdkserver.QueryBlockCmd(),
		sdkserver.QueryBlocksCmd(),
		sdkserver.QueryBlockResultsCmd(),
		authcmd.QueryTxCmd(),
		authcmd.QueryTxsByEventsCmd(),
		authcmd.QueryTxsByEventsCmd(),
		authcmd.GetSimulateCmd(),
	)

	appconfig.AddChainIDFlag(cmd)

	return cmd
}

func txCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        "tx",
		Short:                      "Transactions subcommands",
		DisableFlagParsing:         true,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}

	cmd.AddCommand(
		authcmd.GetSignCommand(),
		authcmd.GetSignBatchCommand(),
		authcmd.GetMultiSignCommand(),
		authcmd.GetMultiSignBatchCmd(),
		authcmd.GetValidateSignaturesCommand(),
		flags.LineBreak,
		authcmd.GetBroadcastCommand(),
		authcmd.GetEncodeCommand(),
		authcmd.GetDecodeCommand(),
		authcmd.GetSimulateCmd(),
		flags.LineBreak,
	)

	appconfig.AddChainIDFlag(cmd)

	return cmd
}

// newApp is an AppCreator, here for compatibility with SDK commands
func newApp(logger log.Logger, db dbm.DB, traceStore io.Writer, appOpts servertypes.AppOptions) servertypes.Application {
	cfg, err := appconfig.GetConfig(appOpts.(*viper.Viper))
	if err != nil {
		panic(err)
	}
	return newInjApp(logger, db, traceStore, cfg)
}

// newInjApp is an InjAppCreator
//
//nolint:revive // cyclo complexity is OK
func newInjApp(logger log.Logger, db dbm.DB, traceStore io.Writer, cfg appconfig.Config) servertypes.Application {
	homeDir := cfg.GetHome()
	chainID := cfg.GetChainID()

	var cache storetypes.MultiStorePersistentCache

	if cfg.InterBlockCache {
		cache = store.NewCommitKVStoreCacheManager()
	}

	skipUpgradeHeights := make(map[int64]bool)
	for _, h := range cast.ToIntSlice(cfg.Get(sdkserver.FlagUnsafeSkipUpgrades)) {
		skipUpgradeHeights[int64(h)] = true
	}

	pruningOpts, err := cfg.GetPruningOptions()
	if err != nil {
		panic(err)
	}

	snapshotDir := filepath.Join(homeDir, "data", "snapshots")
	snapshotDB, err := dbm.NewDB("metadata", cfg.GetDBBackend(), snapshotDir)
	if err != nil {
		panic(err)
	}
	snapshotStore, err := snapshots.NewStore(snapshotDB, snapshotDir)
	if err != nil {
		panic(err)
	}

	snapshotOptions := snapshottypes.NewSnapshotOptions(cfg.StateSync.SnapshotInterval, cfg.StateSync.SnapshotKeepRecent)

	if chainID == "" || chainID == appconfig.DefaultChainID {
		// fallback to genesis chain-id
		appGenesis, err := genutiltypes.AppGenesisFromFile(filepath.Join(homeDir, "config", "genesis.json"))
		if err != nil {
			panic(err)
		}

		chainID = appGenesis.ChainID
	}

	baseAppOptions := []func(*baseapp.BaseApp){
		baseapp.SetPruning(pruningOpts),
		baseapp.SetMinGasPrices(cfg.MinGasPrices),
		baseapp.SetHaltHeight(cfg.HaltHeight),
		baseapp.SetHaltTime(cfg.HaltTime),
		baseapp.SetMinRetainBlocks(cfg.MinRetainBlocks),
		baseapp.SetInterBlockCache(cache),
		baseapp.SetTrace(cast.ToBool(cfg.Get(sdkserver.FlagTrace))),
		baseapp.SetIndexEvents(cfg.IndexEvents),
		baseapp.SetSnapshot(snapshotStore, snapshotOptions),
		baseapp.SetIAVLCacheSize(int(cfg.IAVLCacheSize)),
		baseapp.SetIAVLDisableFastNode(cfg.IAVLDisableFastNode),
		baseapp.SetIAVLSyncPruning(cast.ToBool(cfg.Get(sdkserver.FlagIAVLSyncPruning))),
		baseapp.SetChainID(chainID),
	}

	if option := cfg.Get(appconfig.FlagOptimisticExecutionEnabled); option != nil {
		if isEnabled, err := cast.ToBoolE(option); err == nil && isEnabled {
			logger.Info("Optimistic execution enabled", isEnabled)
			baseAppOptions = append(baseAppOptions, baseapp.SetOptimisticExecution())
		} else {
			logger.Info("Optimistic execution disabled")
		}
	}

	loadLatest := os.Getenv("COSMOS_SDK_ROLLBACK_SKIP_LOAD_LATEST") != "true"

	return app.NewInjectiveApp(
		logger, db, traceStore,
		loadLatest,
		cfg,
		baseAppOptions...,
	)
}

// appExport creates a new simapp (optionally at a given height)
// and exports state.
func appExport(
	logger log.Logger,
	db dbm.DB,
	traceStore io.Writer,
	height int64,
	forZeroHeight bool,
	jailAllowedAddrs []string,
	appOpts servertypes.AppOptions,
	modulesToExport []string,
) (servertypes.ExportedApp, error) {
	// this check is necessary as we use the flag in x/upgrade.
	// we can exit more gracefully by checking the flag here.
	var injectiveApp *app.InjectiveApp
	homePath, ok := appOpts.Get(flags.FlagHome).(string)
	if !ok || homePath == "" {
		return servertypes.ExportedApp{}, errors.New("application home not set")
	}

	viperAppOpts, ok := appOpts.(*viper.Viper)
	if !ok {
		return servertypes.ExportedApp{}, errors.New("appOpts is not viper.Viper")
	}

	// overwrite the FlagInvCheckPeriod
	viperAppOpts.Set(sdkserver.FlagInvCheckPeriod, 1)
	appOpts = viperAppOpts

	cfg, err := appconfig.GetConfig(viperAppOpts)
	if err != nil {
		return servertypes.ExportedApp{}, err
	}

	if height != -1 {
		injectiveApp = app.NewInjectiveApp(logger, db, traceStore, false, cfg)

		if err := injectiveApp.LoadHeight(height); err != nil {
			return servertypes.ExportedApp{}, err
		}
	} else {
		injectiveApp = app.NewInjectiveApp(logger, db, traceStore, true, cfg)
	}

	return injectiveApp.ExportAppStateAndValidators(forZeroHeight, jailAllowedAddrs, modulesToExport)
}

var tempDir = func() string {
	dir, err := os.MkdirTemp("", "injectiveapp")
	if err != nil {
		dir = appconfig.DefaultNodeHome
	}
	defer os.RemoveAll(dir)

	return dir
}
