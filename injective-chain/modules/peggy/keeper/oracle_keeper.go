package keeper

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"

	oracletypes "github.com/InjectiveLabs/injective-core/injective-chain/modules/oracle/types"
)

// OracleKeeper is the read-only oracle dependency for peggy.
// Defined here (not in peggy/types) to avoid the import cycle:
// oracle/types -> peggy/types -> oracle/... -> oracle/types
type OracleKeeper interface {
	GetPythPriceState(ctx sdk.Context, priceID common.Hash) *oracletypes.PythPriceState
}
