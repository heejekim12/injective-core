package derivative

import (
	"cosmossdk.io/errors"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	"github.com/ethereum/go-ethereum/common"

	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/keeper/base"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/keeper/feediscounts"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/keeper/rewards"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/keeper/subaccount"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/risk"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/types"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/types/v2"
)

//nolint:revive // ok
type DerivativeKeeper struct {
	*base.BaseKeeper

	subaccount        *subaccount.SubaccountKeeper
	insurance         types.InsuranceKeeper
	oracle            types.OracleKeeper
	feeDiscounts      *feediscounts.FeeDiscountsKeeper
	trading           *rewards.TradingKeeper
	bank              bankkeeper.Keeper
	wasm              types.WasmViewKeeper    // set after New
	permissionsKeeper types.PermissionsKeeper // set after New
	riskEngine        *risk.Engine
}

func New(
	b *base.BaseKeeper,
	re *risk.Engine,
	sa *subaccount.SubaccountKeeper,
	o types.OracleKeeper,
	fd *feediscounts.FeeDiscountsKeeper,
	bk bankkeeper.Keeper,
	i types.InsuranceKeeper,
	tw *rewards.TradingKeeper,
	pk types.PermissionsKeeper,
) *DerivativeKeeper {
	return &DerivativeKeeper{
		BaseKeeper:        b,
		subaccount:        sa,
		oracle:            o,
		feeDiscounts:      fd,
		bank:              bk,
		insurance:         i,
		trading:           tw,
		permissionsKeeper: pk,
		riskEngine:        re,
	}
}

// consequence of app init's chicken-or-egg problem
func (k DerivativeKeeper) SetWasm(ws types.WasmViewKeeper) *DerivativeKeeper {
	return &DerivativeKeeper{
		BaseKeeper:        k.BaseKeeper,
		subaccount:        k.subaccount,
		oracle:            k.oracle,
		feeDiscounts:      k.feeDiscounts,
		bank:              k.bank,
		insurance:         k.insurance,
		trading:           k.trading,
		wasm:              ws,
		permissionsKeeper: k.permissionsKeeper,
		riskEngine:        k.riskEngine,
	}
}

func (k *DerivativeKeeper) SetPermissionsKeeper(pk types.PermissionsKeeper) {
	k.permissionsKeeper = pk
}

// GetFeeDiscountConfigForMarket returns the fee discount configuration for a market.
// This is used by the FBA package to process derivative matching results.
func (k DerivativeKeeper) GetFeeDiscountConfigForMarket(ctx sdk.Context, marketID common.Hash, stakingInfo *v2.FeeDiscountStakingInfo) *v2.FeeDiscountConfig {
	return k.feeDiscounts.GetFeeDiscountConfigForMarket(ctx, marketID, stakingInfo)
}

func (k DerivativeKeeper) RiskEngine() risk.ReadOnlyEngine {
	return k.riskEngine
}

func (k DerivativeKeeper) TokenDenomDecimals(ctx sdk.Context, tokenDenom string) (decimals uint32, err error) {
	tokenMetadata, found := k.bank.GetDenomMetaData(ctx, tokenDenom)
	if !found {
		return 0, errors.Wrapf(types.ErrInvalidQuoteDenom, "denom %s does not have denom metadata", tokenDenom)
	}
	if tokenMetadata.Decimals == 0 {
		return 0, errors.Wrapf(types.ErrInvalidQuoteDenom, "denom units for %s are not correctly configured", tokenDenom)
	}

	return tokenMetadata.Decimals, nil
}

func (k DerivativeKeeper) SavePosition(
	ctx sdk.Context,
	marketID,
	subaccountID common.Hash,
	position *v2.Position,
) {
	defer k.Meter(ctx).FuncTiming(&ctx, "SavePosition")()

	k.RiskEngine().EvictCrossPoolSnapshotCache(ctx, subaccountID)

	k.SetTransientPosition(ctx, marketID, subaccountID, position)

	if position.Quantity.IsZero() {
		k.RemovePosition(ctx, marketID, subaccountID)
		return
	}

	k.SetPosition(ctx, marketID, subaccountID, position)
}

func (k DerivativeKeeper) RemovePosition(
	ctx sdk.Context,
	marketID,
	subaccountID common.Hash,
) {
	defer k.Meter(ctx).FuncTiming(&ctx, "RemovePosition")()

	k.InvalidateConditionalOrdersIfNoMarginLocked(ctx, marketID, subaccountID, true, nil, nil)
	k.DeletePosition(ctx, marketID, subaccountID)
}

func (k DerivativeKeeper) CalculateOpenInterestForMarket(ctx sdk.Context, marketID common.Hash) (math.LegacyDec, error) {
	defer k.Meter(ctx).FuncTiming(&ctx, "CalculateOpenInterestForMarket")()

	positions := k.GetAllPositionsByMarket(ctx, marketID)
	if len(positions) == 0 {
		return math.LegacyZeroDec(), nil
	}

	longOpenInterest := math.LegacyZeroDec()
	shortOpenInterest := math.LegacyZeroDec()

	for _, position := range positions {
		if position.Position.Quantity.IsNegative() {
			err := errors.Wrapf(
				sdkerrors.ErrLogic,
				"negative position quantity for market %s and subaccount %s",
				marketID.Hex(),
				position.SubaccountId,
			)
			return math.LegacyZeroDec(), err
		}

		if position.Position.IsLong {
			longOpenInterest.AddMut(position.Position.Quantity)
		} else {
			shortOpenInterest.AddMut(position.Position.Quantity)
		}
	}

	if !longOpenInterest.Equal(shortOpenInterest) {
		err := errors.Wrapf(
			sdkerrors.ErrLogic,
			"open interest mismatch for market %s: long %s, short %s",
			marketID.Hex(),
			longOpenInterest.String(),
			shortOpenInterest.String(),
		)
		return math.LegacyZeroDec(), err
	}

	openInterest := longOpenInterest.AddMut(shortOpenInterest)
	return openInterest, nil
}
