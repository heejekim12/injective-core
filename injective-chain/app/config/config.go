package config

import (
	"fmt"

	pruningtypes "cosmossdk.io/store/pruning/types"
	"github.com/InjectiveLabs/metrics/v2"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/server"
	sdkconfig "github.com/cosmos/cosmos-sdk/server/config"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/mitchellh/mapstructure"
	"github.com/spf13/cast"
	"github.com/spf13/viper"

	chainstreamtypes "github.com/InjectiveLabs/injective-core/injective-chain/stream/types"
)

const (
	defaultMinGasPrices = "160000000inj"

	// DefaultAPIAddress defines the default address to bind the API server to.
	DefaultAPIAddress = "tcp://0.0.0.0:10337"

	// DefaultGRPCAddress is the default address the gRPC server binds to.
	DefaultGRPCAddress = "0.0.0.0:9900"
)

// Config defines the server's top level configuration
type Config struct {
	viper *viper.Viper // can be nil, a fallback for Get(key) method if key is not amongst struct field names

	sdkconfig.BaseConfig `mapstructure:",squash"`

	// Standard Cosmos SDK config

	API       sdkconfig.APIConfig       `mapstructure:"api"`
	GRPC      sdkconfig.GRPCConfig      `mapstructure:"grpc"`
	GRPCWeb   sdkconfig.GRPCWebConfig   `mapstructure:"grpc-web"`
	StateSync sdkconfig.StateSyncConfig `mapstructure:"state-sync"`
	Streaming sdkconfig.StreamingConfig `mapstructure:"streaming"`
	Mempool   sdkconfig.MempoolConfig   `mapstructure:"mempool"`

	// Injective-specific configs

	Metrics            metrics.Config          `mapstructure:"metrics"`
	JSONRPC            JSONRPCConfig           `mapstructure:"json-rpc"`
	JSONRPCDebug       JSONRPCConfig           `mapstructure:"json-rpc-debug"`
	EVM                EVMConfig               `mapstructure:"evm"`
	ChainStream        chainstreamtypes.Config `mapstructure:"chainstream"`
	InjectiveWebsocket WebsocketConfig         `mapstructure:"injective-websocket"`
}

// DefaultConfig returns server's default configuration.
func DefaultConfig() *Config {
	defaultConfig := sdkconfig.DefaultConfig()

	defaultConfig.MinGasPrices = defaultMinGasPrices
	defaultConfig.Pruning = pruningtypes.PruningOptionNothing
	defaultConfig.IAVLDisableFastNode = true

	defaultConfig.API.Enable = true
	defaultConfig.API.EnableUnsafeCORS = true
	defaultConfig.API.Swagger = true
	defaultConfig.API.Address = DefaultAPIAddress

	defaultConfig.GRPC.Address = DefaultGRPCAddress

	return &Config{
		viper: viper.New(),

		BaseConfig: defaultConfig.BaseConfig,

		Metrics:   metrics.DefaultConfig(),
		API:       defaultConfig.API,
		GRPC:      defaultConfig.GRPC,
		GRPCWeb:   defaultConfig.GRPCWeb,
		StateSync: defaultConfig.StateSync,
		Streaming: defaultConfig.Streaming,
		Mempool:   defaultConfig.Mempool,

		JSONRPC:            *DefaultJSONRPCConfig(),
		JSONRPCDebug:       *DefaultJSONRPCDebugConfig(),
		EVM:                *DefaultEVMConfig(),
		ChainStream:        chainstreamtypes.DefaultConfig(),
		InjectiveWebsocket: *DefaultWebsocketConfig(),
	}
}

// GetConfig returns a fully parsed Config object using Cosmos SDK defaults plus Injective-specific sections.
func GetConfig(v *viper.Viper) (Config, error) {
	cfg := *DefaultConfig()

	// this will override the values already presented in cfg with flag defaults, if flag value is not set.
	// Because of this flag defaults should match the defaults from DefaultConfig().
	if err := v.Unmarshal(&cfg); err != nil {
		return cfg, err
	}

	if err := cfg.Validate(); err != nil {
		return cfg, err
	}

	cfg.viper = v

	return cfg, nil
}

func (cfg Config) Validate() error {
	if cfg.MinGasPrices == "" {
		return sdkerrors.ErrAppConfig.Wrap("set min gas price in app.toml or flag or env variable")
	}
	if cfg.Pruning == pruningtypes.PruningOptionEverything && cfg.StateSync.SnapshotInterval > 0 {
		return sdkerrors.ErrAppConfig.Wrapf(
			"cannot enable state sync snapshots with '%s' pruning setting", pruningtypes.PruningOptionEverything,
		)
	}
	if err := cfg.EVM.Validate(); err != nil {
		return fmt.Errorf("invalid evm config: %w", err)
	}
	if err := cfg.JSONRPC.Validate(); err != nil {
		return fmt.Errorf("invalid json-rpc config: %w", err)
	}
	if err := cfg.JSONRPCDebug.Validate(); err != nil {
		return fmt.Errorf("invalid json-rpc-debug config: %w", err)
	}

	if err := cfg.ChainStream.Validate(); err != nil {
		return fmt.Errorf("invalid ChainStream config: %w", err)
	}

	if err := cfg.InjectiveWebsocket.Validate(); err != nil {
		return fmt.Errorf("invalid WebSocket config: %w", err)
	}

	return nil
}

// Get first tries to find Config.key field via reflection (using mapstructure), and then fallbacks into viper search.
// implements servertypes.AppOptions, so Config can be passed to an AppCreator.
func (cfg Config) Get(key string) any {
	var encodedMap map[string]any
	if err := mapstructure.Decode(cfg, &encodedMap); err != nil {
		panic(err)
	}
	if val, ok := encodedMap[key]; ok {
		return val
	}

	if cfg.viper != nil {
		return cfg.viper.Get(key)
	}

	return nil
}

func (cfg *Config) Set(key string, val any) {
	if cfg.viper != nil {
		cfg.viper.Set(key, val)
	}
}

func (cfg Config) GetHome() string {
	return cast.ToString(cfg.Get(flags.FlagHome))
}

func (cfg Config) GetChainID() string {
	return cast.ToString(cfg.Get(flags.FlagChainID))
}

func (cfg Config) GetDBBackend() dbm.BackendType {
	if cfg.AppDBBackend != "" {
		return dbm.BackendType(cfg.AppDBBackend)
	}
	return server.GetAppDBBackend(cfg)
}

func (cfg Config) GetPruningOptions() (pruningtypes.PruningOptions, error) {
	switch cfg.Pruning {
	case pruningtypes.PruningOptionDefault, pruningtypes.PruningOptionNothing, pruningtypes.PruningOptionEverything:
		return pruningtypes.NewPruningOptionsFromString(cfg.Pruning), nil

	case pruningtypes.PruningOptionCustom:
		opts := pruningtypes.NewCustomPruningOptions(
			cast.ToUint64(cfg.PruningKeepRecent),
			cast.ToUint64(cfg.PruningInterval),
		)

		if err := opts.Validate(); err != nil {
			return opts, fmt.Errorf("invalid custom pruning options: %w", err)
		}

		return opts, nil

	default:
		return pruningtypes.PruningOptions{}, fmt.Errorf("unknown pruning strategy %s", cfg.Pruning)
	}
}

// SetMinGasPrices sets the validator's minimum gas prices.
func (cfg *Config) SetMinGasPrices(gasPrices sdk.DecCoins) {
	cfg.MinGasPrices = gasPrices.String()
}

// GetMinGasPrices returns the validator's minimum gas prices based on the set configuration.
func (cfg Config) GetMinGasPrices() sdk.DecCoins {
	if cfg.MinGasPrices == "" {
		return sdk.DecCoins{}
	}

	gasPrices, err := sdk.ParseDecCoins(cfg.MinGasPrices)
	if err != nil {
		panic(fmt.Sprintf("invalid minimum gas prices: %v", err))
	}

	return gasPrices
}

// SDKConfig transforms Injective config to SDK config
func (cfg Config) SDKConfig() sdkconfig.Config {
	defaultSdkConfig := sdkconfig.DefaultConfig()

	return sdkconfig.Config{
		BaseConfig: cfg.BaseConfig,
		Telemetry:  defaultSdkConfig.Telemetry,
		API:        cfg.API,
		GRPC:       cfg.GRPC,
		GRPCWeb:    cfg.GRPCWeb,
		StateSync:  cfg.StateSync,
		Streaming:  cfg.Streaming,
		Mempool:    cfg.Mempool,
	}
}
