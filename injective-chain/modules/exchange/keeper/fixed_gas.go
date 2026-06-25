package keeper

import (
	"context"
	"fmt"

	"cosmossdk.io/math"
	storetypes "cosmossdk.io/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"

	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/types"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/types/v2"
)

var (
	MsgCreateDerivativeLimitOrderGas         = storetypes.Gas(210_000)
	MsgCreateDerivativeLimitPostOnlyOrderGas = storetypes.Gas(250_000)
	MsgCreateDerivativeMarketOrderGas        = storetypes.Gas(190_000)
	MsgCancelDerivativeOrderGas              = storetypes.Gas(80_000)

	MsgCreateSpotLimitOrderGas         = storetypes.Gas(180_000)
	MsgCreateSpotLimitPostOnlyOrderGas = storetypes.Gas(210_000)
	MsgCreateSpotMarketOrderGas        = storetypes.Gas(180_000)
	MsgCancelSpotOrderGas              = storetypes.Gas(65_000)

	GTBOrdersGasMultiplier = math.LegacyMustNewDecFromStr("1.1")

	// NOTE: binary option orders are handled identically as derivative orders
	MsgCreateBinaryOptionsLimitOrderGas         = MsgCreateDerivativeLimitOrderGas
	MsgCreateBinaryOptionsLimitPostOnlyOrderGas = MsgCreateDerivativeLimitPostOnlyOrderGas
	MsgCreateBinaryOptionsMarketOrderGas        = MsgCreateDerivativeMarketOrderGas
	MsgCancelBinaryOptionsOrderGas              = MsgCancelDerivativeOrderGas

	MsgDepositGas = storetypes.Gas(210_000)
	// Withdraw, transfer, and margin messages now run cross-margin maintenance/admission checks
	// that build pool snapshots (iterating active markets and orders). Keep these conservatively
	// priced to avoid underpricing heavy execution in fixed-gas mode.
	MsgWithdrawGas = storetypes.Gas(200_000)
	// Risk profile switching (isolated→cross) iterates all derivative orders to release per-order holds.
	// Base gas covers the fixed overhead; per-order gas scales with actual order count.
	// Gas is determined inside the msg server (like MsgBatchCancel*), not via DetermineGas.
	MsgUpdateSubaccountRiskProfileGas = storetypes.Gas(125_000)
	PerOrderHoldReleaseGas            = storetypes.Gas(5_000)
	MsgSubaccountTransferGas          = storetypes.Gas(100_000)
	MsgExternalTransferGas            = storetypes.Gas(180_000)
	MsgIncreasePositionMarginGas          = storetypes.Gas(65_000)
	MsgDecreasePositionMarginGas          = storetypes.Gas(80_000)
	MsgLiquidateCrossMarginPoolGas        = storetypes.Gas(500_000)

	// SpotMarketScanGas is the per-market gas cost for the global spot market scan during
	// cross-pool liquidation cancel-first. cancelAllSpotOrdersLockingDenom iterates all spot
	// markets (no per-subaccount index exists for spot orders), so this charges for the
	// iteration itself, not just for orders actually cancelled.
	SpotMarketScanGas = storetypes.Gas(1_000)

	// CrossPoolPerPositionCloseGas is the per-position gas charge for closePoolPositions
	// during cross-pool liquidation. Each position close involves market order creation,
	// matching via ExecuteDerivativeMarketOrderImmediately, and balance updates.
	CrossPoolPerPositionCloseGas = storetypes.Gas(200_000)
)

//nolint:revive //this is fine
func DetermineGas(msg sdk.Msg) uint64 {
	switch msg := msg.(type) {
	case *v2.MsgCreateSpotLimitOrder:
		requiredGas := MsgCreateSpotLimitOrderGas
		if msg.Order.OrderType.IsPostOnly() {
			requiredGas = MsgCreateSpotLimitPostOnlyOrderGas
		}
		if msg.Order.ExpirationBlock > 0 {
			requiredGas = storetypes.Gas(GTBOrdersGasMultiplier.Mul(math.LegacyNewDec(int64(requiredGas))).TruncateInt64())
		}
		return requiredGas
	case *v2.MsgCreateSpotMarketOrder:
		return MsgCreateSpotMarketOrderGas
	case *v2.MsgCancelSpotOrder:
		return MsgCancelSpotOrderGas
	case *v2.MsgBatchCreateSpotLimitOrders:
		sum := uint64(0)
		for _, order := range msg.Orders {
			requiredGas := MsgCreateSpotLimitOrderGas
			if order.OrderType.IsPostOnly() {
				requiredGas = MsgCreateSpotLimitPostOnlyOrderGas
			}
			if order.ExpirationBlock > 0 {
				requiredGas = storetypes.Gas(GTBOrdersGasMultiplier.Mul(math.LegacyNewDec(int64(requiredGas))).TruncateInt64())
			}
			sum += requiredGas
		}

		return sum
	case *v2.MsgBatchCancelSpotOrders:
		panic("developer error: MsgBatchCancelSpotOrders gas already determined in msg server impl")
	case *v2.MsgCreateDerivativeLimitOrder:
		requiredGas := MsgCreateDerivativeLimitOrderGas
		if msg.Order.OrderType.IsPostOnly() {
			requiredGas = MsgCreateDerivativeLimitPostOnlyOrderGas
		}
		if msg.Order.ExpirationBlock > 0 {
			requiredGas = storetypes.Gas(GTBOrdersGasMultiplier.Mul(math.LegacyNewDec(int64(requiredGas))).TruncateInt64())
		}
		return requiredGas
	case *v2.MsgCreateDerivativeMarketOrder:
		return MsgCreateDerivativeMarketOrderGas
	case *v2.MsgCancelDerivativeOrder:
		return MsgCancelDerivativeOrderGas
	case *v2.MsgBatchCreateDerivativeLimitOrders:
		sum := uint64(0)
		for _, order := range msg.Orders {
			requiredGas := MsgCreateDerivativeLimitOrderGas
			if order.OrderType.IsPostOnly() {
				requiredGas = MsgCreateDerivativeLimitPostOnlyOrderGas
			}
			if order.ExpirationBlock > 0 {
				requiredGas = storetypes.Gas(GTBOrdersGasMultiplier.Mul(math.LegacyNewDec(int64(requiredGas))).TruncateInt64())
			}
			sum += requiredGas
		}

		return sum
	case *v2.MsgBatchCancelDerivativeOrders:
		panic("developer error: MsgBatchCancelDerivativeOrders gas already determined in msg server impl")
	case *v2.MsgCreateBinaryOptionsLimitOrder:
		requiredGas := MsgCreateBinaryOptionsLimitOrderGas
		if msg.Order.OrderType.IsPostOnly() {
			requiredGas = MsgCreateBinaryOptionsLimitPostOnlyOrderGas
		}
		if msg.Order.ExpirationBlock > 0 {
			requiredGas = storetypes.Gas(GTBOrdersGasMultiplier.Mul(math.LegacyNewDec(int64(requiredGas))).TruncateInt64())
		}
		return requiredGas
	case *v2.MsgCreateBinaryOptionsMarketOrder:
		return MsgCreateBinaryOptionsMarketOrderGas
	case *v2.MsgCancelBinaryOptionsOrder:
		return MsgCancelBinaryOptionsOrderGas
	case *v2.MsgBatchCancelBinaryOptionsOrders:
		panic("developer error: MsgBatchCancelBinaryOptionsOrders gas already determined in msg server impl")
	//	MISCELLANEOUS //
	case *v2.MsgDeposit:
		return MsgDepositGas
	case *v2.MsgWithdraw:
		return MsgWithdrawGas
	case *v2.MsgUpdateSubaccountRiskProfile:
		panic("developer error: MsgUpdateSubaccountRiskProfile gas already determined in msg server impl")
	case *v2.MsgSubaccountTransfer:
		return MsgSubaccountTransferGas
	case *v2.MsgExternalTransfer:
		return MsgExternalTransferGas
	case *v2.MsgIncreasePositionMargin:
		return MsgIncreasePositionMarginGas
	case *v2.MsgDecreasePositionMargin:
		return MsgDecreasePositionMarginGas
	case *v2.MsgLiquidateCrossMarginPool:
		return MsgLiquidateCrossMarginPoolGas
	default:
		panic(fmt.Sprintf("developer error: unknown message type: %T", msg))
	}
}

//nolint:revive // this is fine
func (k *Keeper) FixedGasBatchUpdateOrders(
	c context.Context,
	msg *v2.MsgBatchUpdateOrders,
) (*v2.MsgBatchUpdateOrdersResponse, error) {
	//	no clever method shadowing here
	ctx := sdk.UnwrapSDKContext(c)
	defer k.Meter(ctx).FuncTiming(&ctx, "FixedGasBatchUpdateOrders")()

	sender, _ := sdk.AccAddressFromBech32(msg.Sender)

	subaccountId := msg.SubaccountId

	// reference the gas meter early to consume gas later on in loop iterations
	gasMeter := ctx.GasMeter()
	gasConsumedBefore := gasMeter.GasConsumed()

	defer func() {
		totalGas := gasMeter.GasConsumed()
		k.Logger(ctx).Info("MsgBatchUpdateOrders",
			"gas_ante", gasConsumedBefore,
			"gas_msg", totalGas-gasConsumedBefore,
			"gas_total", totalGas,
			"sender", msg.Sender,
		)
	}()

	ctx = ctx.WithGasMeter(storetypes.NewInfiniteGasMeter())

	/**	1. Cancel all **/
	// NOTE: provided subaccountID indicates cancelling all orders in a market for given market IDs
	if isCancelAll := subaccountId != ""; isCancelAll {
		//  Derive the subaccountID.
		subaccountID := types.MustGetSubaccountIDOrDeriveFromNonce(sender, subaccountId)

		/**	1. a) Cancel all spot limit orders in markets **/
		for _, spotMarketIdToCancelAll := range msg.SpotMarketIdsToCancelAll {
			marketID := common.HexToHash(spotMarketIdToCancelAll)
			market := k.GetSpotMarketByID(ctx, marketID)
			if market == nil {
				continue
			}

			if !market.StatusSupportsOrderCancellations() {
				k.Logger(ctx).Debug("failed to cancel all spot limit orders", "marketID", marketID.Hex())
				continue
			}

			// k.CancelAllSpotLimitOrders(ctx, market, subaccountID, marketID)
			// get all orders to cancel
			var (
				restingBuyOrders = k.GetAllSpotLimitOrdersBySubaccountAndMarket(
					ctx,
					marketID,
					true,
					subaccountID,
				)
				restingSellOrders = k.GetAllSpotLimitOrdersBySubaccountAndMarket(
					ctx,
					marketID,
					false,
					subaccountID,
				)
				transientBuyOrders = k.GetAllTransientSpotLimitOrdersBySubaccountAndMarket(
					ctx,
					marketID,
					true,
					subaccountID,
				)
				transientSellOrders = k.GetAllTransientSpotLimitOrdersBySubaccountAndMarket(
					ctx,
					marketID,
					false,
					subaccountID,
				)
			)

			// consume gas
			gasMeter.ConsumeGas(MsgCancelSpotOrderGas*uint64(len(restingBuyOrders)), "")
			gasMeter.ConsumeGas(MsgCancelSpotOrderGas*uint64(len(restingSellOrders)), "")
			gasMeter.ConsumeGas(MsgCancelSpotOrderGas*uint64(len(transientBuyOrders)), "")
			gasMeter.ConsumeGas(MsgCancelSpotOrderGas*uint64(len(transientSellOrders)), "")
		}

		/**	1. b) Cancel all derivative limit orders in markets **/
		for _, derivativeMarketIdToCancelAll := range msg.DerivativeMarketIdsToCancelAll {
			marketID := common.HexToHash(derivativeMarketIdToCancelAll)
			market := k.GetDerivativeMarketByID(ctx, marketID)
			if market == nil {
				k.Logger(ctx).Debug(
					"failed to cancel all derivative limit orders for non-existent market",
					"marketID",
					marketID.Hex(),
				)
				continue
			}

			if !market.StatusSupportsOrderCancellations() {
				k.Logger(ctx).Debug(
					"failed to cancel all derivative limit orders for market whose status doesnt support cancellations",
					"marketID",
					marketID.Hex(),
				)
				continue
			}

			var (
				restingBuyOrderHashes = k.GetAllRestingDerivativeLimitOrderHashesBySubaccountAndMarket(
					ctx,
					marketID,
					true,
					subaccountID,
				)
				restingSellOrderHashes = k.GetAllRestingDerivativeLimitOrderHashesBySubaccountAndMarket(
					ctx,
					marketID,
					false, subaccountID,
				)
				buyOrders = k.GetAllTransientDerivativeLimitOrdersByMarketDirectionBySubaccountID(
					ctx,
					marketID,
					&subaccountID,
					true,
				)
				sellOrders = k.GetAllTransientDerivativeLimitOrdersByMarketDirectionBySubaccountID(
					ctx,
					marketID,
					&subaccountID,
					false,
				)
				higherMarketOrders = k.GetAllConditionalOrderHashesBySubaccountAndMarket(
					ctx,
					marketID,
					true,
					true,
					subaccountID,
				)
				lowerMarketOrders = k.GetAllConditionalOrderHashesBySubaccountAndMarket(
					ctx,
					marketID,
					false,
					true,
					subaccountID,
				)
				higherLimitOrders = k.GetAllConditionalOrderHashesBySubaccountAndMarket(
					ctx,
					marketID,
					true,
					false,
					subaccountID,
				)
				lowerLimitOrders = k.GetAllConditionalOrderHashesBySubaccountAndMarket(
					ctx,
					marketID,
					false,
					false,
					subaccountID,
				)
			)

			// consume gas
			gasMeter.ConsumeGas(MsgCancelDerivativeOrderGas*uint64(len(restingBuyOrderHashes)), "")
			gasMeter.ConsumeGas(MsgCancelDerivativeOrderGas*uint64(len(restingSellOrderHashes)), "")
			gasMeter.ConsumeGas(MsgCancelDerivativeOrderGas*uint64(len(buyOrders)), "")
			gasMeter.ConsumeGas(MsgCancelDerivativeOrderGas*uint64(len(sellOrders)), "")
			gasMeter.ConsumeGas(MsgCancelDerivativeOrderGas*uint64(len(higherMarketOrders)), "")
			gasMeter.ConsumeGas(MsgCancelDerivativeOrderGas*uint64(len(lowerMarketOrders)), "")
			gasMeter.ConsumeGas(MsgCancelDerivativeOrderGas*uint64(len(higherLimitOrders)), "")
			gasMeter.ConsumeGas(MsgCancelDerivativeOrderGas*uint64(len(lowerLimitOrders)), "")
		}

		/**	1. c) Cancel all bo limit orders in markets **/
		for _, binaryOptionsMarketIdToCancelAll := range msg.BinaryOptionsMarketIdsToCancelAll {
			marketID := common.HexToHash(binaryOptionsMarketIdToCancelAll)
			market := k.GetBinaryOptionsMarketByID(ctx, marketID)
			if market == nil {
				k.Logger(ctx).Debug(
					"failed to cancel all binary options limit orders for non-existent market",
					"marketID",
					marketID.Hex(),
				)
				continue
			}

			if !market.StatusSupportsOrderCancellations() {
				k.Logger(ctx).Debug(
					"failed to cancel all binary options limit orders for market whose status doesnt support cancellations",
					"marketID",
					marketID.Hex(),
				)
				continue
			}

			var (
				restingBuyOrderHashes = k.GetAllRestingDerivativeLimitOrderHashesBySubaccountAndMarket(
					ctx,
					marketID,
					true,
					subaccountID,
				)
				restingSellOrderHashes = k.GetAllRestingDerivativeLimitOrderHashesBySubaccountAndMarket(
					ctx,
					marketID,
					false,
					subaccountID,
				)
				buyOrders = k.GetAllTransientDerivativeLimitOrdersByMarketDirectionBySubaccountID(
					ctx,
					marketID,
					&subaccountID,
					true,
				)
				sellOrders = k.GetAllTransientDerivativeLimitOrdersByMarketDirectionBySubaccountID(
					ctx,
					marketID,
					&subaccountID,
					false,
				)
				higherMarketOrders = k.GetAllConditionalOrderHashesBySubaccountAndMarket(
					ctx,
					marketID,
					true,
					true,
					subaccountID,
				)
				lowerMarketOrders = k.GetAllConditionalOrderHashesBySubaccountAndMarket(
					ctx,
					marketID,
					false,
					true,
					subaccountID,
				)
				higherLimitOrders = k.GetAllConditionalOrderHashesBySubaccountAndMarket(
					ctx,
					marketID,
					true,
					false,
					subaccountID,
				)
				lowerLimitOrders = k.GetAllConditionalOrderHashesBySubaccountAndMarket(
					ctx,
					marketID,
					false,
					false,
					subaccountID,
				)
			)

			gasMeter.ConsumeGas(MsgCancelBinaryOptionsOrderGas*uint64(len(restingBuyOrderHashes)), "")
			gasMeter.ConsumeGas(MsgCancelBinaryOptionsOrderGas*uint64(len(restingSellOrderHashes)), "")
			gasMeter.ConsumeGas(MsgCancelBinaryOptionsOrderGas*uint64(len(buyOrders)), "")
			gasMeter.ConsumeGas(MsgCancelBinaryOptionsOrderGas*uint64(len(sellOrders)), "")
			gasMeter.ConsumeGas(MsgCancelBinaryOptionsOrderGas*uint64(len(higherMarketOrders)), "")
			gasMeter.ConsumeGas(MsgCancelBinaryOptionsOrderGas*uint64(len(lowerMarketOrders)), "")
			gasMeter.ConsumeGas(MsgCancelBinaryOptionsOrderGas*uint64(len(higherLimitOrders)), "")
			gasMeter.ConsumeGas(MsgCancelBinaryOptionsOrderGas*uint64(len(lowerLimitOrders)), "")
		}
	}

	gasMeter.ConsumeGas(MsgCancelSpotOrderGas*uint64(len(msg.SpotOrdersToCancel)), "")
	gasMeter.ConsumeGas(MsgCancelDerivativeOrderGas*uint64(len(msg.DerivativeOrdersToCancel)), "")
	gasMeter.ConsumeGas(MsgCancelBinaryOptionsOrderGas*uint64(len(msg.BinaryOptionsOrdersToCancel)), "")

	gasMeter.ConsumeGas(MsgCreateSpotMarketOrderGas*uint64(len(msg.SpotMarketOrdersToCreate)), "")
	gasMeter.ConsumeGas(MsgCreateDerivativeMarketOrderGas*uint64(len(msg.DerivativeMarketOrdersToCreate)), "")
	gasMeter.ConsumeGas(MsgCreateBinaryOptionsMarketOrderGas*uint64(len(msg.BinaryOptionsMarketOrdersToCreate)), "")

	// For limit orders creation we determine the gas using the DetermineGas function to reuse the logic
	for _, spotOrder := range msg.SpotOrdersToCreate {
		dummySpotOrderMessage := v2.MsgCreateSpotLimitOrder{
			Order: *spotOrder,
		}
		gasMeter.ConsumeGas(DetermineGas(&dummySpotOrderMessage), "")
	}
	for _, derivativeOrder := range msg.DerivativeOrdersToCreate {
		dummyDerivativeOrderMessage := v2.MsgCreateDerivativeLimitOrder{
			Order: *derivativeOrder,
		}
		gasMeter.ConsumeGas(DetermineGas(&dummyDerivativeOrderMessage), "")
	}
	for _, order := range msg.BinaryOptionsOrdersToCreate {
		dummyBinaryOptionsOrderMessage := v2.MsgCreateBinaryOptionsLimitOrder{
			Order: *order,
		}
		gasMeter.ConsumeGas(DetermineGas(&dummyBinaryOptionsOrderMessage), "")
	}

	return k.ExecuteBatchUpdateOrders(
		ctx,
		sender,
		msg.SubaccountId,
		msg.SpotMarketIdsToCancelAll,
		msg.DerivativeMarketIdsToCancelAll,
		msg.BinaryOptionsMarketIdsToCancelAll,
		msg.SpotOrdersToCancel,
		msg.DerivativeOrdersToCancel,
		msg.BinaryOptionsOrdersToCancel,
		msg.SpotOrdersToCreate,
		msg.DerivativeOrdersToCreate,
		msg.BinaryOptionsOrdersToCreate,
		msg.SpotMarketOrdersToCreate,
		msg.DerivativeMarketOrdersToCreate,
		msg.BinaryOptionsMarketOrdersToCreate,
	)
}
