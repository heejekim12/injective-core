package isolated

import (
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/risk"
)

func init() {
	risk.RegisterIsolatedModelFactory(func(_ *risk.Engine) risk.Model {
		return NewModel()
	})
}
