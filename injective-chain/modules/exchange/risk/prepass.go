package risk

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
)

// PrepassResult captures the deterministic, read-only outputs of the risk pre-pass.
type PrepassResult struct {
	TouchedSubaccounts []common.Hash

	// CrossPoolSnapshots caches per-(subaccount, quoteDenom) cross-margin snapshots for this stage.
	// It is a best-effort cache scoped to the canonical touched set; matching-time logic must
	// fall back deterministically when a snapshot is missing.
	CrossPoolSnapshots map[common.Hash]map[string]*CrossPoolSnapshot
}

type prepassContextKey struct{}

var ctxKeyPrepass = prepassContextKey{}

func WithPrepassResult(ctx sdk.Context, result *PrepassResult) sdk.Context {
	return ctx.WithValue(ctxKeyPrepass, result)
}

func GetPrepassResult(ctx sdk.Context) (*PrepassResult, bool) {
	v := ctx.Value(ctxKeyPrepass)
	if v == nil {
		return nil, false
	}
	result, ok := v.(*PrepassResult)
	return result, ok
}
