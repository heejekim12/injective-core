package types //nolint:revive // types is good name

import (
	"errors"

	"github.com/spf13/cobra"
)

type Config struct {
	ServerAddress             string `mapstructure:"server"`
	ServerBufferCapacity      uint   `mapstructure:"buffer-cap"`
	PublisherBufferCapacity   uint   `mapstructure:"publisher-buffer-cap"`
	EnforceKeepalive          bool   `mapstructure:"enforce-keepalive"`
	MinClientPingInterval     uint64 `mapstructure:"min-client-ping-interval"`
	MaxConnectionIdle         uint64 `mapstructure:"max-connection-idle"`
	ServerPingInterval        uint64 `mapstructure:"server-ping-interval"`
	ServerPingResponseTimeout uint64 `mapstructure:"server-ping-response-timeout"`
}

const (
	FlagStreamServer                    = "chainstream.server"
	FlagStreamServerBufferCapacity      = "chainstream.buffer-cap"
	FlagStreamPublisherBufferCapacity   = "chainstream.publisher-buffer-cap"
	FlagStreamEnforceKeepalive          = "chainstream.enforce-keepalive"
	FlagStreamMinClientPingInterval     = "chainstream.min-client-ping-interval"
	FlagStreamMaxConnectionIdle         = "chainstream.max-connection-idle"
	FlagStreamServerPingInterval        = "chainstream.server-ping-interval"
	FlagStreamServerPingResponseTimeout = "chainstream.server-ping-response-timeout"
)

func DefaultConfig() Config {
	return Config{
		ServerAddress:             "",
		ServerBufferCapacity:      100,
		PublisherBufferCapacity:   100,
		EnforceKeepalive:          false,
		MinClientPingInterval:     30,
		MaxConnectionIdle:         180,
		ServerPingInterval:        60,
		ServerPingResponseTimeout: 40,
	}
}

func (cfg Config) Validate() error {
	if cfg.ServerAddress == "" {
		return nil
	}

	if cfg.ServerBufferCapacity == 0 {
		return errors.New("invalid stream buffer capacity: must be greater than 0")
	}

	if cfg.PublisherBufferCapacity == 0 {
		return errors.New("invalid publisher buffer capacity: must be greater than 0")
	}

	return nil
}

var defaultCfg = DefaultConfig()

func AddCmdFlags(cmd *cobra.Command) {
	cmd.Flags().String(FlagStreamServer, defaultCfg.ServerAddress, "ChainStream server address to bind to.")
	cmd.Flags().Uint(FlagStreamServerBufferCapacity, defaultCfg.ServerBufferCapacity,
		"Configure ChainStream server buffer capacity for each connected client",
	)
	cmd.Flags().Uint(FlagStreamPublisherBufferCapacity, defaultCfg.PublisherBufferCapacity, "Configure ChainStream publisher buffer capacity")
	cmd.Flags().Bool(FlagStreamEnforceKeepalive, defaultCfg.EnforceKeepalive,
		"Define if Keepalive configuration params should be applied to chainstream gRPC server",
	)
	cmd.Flags().Uint64(FlagStreamMinClientPingInterval, defaultCfg.MinClientPingInterval,
		"Amount of time (in seconds) a client should wait before sending a keepalive ping",
	)
	cmd.Flags().Uint64(FlagStreamMaxConnectionIdle, defaultCfg.MaxConnectionIdle,
		"Amount of time in seconds a connection is allowed to stay idle before forcing the disconnection",
	)
	cmd.Flags().Uint64(FlagStreamServerPingInterval, defaultCfg.ServerPingInterval,
		"Amount of time in seconds after which the server will send a keepalive ping to the client on an idle connection",
	)
	cmd.Flags().Uint64(FlagStreamServerPingResponseTimeout, defaultCfg.ServerPingResponseTimeout,
		"Amount of time in seconds the server waits for the client to respond to a ping message before forcing a disconnection",
	)
}
