package config

import (
	"fmt"
	"time"

	injwebsocket "github.com/InjectiveLabs/injective-core/injective-chain/websocket"
)

// WebsocketConfig defines configuration for the chainstream websocket server.
type WebsocketConfig struct {
	Address string `mapstructure:"address"`

	MaxOpenConnections  int           `mapstructure:"max-open-connections"`
	ReadTimeout         time.Duration `mapstructure:"read-timeout"`
	WriteTimeout        time.Duration `mapstructure:"write-timeout"`
	MaxBodyBytes        int64         `mapstructure:"max-body-bytes"`
	MaxHeaderBytes      int           `mapstructure:"max-header-bytes"`
	MaxRequestBatchSize int           `mapstructure:"max-request-batch-size"`
}

// DefaultWebsocketConfig returns the default websocket configuration.
func DefaultWebsocketConfig() *WebsocketConfig {
	rpcCfg := injwebsocket.DefaultRPCConfig()
	return &WebsocketConfig{
		Address:             "",
		MaxOpenConnections:  rpcCfg.MaxOpenConnections,
		ReadTimeout:         rpcCfg.ReadTimeout,
		WriteTimeout:        rpcCfg.WriteTimeout,
		MaxBodyBytes:        rpcCfg.MaxBodyBytes,
		MaxHeaderBytes:      rpcCfg.MaxHeaderBytes,
		MaxRequestBatchSize: rpcCfg.MaxRequestBatchSize,
	}
}

func (cfg WebsocketConfig) Validate() error {
	if cfg.Address == "" {
		return nil
	}

	if cfg.MaxOpenConnections < 0 {
		return fmt.Errorf("invalid websocket max open connections %d: please set a non-negative value", cfg.MaxOpenConnections)
	}
	if cfg.ReadTimeout < 0 {
		return fmt.Errorf("invalid websocket read timeout %s: please set a non-negative duration", cfg.ReadTimeout)
	}
	if cfg.WriteTimeout < 0 {
		return fmt.Errorf("invalid websocket write timeout %s: please set a non-negative duration", cfg.WriteTimeout)
	}
	if cfg.MaxBodyBytes < 0 {
		return fmt.Errorf("invalid websocket max body bytes %d: please set a non-negative value", cfg.MaxBodyBytes)
	}
	if cfg.MaxHeaderBytes < 0 {
		return fmt.Errorf("invalid websocket max header bytes %d: please set a non-negative value", cfg.MaxHeaderBytes)
	}
	if cfg.MaxRequestBatchSize < 0 {
		return fmt.Errorf("invalid websocket max request batch size %d: please set a non-negative value", cfg.MaxRequestBatchSize)
	}

	return nil
}
