package main

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"cosmossdk.io/log"
	tmcfg "github.com/cometbft/cometbft/config"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/server"

	"github.com/InjectiveLabs/injective-core/injective-chain/app/config"
)

// InterceptConfigsPreRunHandler performs a pre-run function for the root daemon
// application command. It will create a Viper literal and a default server
// Context. The server Tendermint configuration will either be read and parsed
// or created and saved to disk, where the server Context is updated to reflect
// the Tendermint configuration. The Viper literal is used to read and parse
// the application configuration. Command handlers can fetch the server Context
// to get the Tendermint configuration or to get access to Viper.
func InterceptConfigsPreRunHandler(cmd *cobra.Command) error {
	serverCtx := server.NewDefaultContext()

	// Get the executable name and configure the viper instance so that environmental
	// variables are checked based off that name. The underscore character is used
	// as a separator
	executableName, err := os.Executable()
	if err != nil {
		return err
	}

	basename := path.Base(executableName)

	// Configure the viper instance
	// flags
	err = serverCtx.Viper.BindPFlags(cmd.Flags())
	if err != nil {
		return err
	}
	err = serverCtx.Viper.BindPFlags(cmd.PersistentFlags())
	if err != nil {
		return err
	}
	// env
	serverCtx.Viper.SetEnvPrefix(basename)
	serverCtx.Viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	serverCtx.Viper.AutomaticEnv()
	// configs
	err = interceptConfigs(serverCtx.Viper, serverCtx.Config)
	if err != nil {
		return err
	}

	serverCtx.Logger, err = createLogger(serverCtx.Viper, serverCtx.Config)
	if err != nil {
		return err
	}

	return server.SetCmdServerContext(cmd, serverCtx)
}

func createLogger(v *viper.Viper, cmtcfg *tmcfg.Config) (log.Logger, error) {
	logLevel := cmtcfg.LogLevel
	if v.IsSet(config.FlagLogLevel) { // for backwards compatibility we read cmd.Flag, if set
		logLevel = v.GetString(config.FlagLogLevel)
	}

	logFormat := cmtcfg.LogFormat
	if v.IsSet(config.FlagLogFormat) { // for backwards compatibility we read cmd.Flag, if set
		logFormat = v.GetString(config.FlagLogFormat)
	}

	logLevelFn, err := log.ParseLogLevel(logLevel)
	if err != nil {
		return nil, err
	}

	logOpts := []log.Option{log.FilterOption(logLevelFn)}

	if logFormat == flags.OutputFormatJSON {
		logOpts = append(logOpts, log.OutputJSONOption())
	}

	logOpts = append(logOpts,
		log.TraceOption(v.GetBool(server.FlagTrace)),
		log.ColorOption(!v.GetBool(config.FlagLogNoColor)),
	)

	return log.NewLogger(os.Stderr, logOpts...).With("module", "main"), nil
}

// interceptConfigs parses and updates a Tendermint configuration file or
// creates a new one and saves it. It also parses and saves the application
// configuration file. The Tendermint configuration file is parsed given a root
// Viper object, whereas the application is parsed with the private package-aware
// viperCfg object.
func interceptConfigs(rootViper *viper.Viper, conf *tmcfg.Config) error {
	rootDir := rootViper.GetString(flags.FlagHome)
	configPath := filepath.Join(rootDir, "config")
	tmCfgFile := filepath.Join(configPath, "config.toml")

	switch _, err := os.Stat(tmCfgFile); {
	case os.IsNotExist(err):
		tmcfg.EnsureRoot(rootDir)

		// overwrite CometBFT defaults with Injective defaults
		conf.RPC.PprofListenAddress = "localhost:6060"
		conf.Consensus.PeerGossipSleepDuration = 10 * time.Millisecond
		conf.P2P.MaxNumOutboundPeers = 40
		conf.Mempool.Size = 200

		if err = conf.ValidateBasic(); err != nil {
			return fmt.Errorf("error in config file: %w", err)
		}

		// write new conf
		tmcfg.WriteConfigFile(tmCfgFile, conf)

	case err != nil:
		return err

	default:
		rootViper.SetConfigType("toml")
		rootViper.SetConfigName("config")
		rootViper.AddConfigPath(configPath)

		if err := rootViper.ReadInConfig(); err != nil {
			return fmt.Errorf("failed to read in %s: %w", tmCfgFile, err)
		}
	}

	// Read into the configuration whatever data the viper instance has for it.
	// This may come from the configuration file above but also any of the other
	// sources viper uses.
	if err := rootViper.Unmarshal(conf); err != nil {
		return err
	}

	conf.SetRoot(rootDir)

	appCfgFilePath := filepath.Join(configPath, "app.toml")
	if _, err := os.Stat(appCfgFilePath); os.IsNotExist(err) {
		appConf, err := config.GetConfig(rootViper)
		if err != nil {
			return fmt.Errorf("failed to parse %s: %w", appCfgFilePath, err)
		}

		config.WriteConfigFile(appCfgFilePath, appConf)
	}

	rootViper.SetConfigType("toml")
	rootViper.SetConfigName("app")
	rootViper.AddConfigPath(configPath)

	if err := rootViper.MergeInConfig(); err != nil {
		return fmt.Errorf("failed to merge configuration: %w", err)
	}

	return nil
}
