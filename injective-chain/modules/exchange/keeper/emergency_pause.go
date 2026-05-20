package keeper

import (
	storetypes "cosmossdk.io/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"

	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/keeper/events"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/types/v2"
)

// MaxEmergencyPauseCancelGas caps the synthetic gas charge for the emergency-pause cancellation
// sweep. Without a cap, the governance proposal could exceed tx/block gas on large books,
// making emergency pause unactivatable when it's needed most.
const MaxEmergencyPauseCancelGas = storetypes.Gas(50_000_000)

// CancelAllCrossMarginOrdersOnEmergencyPause cancels all derivative and spot orders for every
// cross-margin subaccount. Must be called whenever EmergencyPaused transitions from false to true.
// MsgUpdateParams calls this automatically; upgrade handlers or genesis init that set
// EmergencyPaused=true must call this explicitly.
func (k *Keeper) CancelAllCrossMarginOrdersOnEmergencyPause(ctx sdk.Context) {
	defer k.Meter(ctx).FuncTiming(&ctx, "CancelAllCrossMarginOrdersOnEmergencyPause")()

	profiles := k.GetAllSubaccountRiskProfiles(ctx)

	// Use infinite gas meter to prevent OOG during the sweep, then charge synthetic gas
	// back to the real meter (capped to ensure the governance proposal always succeeds).
	realMeter := ctx.GasMeter()
	infiniteCtx := ctx.WithGasMeter(storetypes.NewInfiniteGasMeter())
	var derivCancelled, spotCancelled, spotMarketsScanned uint64

	for _, record := range profiles {
		if record.RiskProfile.Mode != v2.RiskMode_RISK_MODE_CROSS {
			continue
		}

		subaccountID := common.HexToHash(record.SubaccountId)

		// Cancel ALL derivative orders across all markets (no denom filter).
		marketIDs := k.GetAllActiveDerivativeMarketIDsForSubaccount(infiniteCtx, subaccountID)
		for _, marketID := range marketIDs {
			market := k.GetDerivativeMarketByID(infiniteCtx, marketID)
			if market == nil || market.GetMarketType().IsBinaryOptions() {
				continue
			}

			for _, isBuy := range []bool{true, false} {
				derivCancelled += uint64(len(k.GetAllRestingDerivativeLimitOrderHashesBySubaccountAndMarket(infiniteCtx, marketID, isBuy, subaccountID)))
				derivCancelled += uint64(len(k.GetAllTransientDerivativeLimitOrdersByMarketDirectionBySubaccountID(infiniteCtx, marketID, &subaccountID, isBuy)))
				derivCancelled += uint64(len(k.GetAllConditionalOrderHashesBySubaccountAndMarket(infiniteCtx, marketID, isBuy, true, subaccountID)))
				derivCancelled += uint64(len(k.GetAllConditionalOrderHashesBySubaccountAndMarket(infiniteCtx, marketID, isBuy, false, subaccountID)))
				if k.HasTransientDerivativeMarketOrderForSubaccount(infiniteCtx, marketID, subaccountID, isBuy) {
					derivCancelled++
				}
			}

			k.CancelAllTransientDerivativeLimitOrdersBySubaccountID(infiniteCtx, market, subaccountID)
			k.CancelAllRestingDerivativeLimitOrdersForSubaccount(infiniteCtx, market, subaccountID, true, true)
			k.CancelAllDerivativeMarketOrdersBySubaccountID(infiniteCtx, market, subaccountID, marketID)
			k.CancelAllConditionalDerivativeOrdersBySubaccountIDAndMarket(infiniteCtx, market, subaccountID)
		}

		// Cancel ALL spot orders across all markets (no denom filter).
		k.IterateSpotMarkets(infiniteCtx, nil, func(market *v2.SpotMarket) (stop bool) {
			spotMarketsScanned++
			marketID := market.MarketID()
			for _, isBuy := range []bool{true, false} {
				spotCancelled += uint64(len(k.GetAllSpotLimitOrdersBySubaccountAndMarket(infiniteCtx, marketID, isBuy, subaccountID)))
				spotCancelled += uint64(len(k.GetAllTransientSpotLimitOrdersBySubaccountAndMarket(infiniteCtx, marketID, isBuy, subaccountID)))
				spotCancelled += uint64(len(k.GetAllSubaccountSpotMarketOrdersByMarketDirection(infiniteCtx, marketID, subaccountID, isBuy)))
			}
			k.cancelSpotOrdersForSideByKeeper(infiniteCtx, market, marketID, subaccountID, true)
			k.cancelSpotOrdersForSideByKeeper(infiniteCtx, market, marketID, subaccountID, false)
			return false
		})
	}

	// Charge synthetic gas capped to MaxEmergencyPauseCancelGas so the proposal always succeeds.
	cancelGas := MsgCancelDerivativeOrderGas*derivCancelled +
		MsgCancelSpotOrderGas*spotCancelled +
		SpotMarketScanGas*spotMarketsScanned
	if cancelGas > MaxEmergencyPauseCancelGas {
		cancelGas = MaxEmergencyPauseCancelGas
	}
	realMeter.ConsumeGas(cancelGas, "emergency-pause cross-margin order cancellations")
}

func (k *Keeper) cancelSpotOrdersForSideByKeeper(
	ctx sdk.Context,
	market *v2.SpotMarket,
	marketID, subaccountID common.Hash,
	isBuy bool,
) {
	for _, order := range k.GetAllSpotLimitOrdersBySubaccountAndMarket(ctx, marketID, isBuy, subaccountID) {
		if err := k.CancelSpotLimitOrder(ctx, market, marketID, subaccountID, isBuy, order); err != nil {
			events.Emit(ctx, k.BaseKeeper, v2.NewEventOrderCancelFail(marketID, subaccountID, order.Hash().Hex(), order.Cid(), err))
		}
	}

	for _, order := range k.GetAllTransientSpotLimitOrdersBySubaccountAndMarket(ctx, marketID, isBuy, subaccountID) {
		if err := k.CancelTransientSpotLimitOrder(ctx, market, marketID, subaccountID, order); err != nil {
			events.Emit(ctx, k.BaseKeeper, v2.NewEventOrderCancelFail(marketID, subaccountID, order.Hash().Hex(), order.Cid(), err))
		}
	}

	for _, order := range k.GetAllSubaccountSpotMarketOrdersByMarketDirection(ctx, marketID, subaccountID, isBuy) {
		k.CancelTransientSpotMarketOrder(ctx, market, marketID, order)
	}
}
