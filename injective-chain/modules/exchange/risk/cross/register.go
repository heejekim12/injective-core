package cross

import (
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/risk"
)

func init() {
	risk.RegisterCrossModelFactory(func(e *risk.Engine) risk.Model {
		return NewModel(e)
	})
}
