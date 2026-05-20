package config

import (
	"io"

	"cosmossdk.io/log"
	dbm "github.com/cosmos/cosmos-db"
	servertypes "github.com/cosmos/cosmos-sdk/server/types"
)

type InjAppCreator func(log.Logger, dbm.DB, io.Writer, Config) servertypes.Application
