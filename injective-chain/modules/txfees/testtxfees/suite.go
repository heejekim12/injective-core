package testtxfees

import (
	"testing"
	"time"

	cmtproto "github.com/cometbft/cometbft/api/cometbft/types/v1"
	"github.com/cosmos/cosmos-sdk/client/flags"
	sdk "github.com/cosmos/cosmos-sdk/types"

	simapp "github.com/InjectiveLabs/injective-core/injective-chain/app"
	"github.com/InjectiveLabs/injective-core/injective-chain/app/config"
)

type TestSuite struct {
	Ctx sdk.Context
	App *simapp.InjectiveApp
}

func NewTestSuite(t *testing.T) TestSuite {
	t.Helper()

	cfg := config.DefaultConfig()
	cfg.Set(flags.FlagHome, t.TempDir())
	app := simapp.Setup(false, cfg)
	ctx := app.NewContextLegacy(false, cmtproto.Header{
		Height: 1,
		Time:   time.Now().UTC(),
	})

	t.Cleanup(func() {
		simapp.Cleanup(app)
	})

	txfeesParams := app.TxFeesKeeper.GetParams(ctx)
	txfeesParams.Mempool1559Enabled = true
	app.TxFeesKeeper.SetParams(ctx, txfeesParams)

	return TestSuite{
		Ctx: ctx,
		App: app,
	}
}
