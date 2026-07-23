package v1dot20dot2beta

import (
    "github.com/InjectiveLabs/injective-core/injective-chain/app/upgrades"
    storetypes "cosmossdk.io/store/types"
)

// 온체인 및 바이너리가 인식할 버전 상수
const UpgradeVersion = "v1.20.3-beta"

func StoreUpgrades() storetypes.StoreUpgrades {
    return storetypes.StoreUpgrades{
        Added:   nil,
        Renamed: nil,
        Deleted: nil,
    }
}

// 테스트를 위한 업그레이드 스텝 (빈 스텝 처리)
func UpgradeSteps() []*upgrades.UpgradeHandlerStep {
    return []*upgrades.UpgradeHandlerStep{}
}