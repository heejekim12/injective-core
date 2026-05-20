package keeper

import (
	"context"
	"errors"
	"fmt"

	cosmoserrors "cosmossdk.io/errors"
	"cosmossdk.io/math"
	storetypes "cosmossdk.io/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/ethereum/go-ethereum/common"

	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/types"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/types/v2"
	oracletypes "github.com/InjectiveLabs/injective-core/injective-chain/modules/oracle/types"
)

type DerivativesMsgServer struct {
	*Keeper
}

// Using a map for the list of enabled oracle types to improve lookup time
var enabledOracleTypes = map[oracletypes.OracleType]struct{}{
	oracletypes.OracleType_Coinbase:             {},
	oracletypes.OracleType_Razor:                {},
	oracletypes.OracleType_Dia:                  {},
	oracletypes.OracleType_API3:                 {},
	oracletypes.OracleType_Uma:                  {},
	oracletypes.OracleType_Pyth:                 {},
	oracletypes.OracleType_PythPro:              {},
	oracletypes.OracleType_Stork:                {},
	oracletypes.OracleType_ChainlinkDataStreams: {},
	oracletypes.OracleType_SedaFast:             {},
}

// NewDerivativesMsgServerImpl returns an implementation of the exchange MsgServer interface for the provided Keeper
// for derivatives market functions.
func NewDerivativesMsgServerImpl(keeper *Keeper) DerivativesMsgServer {
	return DerivativesMsgServer{
		Keeper: keeper,
	}
}

func (k DerivativesMsgServer) InstantPerpetualMarketLaunch(
	c context.Context, msg *v2.MsgInstantPerpetualMarketLaunch,
) (*v2.MsgInstantPerpetualMarketLaunchResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer k.Meter(ctx).FuncTiming(&ctx, "InstantPerpetualMarketLaunch")()

	isRegistrationAllowed := k.IsAdmin(ctx, msg.Sender)

	if !k.GetCachedParams(ctx).IsInstantDerivativeMarketLaunchEnabled {
		return nil, types.ErrFeatureDisabled
	}

	if !isRegistrationAllowed {
		return nil, sdkerrors.ErrUnauthorized.Wrap("Unauthorized to instant launch a perpetual market")
	}

	senderAddr, _ := sdk.AccAddressFromBech32(msg.Sender)

	if err := k.checkDenomMinNotional(ctx, senderAddr, msg.QuoteDenom, msg.MinNotional); err != nil {
		return nil, err
	}

	// check if the market launch proposal already exists
	marketID := types.NewPerpetualMarketID(msg.Ticker, msg.QuoteDenom, msg.OracleBase, msg.OracleQuote, msg.OracleType)
	if k.checkIfMarketLaunchProposalExist(ctx, marketID, types.ProposalTypePerpetualMarketLaunch, v2.ProposalTypePerpetualMarketLaunch) {
		k.Logger(ctx).Error("the perpetual market launch proposal already exists: marketID=%s", marketID.Hex())
		return nil, types.ErrMarketLaunchProposalAlreadyExists.Wrapf(
			"the perpetual market launch proposal already exists: marketID=%s", marketID.Hex(),
		)
	}

	fee := k.GetCachedParams(ctx).DerivativeMarketInstantListingFee
	err := k.DistributionKeeper.FundCommunityPool(ctx, sdk.Coins{fee}, senderAddr)
	if err != nil {
		k.Logger(ctx).Error("failed launching derivative market", err)
		return nil, err
	}

	adminInfo := v2.EmptyAdminInfo()
	_, _, err = k.PerpetualMarketLaunch(
		ctx, msg.Ticker, msg.QuoteDenom, msg.OracleBase, msg.OracleQuote,
		msg.OracleScaleFactor, msg.OracleType, msg.InitialMarginRatio,
		msg.MaintenanceMarginRatio, msg.ReduceMarginRatio, msg.MakerFeeRate, msg.TakerFeeRate,
		msg.MinPriceTickSize, msg.MinQuantityTickSize, msg.MinNotional, msg.OpenNotionalCap,
		&adminInfo, msg.CrossMarginEligible,
	)
	if err != nil {
		k.Logger(ctx).Error("failed launching derivative market", err)
		return nil, err
	}

	return &v2.MsgInstantPerpetualMarketLaunchResponse{}, err
}

func (k DerivativesMsgServer) InstantExpiryFuturesMarketLaunch(
	c context.Context, msg *v2.MsgInstantExpiryFuturesMarketLaunch,
) (*v2.MsgInstantExpiryFuturesMarketLaunchResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer k.Meter(ctx).FuncTiming(&ctx, "InstantExpiryFuturesMarketLaunch")()

	isRegistrationAllowed := k.IsAdmin(ctx, msg.Sender)

	if !k.GetCachedParams(ctx).IsInstantDerivativeMarketLaunchEnabled {
		return nil, types.ErrFeatureDisabled
	}

	if !isRegistrationAllowed {
		return nil, sdkerrors.ErrUnauthorized.Wrap("Unauthorized to instant launch an expiry futures market")
	}

	senderAddr, _ := sdk.AccAddressFromBech32(msg.Sender)

	if err := k.checkDenomMinNotional(ctx, senderAddr, msg.QuoteDenom, msg.MinNotional); err != nil {
		return nil, err
	}

	// check if the market launch proposal already exists
	marketID := types.NewExpiryFuturesMarketID(msg.Ticker, msg.QuoteDenom, msg.OracleBase, msg.OracleQuote, msg.OracleType, msg.Expiry)
	if k.checkIfMarketLaunchProposalExist(
		ctx, marketID, types.ProposalTypeExpiryFuturesMarketLaunch, v2.ProposalTypeExpiryFuturesMarketLaunch,
	) {
		k.Logger(ctx).Error("the expiry futures market launch proposal already exists: marketID=%s", marketID.Hex())
		return nil, types.ErrMarketLaunchProposalAlreadyExists.Wrapf(
			"the expiry futures market launch proposal already exists: marketID=%s", marketID.Hex(),
		)
	}

	fee := k.GetCachedParams(ctx).DerivativeMarketInstantListingFee
	err := k.DistributionKeeper.FundCommunityPool(ctx, sdk.Coins{fee}, senderAddr)
	if err != nil {
		k.Logger(ctx).Error("failed launching derivative market", err)
		return nil, err
	}

	adminInfo := v2.EmptyAdminInfo()
	if _, _, err := k.ExpiryFuturesMarketLaunch(
		ctx, msg.Ticker, msg.QuoteDenom,
		msg.OracleBase, msg.OracleQuote, msg.OracleScaleFactor, msg.OracleType, msg.Expiry,
		msg.InitialMarginRatio, msg.MaintenanceMarginRatio, msg.ReduceMarginRatio,
		msg.MakerFeeRate, msg.TakerFeeRate, msg.MinPriceTickSize, msg.MinQuantityTickSize,
		msg.MinNotional, msg.OpenNotionalCap, &adminInfo, msg.CrossMarginEligible,
	); err != nil {
		k.Logger(ctx).Error("failed launching derivative market", err)
		return nil, err
	}

	return &v2.MsgInstantExpiryFuturesMarketLaunchResponse{}, err
}

func (k DerivativesMsgServer) UpdateDerivativeMarket(c context.Context, msg *v2.MsgUpdateDerivativeMarket) (*v2.MsgUpdateDerivativeMarketResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer k.Meter(ctx).FuncTiming(&ctx, "UpdateDerivativeMarket")()

	market := k.GetDerivativeMarketByID(ctx, common.HexToHash(msg.MarketId))
	if market == nil {
		return nil, cosmoserrors.Wrap(types.ErrDerivativeMarketNotFound, "unknown market id")
	}

	hasCrossMarginEligibilityUpdate := msg.HasCrossMarginEligibilityUpdate()
	hasMarketAdminUpdate := msg.HasTickerUpdate() ||
		msg.HasMinPriceTickSizeUpdate() ||
		msg.HasMinQuantityTickSizeUpdate() ||
		msg.HasMinNotionalUpdate() ||
		msg.HasInitialMarginRatioUpdate() ||
		msg.HasMaintenanceMarginRatioUpdate() ||
		msg.HasReduceMarginRatioUpdate() ||
		msg.HasOpenNotionalCapUpdate()

	if hasCrossMarginEligibilityUpdate && !k.IsAdmin(ctx, msg.Admin) {
		return nil, cosmoserrors.Wrap(types.ErrInvalidAccessLevel, "only exchange module admins can update cross_margin_eligibility")
	}

	if hasMarketAdminUpdate || !hasCrossMarginEligibilityUpdate {
		switch {
		case market.Admin == "":
			if !k.IsAdmin(ctx, msg.Admin) {
				return nil, cosmoserrors.Wrap(types.ErrInvalidAccessLevel, "no market admin defined and sender is not an exchange module admin")
			}
		case market.Admin != msg.Admin:
			return nil, cosmoserrors.Wrapf(types.ErrInvalidAccessLevel, "market belongs to another admin (%v)", market.Admin)
		default:
			// only check permissions if the market has an admin

			if market.AdminPermissions == 0 {
				return nil, cosmoserrors.Wrap(types.ErrInvalidAccessLevel, "no permissions found")
			}

			permissions := types.MarketAdminPermissions(market.AdminPermissions)

			if msg.HasTickerUpdate() && !permissions.HasPerm(types.TickerPerm) {
				return nil, cosmoserrors.Wrap(types.ErrInvalidAccessLevel, "admin does not have permission to update ticker")
			}

			if msg.HasMinPriceTickSizeUpdate() && !permissions.HasPerm(types.MinPriceTickSizePerm) {
				return nil, cosmoserrors.Wrap(types.ErrInvalidAccessLevel, "admin does not have permission to update min_price_tick_size")
			}

			if msg.HasMinQuantityTickSizeUpdate() && !permissions.HasPerm(types.MinQuantityTickSizePerm) {
				return nil, cosmoserrors.Wrap(types.ErrInvalidAccessLevel, "admin does not have permission to update min_quantity_tick_size")
			}

			if msg.HasMinNotionalUpdate() && !permissions.HasPerm(types.MinNotionalPerm) {
				return nil, cosmoserrors.Wrap(types.ErrInvalidAccessLevel, "admin does not have permission to update market min_notional")
			}

			if msg.HasInitialMarginRatioUpdate() && !permissions.HasPerm(types.InitialMarginRatioPerm) {
				return nil, cosmoserrors.Wrap(types.ErrInvalidAccessLevel, "admin does not have permission to update initial_margin_ratio")
			}

			if msg.HasMaintenanceMarginRatioUpdate() && !permissions.HasPerm(types.MaintenanceMarginRatioPerm) {
				return nil, cosmoserrors.Wrap(types.ErrInvalidAccessLevel, "admin does not have permission to update maintenance_margin_ratio")
			}

			if msg.HasReduceMarginRatioUpdate() && !permissions.HasPerm(types.ReduceMarginRatioPerm) {
				return nil, cosmoserrors.Wrap(types.ErrInvalidAccessLevel, "admin does not have permission to update reduce_margin_ratio")
			}

			if msg.HasOpenNotionalCapUpdate() && !permissions.HasPerm(types.OpenNotionalCapPerm) {
				return nil, cosmoserrors.Wrap(types.ErrInvalidAccessLevel, "admin does not have permission to update open_notional_cap")
			}
		}
	}

	if msg.HasTickerUpdate() {
		market.Ticker = msg.NewTicker
	}

	if msg.HasMinPriceTickSizeUpdate() {
		market.MinPriceTickSize = msg.NewMinPriceTickSize
	}

	if msg.HasMinQuantityTickSizeUpdate() {
		market.MinQuantityTickSize = msg.NewMinQuantityTickSize
	}

	if msg.HasMinNotionalUpdate() {
		sender, err := sdk.AccAddressFromBech32(msg.Admin)
		if err != nil {
			return nil, err
		}
		if err := k.checkDenomMinNotional(ctx, sender, market.QuoteDenom, msg.NewMinNotional); err != nil {
			return nil, err
		}
		market.MinNotional = msg.NewMinNotional
	}

	if msg.HasOpenNotionalCapUpdate() {
		market.OpenNotionalCap = msg.NewOpenNotionalCap
	}

	params := k.GetParams(ctx)

	if msg.HasInitialMarginRatioUpdate() {
		// disallow admins from decreasing initial margin ratio below the default param
		if msg.NewInitialMarginRatio.LT(params.DefaultInitialMarginRatio) {
			return nil, types.ErrInvalidMarginRatio
		}

		market.InitialMarginRatio = msg.NewInitialMarginRatio
	}

	if msg.HasMaintenanceMarginRatioUpdate() {
		// disallow admins from decreasing maintenance margin ratio below the default param
		if msg.NewMaintenanceMarginRatio.LT(params.DefaultMaintenanceMarginRatio) {
			return nil, types.ErrInvalidMarginRatio
		}

		market.MaintenanceMarginRatio = msg.NewMaintenanceMarginRatio
	}

	if msg.HasReduceMarginRatioUpdate() {
		// disallow admins from decreasing reduce margin ratio below the default param
		if msg.NewReduceMarginRatio.LT(params.DefaultReduceMarginRatio) {
			return nil, types.ErrInvalidMarginRatio
		}

		market.ReduceMarginRatio = msg.NewReduceMarginRatio
	}

	if market.InitialMarginRatio.LTE(market.MaintenanceMarginRatio) {
		return nil, types.ErrMarginsRelation
	}

	if market.ReduceMarginRatio.LT(market.InitialMarginRatio) {
		return nil, types.ErrMarginsRelation
	}

	if hasCrossMarginEligibilityUpdate {
		market.CrossMarginEligible = msg.CrossMarginEligibility == v2.CrossMarginEligibility_CM_ELIGIBILITY_ELIGIBLE
	}

	k.SetDerivativeMarketWithInfo(ctx, market, nil, nil, nil)

	return &v2.MsgUpdateDerivativeMarketResponse{}, nil
}

func (k DerivativesMsgServer) CreateDerivativeLimitOrder(
	c context.Context, msg *v2.MsgCreateDerivativeLimitOrder,
) (*v2.MsgCreateDerivativeLimitOrderResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer k.Meter(ctx).FuncTiming(&ctx, "CreateDerivativeLimitOrder")()

	if k.IsFixedGasEnabled() {
		ctx.GasMeter().ConsumeGas(DetermineGas(msg), "MsgCreateDerivativeLimitOrder")
		ctx = ctx.WithGasMeter(storetypes.NewInfiniteGasMeter())
	}

	account, _ := sdk.AccAddressFromBech32(msg.Sender)

	market, markPrice := k.GetDerivativeMarketWithMarkPrice(ctx, msg.Order.MarketID(), true)
	if market == nil || markPrice.IsNil() {
		k.Logger(ctx).Error(
			"active derivative market with valid mark price doesn't exist",
			"marketId", msg.Order.MarketId,
			"mark price", markPrice.String(),
		)
		return nil, types.ErrDerivativeMarketNotFound.Wrapf("active derivative market for marketID %s not found", msg.Order.MarketId)
	}

	orderHash, err := k.DerivativeKeeper.CreateDerivativeLimitOrder(ctx, account, &msg.Order, market, markPrice)

	if err != nil {
		return nil, err
	}

	return &v2.MsgCreateDerivativeLimitOrderResponse{
		OrderHash: orderHash.Hex(),
		Cid:       msg.Order.Cid(),
	}, nil
}

func (k DerivativesMsgServer) BatchCreateDerivativeLimitOrders(
	c context.Context, msg *v2.MsgBatchCreateDerivativeLimitOrders,
) (*v2.MsgBatchCreateDerivativeLimitOrdersResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer k.Meter(ctx).FuncTiming(&ctx, "BatchCreateDerivativeLimitOrders")()

	if k.IsFixedGasEnabled() {
		ctx.GasMeter().ConsumeGas(DetermineGas(msg), "MsgBatchCreateDerivativeLimitOrders")
		ctx = ctx.WithGasMeter(storetypes.NewInfiniteGasMeter())
	}

	sender := sdk.MustAccAddressFromBech32(msg.Sender)

	orderFailEvent := v2.EventOrderFail{
		Account: sender.Bytes(),
		Hashes:  make([][]byte, 0),
		Flags:   make([]uint32, 0),
		Cids:    make([]string, 0),
	}

	marketsCache := make(map[common.Hash]*v2.FullDerivativeMarket)
	orderHashes := make([]string, len(msg.Orders))
	createdOrdersCids := make([]string, 0)
	failedOrdersCids := make([]string, 0)

	for idx := range msg.Orders {
		orderHash, createdCid, failedCid := k.createDerivativeLimitOrderFromBatch(ctx, sender, msg.Orders[idx], marketsCache, &orderFailEvent)
		orderHashes[idx] = orderHash

		if createdCid != "" {
			createdOrdersCids = append(createdOrdersCids, createdCid)
		}

		if failedCid != "" {
			failedOrdersCids = append(failedOrdersCids, failedCid)
		}
	}

	if !orderFailEvent.IsEmpty() {
		k.EmitEvent(ctx, &orderFailEvent)
	}

	return &v2.MsgBatchCreateDerivativeLimitOrdersResponse{
		OrderHashes:       orderHashes,
		CreatedOrdersCids: createdOrdersCids,
		FailedOrdersCids:  failedOrdersCids,
	}, nil
}

// createDerivativeLimitOrderFromBatch processes a single derivative limit order from a batch
func (k DerivativesMsgServer) createDerivativeLimitOrderFromBatch(
	ctx sdk.Context,
	sender sdk.AccAddress,
	order v2.DerivativeOrder,
	marketsCache map[common.Hash]*v2.FullDerivativeMarket,
	orderFailEvent *v2.EventOrderFail,
) (orderHashString, createdCid, failedCid string) {
	defer k.Meter(ctx).FuncTiming(&ctx, "createDerivativeLimitOrderFromBatch")()

	marketID := order.MarketID()

	fullMarket, ok := marketsCache[marketID]
	if !ok {
		market, markPrice := k.GetDerivativeMarketWithMarkPrice(ctx, marketID, true)

		// edge case when active market doesn't exist
		if market == nil || markPrice.IsNil() {
			return fmt.Sprintf("%d", types.ErrDerivativeMarketNotFound.ABCICode()), "", ""
		}

		fullMarket = &v2.FullDerivativeMarket{Market: market, MarkPrice: markPrice}
		marketsCache[marketID] = fullMarket
	}

	orderHash, err := k.DerivativeKeeper.CreateDerivativeLimitOrder(ctx, sender, &order, fullMarket.Market, fullMarket.MarkPrice)
	if err != nil {
		sdkerror := &cosmoserrors.Error{}
		if errors.As(err, &sdkerror) {
			orderFailEvent.AddOrderFail(orderHash, order.Cid(), sdkerror.ABCICode())
			return fmt.Sprintf("%d", sdkerror.ABCICode()), "", order.Cid()
		}
		return "", "", ""
	}

	return orderHash.Hex(), order.Cid(), ""
}

func (k DerivativesMsgServer) CreateDerivativeMarketOrder(
	c context.Context, msg *v2.MsgCreateDerivativeMarketOrder,
) (*v2.MsgCreateDerivativeMarketOrderResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer k.Meter(ctx).FuncTiming(&ctx, "CreateDerivativeMarketOrder")()

	if k.IsFixedGasEnabled() {
		ctx.GasMeter().ConsumeGas(DetermineGas(msg), "MsgCreateDerivativeMarketOrder")
		ctx = ctx.WithGasMeter(storetypes.NewInfiniteGasMeter())
	}

	account, _ := sdk.AccAddressFromBech32(msg.Sender)

	market, markPrice := k.GetDerivativeMarketWithMarkPrice(ctx, msg.Order.MarketID(), true)
	if market == nil {
		k.Logger(ctx).Error("active derivative market doesn't exist", "marketId", msg.Order.MarketId)
		return nil, types.ErrDerivativeMarketNotFound.Wrapf("active derivative market for marketID %s not found", msg.Order.MarketId)
	}

	orderHash, results, err := k.DerivativeKeeper.CreateDerivativeMarketOrder(ctx, account, &msg.Order, market, markPrice)
	if err != nil {
		return nil, err
	}

	resp := &v2.MsgCreateDerivativeMarketOrderResponse{
		OrderHash: orderHash.Hex(),
		Cid:       msg.Order.Cid(),
	}
	if results != nil {
		resp.Results = results
	}
	return resp, nil
}

func (k DerivativesMsgServer) CancelDerivativeOrder(
	c context.Context, msg *v2.MsgCancelDerivativeOrder,
) (*v2.MsgCancelDerivativeOrderResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer k.Meter(ctx).FuncTiming(&ctx, "CancelDerivativeOrder")()

	if k.IsFixedGasEnabled() {
		ctx.GasMeter().ConsumeGas(DetermineGas(msg), "MsgCancelDerivativeOrder")
		ctx = ctx.WithGasMeter(storetypes.NewInfiniteGasMeter())
	}

	var (
		marketID     = common.HexToHash(msg.MarketId)
		sender       = sdk.MustAccAddressFromBech32(msg.Sender)
		subaccountID = types.MustGetSubaccountIDOrDeriveFromNonce(sender, msg.SubaccountId)
		identifier   = types.GetOrderIdentifier(msg.OrderHash, msg.Cid)
	)

	market := k.GetDerivativeMarketByID(ctx, marketID)
	if err := k.DerivativeKeeper.CancelDerivativeOrder(ctx, subaccountID, identifier, market, marketID, msg.OrderMask); err != nil {
		k.EmitEvent(ctx, v2.NewEventOrderCancelFail(marketID, subaccountID, msg.OrderHash, msg.Cid, err))
		return nil, err
	}

	return &v2.MsgCancelDerivativeOrderResponse{}, nil
}

func (k DerivativesMsgServer) BatchCancelDerivativeOrders(
	c context.Context, msg *v2.MsgBatchCancelDerivativeOrders,
) (*v2.MsgBatchCancelDerivativeOrdersResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer k.Meter(ctx).FuncTiming(&ctx, "BatchCancelDerivativeOrders")()

	successes := make([]bool, len(msg.Data))
	for idx := range msg.Data {
		if _, err := k.CancelDerivativeOrder(ctx, &v2.MsgCancelDerivativeOrder{
			Sender:       msg.Sender,
			MarketId:     msg.Data[idx].MarketId,
			SubaccountId: msg.Data[idx].SubaccountId,
			OrderHash:    msg.Data[idx].OrderHash,
			OrderMask:    msg.Data[idx].OrderMask,
			Cid:          msg.Data[idx].Cid,
		}); err == nil {
			successes[idx] = true
		}
	}

	return &v2.MsgBatchCancelDerivativeOrdersResponse{
		Success: successes,
	}, nil
}

func (k DerivativesMsgServer) IncreasePositionMargin(
	c context.Context, msg *v2.MsgIncreasePositionMargin,
) (*v2.MsgIncreasePositionMarginResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer k.Meter(ctx).FuncTiming(&ctx, "IncreasePositionMargin")()

	if k.IsFixedGasEnabled() {
		ctx.GasMeter().ConsumeGas(DetermineGas(msg), "MsgIncreasePositionMargin")
		ctx = ctx.WithGasMeter(storetypes.NewInfiniteGasMeter())
	}

	var (
		sender                  = sdk.MustAccAddressFromBech32(msg.Sender)
		sourceSubaccountID      = types.MustGetSubaccountIDOrDeriveFromNonce(sender, msg.SourceSubaccountId)
		destinationSubaccountID = types.MustGetSubaccountIDOrDeriveFromNonce(sender, msg.DestinationSubaccountId)
		marketID                = common.HexToHash(msg.MarketId)
	)
	// Cross-margin note: When source == destination, this moves funds from QuoteBalance to
	// PositionMarginTotal within the same pool — net equity is unchanged. When source != destination
	// and source is CROSS, funds leave the source pool and we must enforce maintenance/admission.

	market := k.GetDerivativeMarket(ctx, marketID, true)
	if market == nil {
		k.Logger(ctx).Error("active derivative market doesn't exist", "marketId", marketID)
		return nil, types.ErrDerivativeMarketNotFound.Wrapf("active derivative market for marketID %s not found", marketID.Hex())
	}

	// Cross margin: if source != destination and source is CROSS, funds leave the source pool.
	// Enforce portfolio-level maintenance and order admission (mirrors DecreasePositionMargin).
	//
	// Skip for default source subaccounts: DecrementDepositOrChargeFromBank may charge from bank
	// balance, which is excluded from cross-margin QuoteBalance. Running the pre-check would
	// incorrectly assume the full amount leaves the pool, rejecting valid bank-funded operations.
	chainFormatAmount := market.NotionalToChainFormat(msg.Amount)

	if sourceSubaccountID != destinationSubaccountID && !types.IsDefaultSubaccountID(sourceSubaccountID) {
		profile, _ := k.RiskEngine().EffectiveProfile(ctx, sourceSubaccountID)
		if profile != nil && profile.Mode == v2.RiskMode_RISK_MODE_CROSS {
			effectiveNotional := msg.Amount

			snapshot, err := k.RiskEngine().BuildCrossPoolSnapshot(ctx, sourceSubaccountID, market.QuoteDenom, market.QuoteDecimals)
			if err != nil {
				return nil, err
			}

			// No positions and no orders: the pool has no risk exposure, so the transfer
			// is unconditionally safe. Mirrors the zero-risk bypass in
			// ensureCrossMarginMaintenanceAfterCollateralDecrease.
			if snapshot.MaintenanceMarginTotal.IsPositive() || snapshot.OrderLockRequirement.IsPositive() {
				equityLiqAfter := snapshot.EquityLiquidation.Sub(effectiveNotional)
				equityAdmAfter := snapshot.EquityAdmission.Sub(effectiveNotional)

				// Emergency pause: strip positive UPnL to enforce isolated-margin-level maintenance.
				if k.GetParams(ctx).CrossMarginParams.EmergencyPaused {
					equityLiqAfter, equityAdmAfter = snapshot.StripPositiveUPnL(equityLiqAfter, equityAdmAfter)
				}

				if equityLiqAfter.LT(snapshot.MaintenanceMarginTotal) {
					return nil, cosmoserrors.Wrapf(
						types.ErrInsufficientMargin,
						"cross-margin maintenance check failed after position margin increase: equity_after %s < maintenance %s",
						equityLiqAfter.String(),
						snapshot.MaintenanceMarginTotal.String(),
					)
				}

				if !k.GetParams(ctx).CrossMarginParams.EmergencyPaused && equityAdmAfter.LT(snapshot.OrderLockRequirement) {
					return nil, cosmoserrors.Wrapf(
						types.ErrInsufficientMargin,
						"cross-margin admission check failed after position margin increase: equity_admission_after %s < order_lock_requirement %s",
						equityAdmAfter.String(),
						snapshot.OrderLockRequirement.String(),
					)
				}
			}
		}
	}

	chainFormatMarginIncrement, err := k.DecrementDepositOrChargeFromBank(ctx, sourceSubaccountID, market.QuoteDenom, chainFormatAmount)
	if err != nil {
		return nil, err
	}

	// Evict cross-pool snapshot cache for source: collateral has been debited.
	k.RiskEngine().EvictCrossPoolSnapshotCache(ctx, sourceSubaccountID)

	k.IncrementMarketBalance(ctx, marketID, chainFormatMarginIncrement)
	marginIncrement := market.NotionalFromChainFormat(chainFormatMarginIncrement)

	position := k.GetPosition(ctx, marketID, destinationSubaccountID)
	if position == nil {
		return nil, types.ErrPositionNotFound.Wrapf("subaccountID %s marketID %s", destinationSubaccountID.Hex(), marketID.Hex())
	}

	position.Margin = position.Margin.Add(marginIncrement)
	k.SavePosition(ctx, marketID, destinationSubaccountID, position)

	return &v2.MsgIncreasePositionMarginResponse{}, nil
}

func (k DerivativesMsgServer) DecreasePositionMargin(
	c context.Context, msg *v2.MsgDecreasePositionMargin,
) (*v2.MsgDecreasePositionMarginResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer k.Meter(ctx).FuncTiming(&ctx, "DecreasePositionMargin")()

	if k.IsFixedGasEnabled() {
		ctx.GasMeter().ConsumeGas(DetermineGas(msg), "MsgDecreasePositionMargin")
		ctx = ctx.WithGasMeter(storetypes.NewInfiniteGasMeter())
	}

	var (
		sender                  = sdk.MustAccAddressFromBech32(msg.Sender)
		sourceSubaccountID      = types.MustGetSubaccountIDOrDeriveFromNonce(sender, msg.SourceSubaccountId)
		destinationSubaccountID = types.MustGetSubaccountIDOrDeriveFromNonce(sender, msg.DestinationSubaccountId)
		marketID                = common.HexToHash(msg.MarketId)
	)

	market, markPrice := k.GetDerivativeMarketWithMarkPrice(ctx, marketID, true)
	if market == nil || markPrice.IsNil() {
		k.Logger(ctx).Error(
			"active derivative market with valid mark price doesn't exist",
			"marketId", msg.MarketId,
			"mark price", markPrice.String(),
		)

		return nil, types.ErrDerivativeMarketNotFound.Wrapf("active derivative market for marketID %s not found", marketID.Hex())
	}

	hasAllowedOracleType := k.isMarginDecreaseEnabledForOracle(market.OracleType)
	if !hasAllowedOracleType {
		return nil, types.ErrUnsupportedOracleType.Wrapf("margin withdrawal for %s oracle not supported", market.OracleType.String())
	}

	pricePairState := k.OracleKeeper.GetPricePairState(ctx, market.OracleType, market.OracleBase, market.OracleQuote, nil)
	if pricePairState == nil {
		return nil, types.ErrInvalidOracle.Wrapf(
			"type %s base %s quote %s", market.OracleType.String(), market.OracleBase, market.OracleQuote,
		)
	}

	currTime := ctx.BlockTime().Unix()

	params := k.GetParams(ctx)
	maxDelayThreshold := params.MarginDecreasePriceTimestampThresholdSeconds

	// enforce freshness of price
	exceedsDelay := (currTime-pricePairState.BaseTimestamp > maxDelayThreshold) ||
		(currTime-pricePairState.QuoteTimestamp > maxDelayThreshold)
	if exceedsDelay {
		return nil, types.ErrStaleOraclePrice.Wrapf(
			"price timestamp (base %d quote %d) vs curr time %d exceeds max delay threshold %d",
			pricePairState.BaseTimestamp,
			pricePairState.QuoteTimestamp,
			currTime,
			maxDelayThreshold,
		)
	}

	position := k.GetPosition(ctx, marketID, sourceSubaccountID)
	if position == nil {

		return nil, types.ErrPositionNotFound.Wrapf("subaccountID %s marketID %s", sourceSubaccountID.Hex(), marketID.Hex())
	}

	if market.IsPerpetual {
		funding := k.GetPerpetualMarketFunding(ctx, marketID)
		position.ApplyFunding(funding)
	}

	position.Margin = position.Margin.Sub(msg.Amount)

	// For cross-margin subaccounts, skip the per-position margin validation. In CM,
	// position.Margin is accounting state and can be negative (e.g. after adverse funding) —
	// pool-level equity covers the position. The pool-level maintenance/admission check
	// below validates solvency. For isolated subaccounts, ValidateDerivativePositionMarginDecrease
	// enforces Margin >= ReduceMarginRatio * Notional, which is strictly tighter than a sign check.
	profile, _ := k.RiskEngine().EffectiveProfile(ctx, sourceSubaccountID)
	isCross := profile != nil && profile.Mode == v2.RiskMode_RISK_MODE_CROSS
	if !isCross {
		if err := k.RiskEngine().ValidateDerivativePositionMarginDecrease(ctx, sourceSubaccountID, position, market, markPrice); err != nil {
			return nil, err
		}
	}

	// Cross margin: if the decreased margin leaves the subaccount's quote-denom pool,
	// enforce portfolio-level maintenance and keep existing open orders admissible.
	//
	// For non-default subaccounts, a self-decrease (dest == source) keeps funds in exchange
	// deposits, so net pool equity is unchanged and the check can be skipped.
	// For default subaccounts, IncrementDepositOrSendToBank forwards the integer portion to
	// bank, and bank balance is excluded from cross-margin QuoteBalance, so a self-decrease
	// still reduces pool equity and the check must run.
	needsCrossCheck := destinationSubaccountID != sourceSubaccountID || types.IsDefaultSubaccountID(sourceSubaccountID)
	if profile != nil && profile.Mode == v2.RiskMode_RISK_MODE_CROSS && needsCrossCheck {
		snapshot, err := k.RiskEngine().BuildCrossPoolSnapshot(ctx, sourceSubaccountID, market.QuoteDenom, market.QuoteDecimals)
		if err != nil {
			return nil, err
		}

		// Compute the effective pool outflow. For a default-subaccount self-decrease,
		// IncrementDepositOrSendToBank sweeps only the integer portion to bank; the
		// fractional remainder stays in exchange deposits (part of pool QuoteBalance).
		// The actual equity decrease is the bank-swept portion, not the full msg.Amount.
		poolOutflow := msg.Amount
		if destinationSubaccountID == sourceSubaccountID && types.IsDefaultSubaccountID(sourceSubaccountID) {
			chainAmount := market.NotionalToChainFormat(msg.Amount)
			deposit := k.GetDeposit(ctx, sourceSubaccountID, market.QuoteDenom)
			newAvail := deposit.AvailableBalance.Add(chainAmount)
			bankSweep := newAvail.TruncateInt()
			if bankSweep.IsPositive() {
				poolOutflow = types.NotionalFromChainFormat(bankSweep.ToLegacyDec(), market.QuoteDecimals)
			} else {
				poolOutflow = math.LegacyZeroDec()
			}
		}

		equityLiqAfter := snapshot.EquityLiquidation.Sub(poolOutflow)
		equityAdmAfter := snapshot.EquityAdmission.Sub(poolOutflow)

		// Emergency pause: strip positive UPnL to enforce isolated-margin-level maintenance.
		if params.CrossMarginParams.EmergencyPaused {
			equityLiqAfter, equityAdmAfter = snapshot.StripPositiveUPnL(equityLiqAfter, equityAdmAfter)
		}

		if equityLiqAfter.LT(snapshot.MaintenanceMarginTotal) {
			return nil, cosmoserrors.Wrapf(
				types.ErrInsufficientMargin,
				"cross-margin maintenance check failed after position margin decrease: equity_after %s < maintenance %s",
				equityLiqAfter.String(),
				snapshot.MaintenanceMarginTotal.String(),
			)
		}

		if !params.CrossMarginParams.EmergencyPaused && equityAdmAfter.LT(snapshot.OrderLockRequirement) {
			return nil, cosmoserrors.Wrapf(
				types.ErrInsufficientMargin,
				"cross-margin admission check failed after position margin decrease: equity_admission_after %s < order_lock_requirement %s",
				equityAdmAfter.String(),
				snapshot.OrderLockRequirement.String(),
			)
		}
	}

	marketBalance := k.GetMarketBalance(ctx, marketID)
	chainFormatMarginDecrease := market.NotionalToChainFormat(msg.Amount)
	if marketBalance.LT(chainFormatMarginDecrease) {
		return nil, types.ErrInsufficientMarketBalance
	}
	k.DecrementMarketBalance(ctx, marketID, chainFormatMarginDecrease)

	k.SavePosition(ctx, marketID, sourceSubaccountID, position)
	k.IncrementDepositOrSendToBank(ctx, destinationSubaccountID, market.QuoteDenom, chainFormatMarginDecrease)

	// Evict destination cross-pool snapshot cache: collateral has been credited.
	// Source is already evicted by SavePosition above.
	if destinationSubaccountID != sourceSubaccountID {
		k.RiskEngine().EvictCrossPoolSnapshotCache(ctx, destinationSubaccountID)
	}

	return &v2.MsgDecreasePositionMarginResponse{}, nil
}

func (k DerivativesMsgServer) LaunchPerpetualMarket(
	c context.Context, msg *v2.MsgPerpetualMarketLaunch,
) (*v2.MsgPerpetualMarketLaunchResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer k.Meter(ctx).FuncTiming(&ctx, "LaunchPerpetualMarket")()

	if !k.IsGovernanceAuthorityAddress(msg.Sender) {
		return nil, sdkerrors.ErrUnauthorized
	}

	if err := k.HandlePerpetualMarketLaunchProposal(ctx, msg.Proposal); err != nil {
		return nil, err
	}

	return &v2.MsgPerpetualMarketLaunchResponse{}, nil
}

func (k DerivativesMsgServer) LaunchExpiryFuturesMarket(
	c context.Context, msg *v2.MsgExpiryFuturesMarketLaunch,
) (*v2.MsgExpiryFuturesMarketLaunchResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer k.Meter(ctx).FuncTiming(&ctx, "LaunchExpiryFuturesMarket")()

	if !k.IsGovernanceAuthorityAddress(msg.Sender) {
		return nil, sdkerrors.ErrUnauthorized
	}

	if err := k.HandleExpiryFuturesMarketLaunchProposal(ctx, msg.Proposal); err != nil {
		return nil, err
	}

	return &v2.MsgExpiryFuturesMarketLaunchResponse{}, nil
}

func (k DerivativesMsgServer) DerivativeMarketParamUpdate(
	c context.Context, msg *v2.MsgDerivativeMarketParamUpdate,
) (*v2.MsgDerivativeMarketParamUpdateResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer k.Meter(ctx).FuncTiming(&ctx, "DerivativeMarketParamUpdate")()

	if !k.IsGovernanceAuthorityAddress(msg.Sender) {
		return nil, sdkerrors.ErrUnauthorized
	}

	if err := k.HandleDerivativeMarketParamUpdateProposal(ctx, msg.Proposal); err != nil {
		return nil, err
	}

	return &v2.MsgDerivativeMarketParamUpdateResponse{}, nil
}

func (DerivativesMsgServer) isMarginDecreaseEnabledForOracle(oracleType oracletypes.OracleType) bool {
	_, found := enabledOracleTypes[oracleType]
	return found
}
