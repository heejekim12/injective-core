package keeper

import (
	"context"
	"errors"
	"fmt"

	"cosmossdk.io/math"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/keeper/utils"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/types"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/types/v2"
)

var _ v2.QueryServer = queryServer{}

type queryServer struct {
	Keeper *Keeper
}

func NewQueryServer(k *Keeper) v2.QueryServer {
	return createQueryServer(k)
}

func createQueryServer(k *Keeper) queryServer {
	return queryServer{
		Keeper: k,
	}
}

func (q queryServer) PositionsInMarket(
	c context.Context, req *v2.QueryPositionsInMarketRequest,
) (*v2.QueryPositionsInMarketResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "PositionsInMarket")()

	res := &v2.QueryPositionsInMarketResponse{
		State: q.Keeper.GetAllPositionsByMarket(ctx, common.HexToHash(req.MarketId)),
	}

	return res, nil
}

func (q queryServer) L3DerivativeOrderBook(
	c context.Context, req *v2.QueryFullDerivativeOrderbookRequest,
) (*v2.QueryFullDerivativeOrderbookResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "L3DerivativeOrderBook")()

	marketId := common.HexToHash(req.MarketId)
	sequence := q.Keeper.GetOrderbookSequence(ctx, marketId)
	res := &v2.QueryFullDerivativeOrderbookResponse{
		Bids: q.Keeper.GetAllStandardizedDerivativeLimitOrdersByMarketDirection(ctx, marketId, true),
		Asks: q.Keeper.GetAllStandardizedDerivativeLimitOrdersByMarketDirection(ctx, marketId, false),
		Seq:  sequence,
	}
	return res, nil
}

func (q queryServer) L3SpotOrderBook(c context.Context, req *v2.QueryFullSpotOrderbookRequest) (*v2.QueryFullSpotOrderbookResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "L3SpotOrderBook")()

	marketId := common.HexToHash(req.MarketId)
	sequence := q.Keeper.GetOrderbookSequence(ctx, marketId)
	res := &v2.QueryFullSpotOrderbookResponse{
		Bids: q.Keeper.GetAllStandardizedSpotLimitOrdersByMarketDirection(ctx, marketId, true),
		Asks: q.Keeper.GetAllStandardizedSpotLimitOrdersByMarketDirection(ctx, marketId, false),
		Seq:  sequence,
	}
	return res, nil
}

func (q queryServer) SubaccountRiskProfile(
	c context.Context, req *v2.QuerySubaccountRiskProfileRequest,
) (*v2.QuerySubaccountRiskProfileResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "SubaccountRiskProfile")()

	if req == nil || req.SubaccountId == "" {
		return nil, status.Error(codes.InvalidArgument, "subaccount_id is required")
	}

	if _, ok := types.IsValidSubaccountID(req.SubaccountId); !ok {
		return nil, status.Errorf(codes.InvalidArgument, "invalid subaccount_id format: %s", req.SubaccountId)
	}

	subaccountID := common.HexToHash(req.SubaccountId)
	profile, isDefault := q.Keeper.GetEffectiveSubaccountRiskProfile(ctx, subaccountID)

	return &v2.QuerySubaccountRiskProfileResponse{
		Profile:   profile,
		IsDefault: isDefault,
	}, nil
}

func (q queryServer) CrossMarginPoolSnapshot(
	c context.Context, req *v2.QueryCrossMarginPoolSnapshotRequest,
) (*v2.QueryCrossMarginPoolSnapshotResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "CrossMarginPoolSnapshot")()

	if req == nil || req.SubaccountId == "" {
		return nil, status.Error(codes.InvalidArgument, "subaccount_id is required")
	}
	if req.QuoteDenom == "" {
		return nil, status.Error(codes.InvalidArgument, "quote_denom is required")
	}

	if _, ok := types.IsValidSubaccountID(req.SubaccountId); !ok {
		return nil, status.Errorf(codes.InvalidArgument, "invalid subaccount_id format: %s", req.SubaccountId)
	}

	subaccountID := common.HexToHash(req.SubaccountId)

	// Validate that the subaccount is in cross-margin mode.
	// This query is only meaningful for cross-margin subaccounts; isolated-mode subaccounts
	// use per-position liquidation checks and should not rely on pool-level snapshots.
	profile, _ := q.Keeper.GetEffectiveSubaccountRiskProfile(ctx, subaccountID)
	if profile == nil || profile.Mode != v2.RiskMode_RISK_MODE_CROSS {
		return nil, status.Errorf(codes.FailedPrecondition,
			"subaccount %s is not in cross-margin mode; this query is only valid for cross-margin subaccounts",
			req.SubaccountId)
	}

	quoteDecimals, err := q.resolveQuoteDecimalsForSnapshot(ctx, subaccountID, req.QuoteDenom)
	if err != nil {
		return nil, err
	}

	snapshot, err := q.Keeper.RiskEngine().BuildCrossPoolSnapshot(ctx, subaccountID, req.QuoteDenom, quoteDecimals)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to build cross-margin snapshot: %v", err)
	}

	resp := &v2.QueryCrossMarginPoolSnapshotResponse{
		QuoteDenom:                   snapshot.QuoteDenom,
		QuoteBalance:                 snapshot.QuoteBalance,
		PositionMarginTotal:          snapshot.PositionMarginTotal,
		UnrealizedPnl:                snapshot.UnrealizedPnl,
		UnrealizedPnlEffective:       snapshot.UnrealizedPnlEff,
		EquityAdmission:              snapshot.EquityAdmission,
		EquityLiquidation:            snapshot.EquityLiquidation,
		InitialMarginTotal:           snapshot.InitialMarginTotal,
		MaintenanceMarginTotal:       snapshot.MaintenanceMarginTotal,
		InitialMarginWithOrdersTotal: snapshot.InitialMarginWithOrdersTotal,
		EntryLossTotal:               snapshot.EntryLossTotal,
		FeeReserveTotal:              snapshot.FeeReserveTotal,
		OrderLockRequirement:         snapshot.OrderLockRequirement,
		PositiveUpnlHaircutRate:      snapshot.PositiveUPnLHaircutRate,
		HealthFactor:                 snapshot.HealthFactor,
	}

	return resp, nil
}

// resolveQuoteDecimalsForSnapshot looks up quote decimals from active markets in the denom pool,
// validating consistency. Falls back to denom metadata for empty pools (preflight UX).
//
// Note: We intentionally do NOT reject queries for disabled denoms.
// During "graceful wind-down" (denom removed from whitelist but positions still exist),
// users need to query their pool status to understand their risk and close positions.
func (q queryServer) resolveQuoteDecimalsForSnapshot(
	ctx sdk.Context,
	subaccountID common.Hash,
	quoteDenom string,
) (uint32, error) {
	marketIDs := q.Keeper.GetAllActiveDerivativeMarketIDsForSubaccount(ctx, subaccountID)

	quoteDecimals, err := q.resolveDecimalsFromMarkets(ctx, marketIDs, quoteDenom)
	if err != nil {
		return 0, err
	}

	if quoteDecimals == 0 {
		// No active markets/orders for this denom pool. Try denom metadata first,
		// then fall back to scanning listed derivative markets (covers IBC denoms
		// that have markets but no bank metadata).
		decimals, err := q.Keeper.TokenDenomDecimals(ctx, quoteDenom)
		if err == nil && decimals > 0 {
			quoteDecimals = decimals
		} else {
			quoteDecimals = q.resolveDecimalsFromListedMarkets(ctx, quoteDenom)
		}
		if quoteDecimals == 0 {
			return 0, status.Errorf(codes.NotFound, "no markets found for quote denom %s", quoteDenom)
		}
	}
	return quoteDecimals, nil
}

// resolveDecimalsFromListedMarkets scans all enabled derivative markets for the given quote denom
// and returns QuoteDecimals from the first match. Used as a fallback when the subaccount has no
// exposure and bank metadata is unavailable (e.g. IBC denoms).
func (q queryServer) resolveDecimalsFromListedMarkets(ctx sdk.Context, quoteDenom string) uint32 {
	var decimals uint32
	q.Keeper.IterateDerivativeMarkets(ctx, nil, func(market *v2.DerivativeMarket) (stop bool) {
		if market.QuoteDenom == quoteDenom && market.QuoteDecimals > 0 {
			decimals = market.QuoteDecimals
			return true
		}
		return false
	})
	return decimals
}

func (q queryServer) resolveDecimalsFromMarkets(ctx sdk.Context, marketIDs []common.Hash, quoteDenom string) (uint32, error) {
	var quoteDecimals uint32
	for _, marketID := range marketIDs {
		market := q.Keeper.GetDerivativeMarketByID(ctx, marketID)
		if market == nil || market.QuoteDenom != quoteDenom || market.GetMarketType().IsBinaryOptions() {
			continue
		}
		if market.QuoteDecimals == 0 {
			return 0, status.Errorf(
				codes.Internal,
				"invalid quote decimals (0) for market %s (quote denom %s)",
				marketID.Hex(),
				quoteDenom,
			)
		}
		if quoteDecimals == 0 {
			quoteDecimals = market.QuoteDecimals
		} else if market.QuoteDecimals != quoteDecimals {
			return 0, status.Errorf(
				codes.Internal,
				"inconsistent quote decimals in cross-pool: market %s has %d, expected %d (quote denom %s)",
				marketID.Hex(),
				market.QuoteDecimals,
				quoteDecimals,
				quoteDenom,
			)
		}
	}
	return quoteDecimals, nil
}

func (q queryServer) QueryExchangeParams(c context.Context, _ *v2.QueryExchangeParamsRequest) (*v2.QueryExchangeParamsResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "QueryExchangeParams")()

	resp := &v2.QueryExchangeParamsResponse{
		Params: q.Keeper.GetParams(ctx),
	}

	return resp, nil
}

func (q queryServer) SubaccountDeposits(
	c context.Context, req *v2.QuerySubaccountDepositsRequest,
) (*v2.QuerySubaccountDepositsResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "SubaccountDeposits")()

	var subaccountID common.Hash
	if subaccount := req.GetSubaccount(); subaccount != nil {
		subaccountId, err := subaccount.GetSubaccountID()
		if err != nil {
			return nil, err
		}

		subaccountID = *subaccountId
	} else if subaccountId := req.GetSubaccountId(); subaccountId != "" {
		subaccountID = common.HexToHash(subaccountId)
	}

	resp := &v2.QuerySubaccountDepositsResponse{
		Deposits: q.Keeper.GetDeposits(ctx, subaccountID),
	}

	return resp, nil
}

func (q queryServer) SubaccountDeposit(
	c context.Context, req *v2.QuerySubaccountDepositRequest,
) (*v2.QuerySubaccountDepositResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "SubaccountDeposit")()

	resp := &v2.QuerySubaccountDepositResponse{
		Deposits: q.Keeper.GetDeposit(ctx, common.HexToHash(req.SubaccountId), req.Denom),
	}

	return resp, nil
}

func (q queryServer) ExchangeBalances(c context.Context, _ *v2.QueryExchangeBalancesRequest) (*v2.QueryExchangeBalancesResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "ExchangeBalances")()

	resp := &v2.QueryExchangeBalancesResponse{
		Balances: q.Keeper.GetAllExchangeBalances(ctx),
	}

	return resp, nil
}

func (q queryServer) AggregateVolume(c context.Context, req *v2.QueryAggregateVolumeRequest) (*v2.QueryAggregateVolumeResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "AggregateVolume")()

	if types.IsHexHash(req.Account) {
		volumes := q.Keeper.GetAllSubaccountMarketAggregateVolumesBySubaccount(ctx, common.HexToHash(req.Account))
		return &v2.QueryAggregateVolumeResponse{AggregateVolumes: volumes}, nil
	}

	accAddress, err := sdk.AccAddressFromBech32(req.Account)
	if err != nil {
		return nil, err
	}

	resp := &v2.QueryAggregateVolumeResponse{
		AggregateVolumes: q.Keeper.GetAllSubaccountMarketAggregateVolumesByAccAddress(ctx, accAddress),
	}

	return resp, nil
}

func (q queryServer) AggregateVolumes(c context.Context, req *v2.QueryAggregateVolumesRequest) (*v2.QueryAggregateVolumesResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "AggregateVolumes")()

	marketVolumes, marketIDs, marketIDMap := q.processMarketVolumes(ctx, req.MarketIds)
	accountVolumes, err := q.processAccountVolumes(ctx, req.Accounts, marketIDs, marketIDMap)
	if err != nil {
		return nil, err
	}

	resp := &v2.QueryAggregateVolumesResponse{
		AggregateAccountVolumes: accountVolumes,
		AggregateMarketVolumes:  marketVolumes,
	}

	return resp, nil
}

func (q queryServer) processMarketVolumes(
	ctx sdk.Context, marketIDs []string,
) ([]*v2.MarketVolume, []common.Hash, map[common.Hash]struct{}) {
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "processMarketVolumes")()

	marketVolumes := make([]*v2.MarketVolume, 0, len(marketIDs))
	processedMarketIDs := make([]common.Hash, 0, len(marketIDs))
	marketIDMap := make(map[common.Hash]struct{})

	for _, marketId := range marketIDs {
		marketID := common.HexToHash(marketId)

		// skip duplicate marketIDs
		if _, found := marketIDMap[marketID]; found {
			continue
		}

		volume := q.Keeper.GetMarketAggregateVolume(ctx, marketID)
		marketVolumes = append(marketVolumes, &v2.MarketVolume{
			MarketId: marketID.Hex(),
			Volume:   volume,
		})

		// minor optimization so we don't check account volumes for markets that have 0 volume
		if !volume.IsZero() {
			processedMarketIDs = append(processedMarketIDs, marketID)
		}

		marketIDMap[marketID] = struct{}{}
	}

	return marketVolumes, processedMarketIDs, marketIDMap
}

func (q queryServer) processAccountVolumes(
	ctx sdk.Context, accounts []string, marketIDs []common.Hash, marketIDMap map[common.Hash]struct{},
) ([]*v2.AggregateAccountVolumeRecord, error) {
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "processAccountVolumes")()

	accountVolumes := make([]*v2.AggregateAccountVolumeRecord, 0, len(accounts))

	for _, account := range accounts {
		accAddress, err := sdk.AccAddressFromBech32(account)
		if err != nil && !types.IsHexHash(account) {
			return nil, err
		}

		volumes, accountStr := q.getAccountVolumes(ctx, account, accAddress, marketIDs, marketIDMap)

		accountVolumes = append(accountVolumes, &v2.AggregateAccountVolumeRecord{
			Account:       accountStr,
			MarketVolumes: volumes,
		})
	}

	return accountVolumes, nil
}

func (q queryServer) getAccountVolumes(
	ctx sdk.Context, account string, accAddress sdk.AccAddress, marketIDs []common.Hash, marketIDMap map[common.Hash]struct{},
) ([]*v2.MarketVolume, string) {
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "getAccountVolumes")()

	var (
		volumes    []*v2.MarketVolume
		accountStr string
	)

	// still return the volumes if the input account is a subaccountID
	if types.IsHexHash(account) {
		subaccountID := common.HexToHash(account)
		accountStr = subaccountID.Hex()
		volumes = q.getSubaccountVolumes(ctx, subaccountID, marketIDs)
	} else {
		accountStr = accAddress.String()
		volumes = q.filterAccountVolumes(ctx, accAddress, marketIDMap)
	}

	return volumes, accountStr
}

func (q queryServer) getSubaccountVolumes(ctx sdk.Context, subaccountID common.Hash, marketIDs []common.Hash) []*v2.MarketVolume {
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "getSubaccountVolumes")()

	volumes := make([]*v2.MarketVolume, 0, len(marketIDs))

	for _, marketID := range marketIDs {
		volume := q.Keeper.GetSubaccountMarketAggregateVolume(ctx, subaccountID, marketID)
		volumes = append(volumes, &v2.MarketVolume{
			MarketId: marketID.Hex(),
			Volume:   volume,
		})
	}

	return volumes
}

func (q queryServer) filterAccountVolumes(
	ctx sdk.Context, accAddress sdk.AccAddress, marketIDMap map[common.Hash]struct{},
) []*v2.MarketVolume {
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "filterAccountVolumes")()

	volumes := q.Keeper.GetAllSubaccountMarketAggregateVolumesByAccAddress(ctx, accAddress)
	filteredVolumes := make([]*v2.MarketVolume, 0, len(volumes))

	// only include volumes for marketIDs requested
	for _, volume := range volumes {
		if _, ok := marketIDMap[common.HexToHash(volume.MarketId)]; ok {
			filteredVolumes = append(filteredVolumes, volume)
		}
	}

	return filteredVolumes
}

func (q queryServer) AggregateMarketVolume(
	c context.Context, req *v2.QueryAggregateMarketVolumeRequest,
) (*v2.QueryAggregateMarketVolumeResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "AggregateMarketVolume")()

	res := &v2.QueryAggregateMarketVolumeResponse{
		Volume: q.Keeper.GetMarketAggregateVolume(ctx, common.HexToHash(req.MarketId)),
	}

	return res, nil
}

func (q queryServer) AggregateMarketVolumes(
	c context.Context, req *v2.QueryAggregateMarketVolumesRequest,
) (*v2.QueryAggregateMarketVolumesResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "AggregateMarketVolumes")()

	// get all the market aggregate volumes if unspecified
	if len(req.MarketIds) == 0 {
		return &v2.QueryAggregateMarketVolumesResponse{Volumes: q.Keeper.GetAllMarketAggregateVolumes(ctx)}, nil
	}

	volumes := make([]*v2.MarketVolume, 0, len(req.MarketIds))
	for _, marketId := range req.MarketIds {
		marketID := common.HexToHash(marketId)

		volumes = append(volumes, &v2.MarketVolume{
			MarketId: marketID.Hex(),
			Volume:   q.Keeper.GetMarketAggregateVolume(ctx, marketID),
		})
	}

	res := &v2.QueryAggregateMarketVolumesResponse{
		Volumes: volumes,
	}

	return res, nil
}

func (q queryServer) AuctionExchangeTransferDenomDecimal(
	c context.Context,
	req *v2.QueryAuctionExchangeTransferDenomDecimalRequest,
) (*v2.QueryAuctionExchangeTransferDenomDecimalResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "AuctionExchangeTransferDenomDecimal")()

	if req.Denom == "" {
		return nil, errors.New("denom is required")
	}

	res := &v2.QueryAuctionExchangeTransferDenomDecimalResponse{
		Decimal: q.Keeper.GetAuctionExchangeTransferDenomDecimals(ctx, req.Denom),
	}

	return res, nil
}

func (q queryServer) AuctionExchangeTransferDenomDecimals(
	c context.Context,
	req *v2.QueryAuctionExchangeTransferDenomDecimalsRequest,
) (*v2.QueryAuctionExchangeTransferDenomDecimalsResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "AuctionExchangeTransferDenomDecimals")()

	if len(req.Denoms) == 0 {
		return &v2.QueryAuctionExchangeTransferDenomDecimalsResponse{
			DenomDecimals: q.Keeper.GetAllAuctionExchangeTransferDenomDecimals(ctx),
		}, nil
	}

	denomDecimals := make([]v2.DenomDecimals, 0, len(req.Denoms))
	for _, denom := range req.Denoms {
		denomDecimals = append(denomDecimals, v2.DenomDecimals{
			Denom:    denom,
			Decimals: q.Keeper.GetAuctionExchangeTransferDenomDecimals(ctx, denom),
		})
	}

	res := &v2.QueryAuctionExchangeTransferDenomDecimalsResponse{
		DenomDecimals: denomDecimals,
	}

	return res, nil
}

func (q queryServer) SpotMarkets(c context.Context, req *v2.QuerySpotMarketsRequest) (*v2.QuerySpotMarketsResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "SpotMarkets")()

	var status v2.MarketStatus
	if req.Status == "" {
		status = v2.MarketStatus_Active
	} else {
		status = v2.MarketStatus(v2.MarketStatus_value[req.Status])
	}

	if status == v2.MarketStatus_Unspecified {
		return &v2.QuerySpotMarketsResponse{Markets: []*v2.SpotMarket{}}, nil
	}

	filters := make([]utils.FilterFn[v2.MarketI], 0)
	filters = append(filters, utils.MarketStatusFilter(status))
	if ids := req.GetMarketIds(); len(ids) > 0 {
		filters = append(filters, utils.MarketIDFilter(ids...))
	}

	theFilter := utils.ChainFilters(filters...)
	spotMarkets := make([]*v2.SpotMarket, 0)
	q.Keeper.IterateSpotMarkets(ctx, nil, func(m *v2.SpotMarket) (stop bool) {
		if !theFilter(m) {
			return false
		}

		spotMarkets = append(spotMarkets, m)
		return false
	})

	resp := &v2.QuerySpotMarketsResponse{
		Markets: spotMarkets,
	}

	return resp, nil
}

func (q queryServer) SpotMarket(c context.Context, req *v2.QuerySpotMarketRequest) (*v2.QuerySpotMarketResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "SpotMarket")()

	marketID := common.HexToHash(req.MarketId)
	market := q.Keeper.GetSpotMarket(ctx, marketID, true)
	if market == nil {
		return nil, types.ErrSpotMarketNotFound
	}

	resp := &v2.QuerySpotMarketResponse{
		Market: market,
	}

	return resp, nil
}

func (q queryServer) FullSpotMarkets(c context.Context, req *v2.QueryFullSpotMarketsRequest) (*v2.QueryFullSpotMarketsResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "FullSpotMarkets")()

	var status v2.MarketStatus
	if req.Status == "" {
		status = v2.MarketStatus_Active
	} else {
		status = v2.MarketStatus(v2.MarketStatus_value[req.Status])
	}

	res := &v2.QueryFullSpotMarketsResponse{
		Markets: []*v2.FullSpotMarket{},
	}

	if status == v2.MarketStatus_Unspecified {
		return res, nil
	}

	filters := make([]utils.FilterFn[v2.MarketI], 0)
	filters = append(filters, utils.MarketStatusFilter(status))
	if ids := req.GetMarketIds(); len(ids) > 0 {
		filters = append(filters, utils.MarketIDFilter(ids...))
	}

	theFilter := utils.ChainFilters(filters...)

	spotMarkets := make([]*v2.FullSpotMarket, 0)
	q.Keeper.IterateSpotMarkets(ctx, nil, func(m *v2.SpotMarket) (stop bool) {
		if !theFilter(m) {
			return false
		}

		fm := &v2.FullSpotMarket{Market: m}

		midPrice, bestBuy, bestSell := q.Keeper.GetSpotMidPriceAndTOB(ctx, fm.GetMarket().MarketID())
		fm.MidPriceAndTob = &v2.MidPriceAndTOB{
			MidPrice:      midPrice,
			BestBuyPrice:  bestBuy,
			BestSellPrice: bestSell,
		}

		spotMarkets = append(spotMarkets, fm)
		return false
	})

	res.Markets = spotMarkets

	return res, nil
}

func (q queryServer) FullSpotMarket(c context.Context, req *v2.QueryFullSpotMarketRequest) (*v2.QueryFullSpotMarketResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "FullSpotMarket")()

	marketID := common.HexToHash(req.MarketId)
	market := q.Keeper.GetSpotMarket(ctx, marketID, true)
	if market == nil {
		return nil, types.ErrSpotMarketNotFound
	}

	fullMarket := &v2.FullSpotMarket{Market: market}
	if req.GetWithMidPriceAndTob() {
		midPrice, bestBuy, bestSell := q.Keeper.GetSpotMidPriceAndTOB(ctx, fullMarket.GetMarket().MarketID())
		fullMarket.MidPriceAndTob = &v2.MidPriceAndTOB{
			MidPrice:      midPrice,
			BestBuyPrice:  bestBuy,
			BestSellPrice: bestSell,
		}
	}

	res := &v2.QueryFullSpotMarketResponse{
		Market: fullMarket,
	}

	return res, nil
}

func (q queryServer) SpotOrderbook(c context.Context, req *v2.QuerySpotOrderbookRequest) (*v2.QuerySpotOrderbookResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "SpotOrderbook")()

	marketID := common.HexToHash(req.MarketId)
	var limit *uint64
	if req.Limit > 0 {
		limit = &req.Limit
	} else if req.LimitCumulativeNotional == nil && req.LimitCumulativeQuantity == nil {
		defaultLimit := types.DefaultQueryOrderbookLimit
		limit = &defaultLimit
	}

	buysPriceLevel := make([]*v2.Level, 0)
	if req.OrderSide == v2.OrderSide_Side_Unspecified || req.OrderSide == v2.OrderSide_Buy {
		buysPriceLevel = q.Keeper.GetOrderbookPriceLevels(
			ctx, true, marketID, true, limit, req.LimitCumulativeNotional, req.LimitCumulativeQuantity,
		)
	}
	sellsPriceLevel := make([]*v2.Level, 0)
	if req.OrderSide == v2.OrderSide_Side_Unspecified || req.OrderSide == v2.OrderSide_Sell {
		sellsPriceLevel = q.Keeper.GetOrderbookPriceLevels(
			ctx, true, marketID, false, limit, req.LimitCumulativeNotional, req.LimitCumulativeQuantity,
		)
	}

	sequence := q.Keeper.GetOrderbookSequence(ctx, marketID)

	resp := &v2.QuerySpotOrderbookResponse{
		BuysPriceLevel:  buysPriceLevel,
		SellsPriceLevel: sellsPriceLevel,
		Seq:             sequence,
	}

	return resp, nil
}

func (q queryServer) TraderSpotOrders(c context.Context, req *v2.QueryTraderSpotOrdersRequest) (*v2.QueryTraderSpotOrdersResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "TraderSpotOrders")()

	var (
		marketID     = common.HexToHash(req.MarketId)
		subaccountID = common.HexToHash(req.SubaccountId)
	)

	resp := &v2.QueryTraderSpotOrdersResponse{
		Orders: q.Keeper.GetAllTraderSpotLimitOrders(ctx, marketID, subaccountID),
	}

	return resp, nil
}

func (q queryServer) AccountAddressSpotOrders(
	c context.Context, req *v2.QueryAccountAddressSpotOrdersRequest,
) (*v2.QueryAccountAddressSpotOrdersResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "AccountAddressSpotOrders")()

	marketID := common.HexToHash(req.MarketId)
	accountAddress, err := sdk.AccAddressFromBech32(req.AccountAddress)
	if err != nil {
		return nil, types.ErrInvalidAddress
	}

	resp := &v2.QueryAccountAddressSpotOrdersResponse{
		Orders: q.Keeper.GetAccountAddressSpotLimitOrders(ctx, marketID, accountAddress),
	}

	return resp, nil
}

func (q queryServer) SpotOrdersByHashes(
	c context.Context, req *v2.QuerySpotOrdersByHashesRequest,
) (*v2.QuerySpotOrdersByHashesResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "SpotOrdersByHashes")()

	var (
		marketID     = common.HexToHash(req.MarketId)
		subaccountID = common.HexToHash(req.SubaccountId)
		orders       = make([]*v2.TrimmedSpotLimitOrder, 0, len(req.OrderHashes))
	)

	for _, hash := range req.OrderHashes {
		order := q.Keeper.GetSpotLimitOrderBySubaccountID(ctx, marketID, nil, subaccountID, common.HexToHash(hash))
		if order == nil {
			continue
		}

		// we append found orders only since including a nil element in the slice results in response being redacted
		orders = append(orders, order.ToTrimmed())
	}

	resp := &v2.QuerySpotOrdersByHashesResponse{
		Orders: orders,
	}

	return resp, nil
}

func (q queryServer) SubaccountOrders(c context.Context, req *v2.QuerySubaccountOrdersRequest) (*v2.QuerySubaccountOrdersResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "SubaccountOrders")()

	var (
		marketID     = common.HexToHash(req.MarketId)
		subaccountID = common.HexToHash(req.SubaccountId)
		buyOrders    = q.Keeper.GetSubaccountOrders(ctx, marketID, subaccountID, true, false)
		sellOrders   = q.Keeper.GetSubaccountOrders(ctx, marketID, subaccountID, false, false)
	)

	resp := &v2.QuerySubaccountOrdersResponse{
		BuyOrders:  buyOrders,
		SellOrders: sellOrders,
	}

	return resp, nil
}

func (q queryServer) TraderSpotTransientOrders(
	c context.Context, req *v2.QueryTraderSpotOrdersRequest,
) (*v2.QueryTraderSpotOrdersResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "TraderSpotTransientOrders")()

	var (
		marketID     = common.HexToHash(req.MarketId)
		subaccountID = common.HexToHash(req.SubaccountId)
	)

	resp := &v2.QueryTraderSpotOrdersResponse{
		Orders: q.Keeper.GetAllTransientTraderSpotLimitOrders(ctx, marketID, subaccountID),
	}

	return resp, nil
}

func (q queryServer) SpotMidPriceAndTOB(
	c context.Context, req *v2.QuerySpotMidPriceAndTOBRequest,
) (*v2.QuerySpotMidPriceAndTOBResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "SpotMidPriceAndTOB")()

	marketID := common.HexToHash(req.MarketId)

	market := q.Keeper.GetSpotMarket(ctx, marketID, true)
	if market == nil {
		return nil, types.ErrSpotMarketNotFound
	}

	midPrice, bestBuyPrice, bestSellPrice := q.Keeper.GetSpotMidPriceAndTOB(ctx, marketID)
	resp := &v2.QuerySpotMidPriceAndTOBResponse{
		MidPrice:      midPrice,
		BestBuyPrice:  bestBuyPrice,
		BestSellPrice: bestSellPrice,
	}

	return resp, nil
}

func (q queryServer) DerivativeMidPriceAndTOB(
	c context.Context, req *v2.QueryDerivativeMidPriceAndTOBRequest,
) (*v2.QueryDerivativeMidPriceAndTOBResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "DerivativeMidPriceAndTOB")()

	marketID := common.HexToHash(req.MarketId)
	midPrice, bestBuyPrice, bestSellPrice := q.Keeper.GetDerivativeMidPriceAndTOB(ctx, marketID)

	resp := &v2.QueryDerivativeMidPriceAndTOBResponse{
		MidPrice:      midPrice,
		BestBuyPrice:  bestBuyPrice,
		BestSellPrice: bestSellPrice,
	}

	return resp, nil
}

func (q queryServer) DerivativeOrderbook(
	c context.Context, req *v2.QueryDerivativeOrderbookRequest,
) (*v2.QueryDerivativeOrderbookResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "DerivativeOrderbook")()
	marketID := common.HexToHash(req.MarketId)

	var limit *uint64
	if req.Limit > 0 {
		limit = &req.Limit
	} else if req.LimitCumulativeNotional == nil {
		defaultLimit := types.DefaultQueryOrderbookLimit
		limit = &defaultLimit
	}

	sequence := q.Keeper.GetOrderbookSequence(ctx, marketID)

	resp := &v2.QueryDerivativeOrderbookResponse{
		BuysPriceLevel:  q.Keeper.GetOrderbookPriceLevels(ctx, false, marketID, true, limit, req.LimitCumulativeNotional, nil),
		SellsPriceLevel: q.Keeper.GetOrderbookPriceLevels(ctx, false, marketID, false, limit, req.LimitCumulativeNotional, nil),
		Seq:             sequence,
	}

	return resp, nil
}

func (q queryServer) TraderDerivativeOrders(
	c context.Context, req *v2.QueryTraderDerivativeOrdersRequest,
) (*v2.QueryTraderDerivativeOrdersResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "TraderDerivativeOrders")()

	var (
		marketID     = common.HexToHash(req.MarketId)
		subaccountID = common.HexToHash(req.SubaccountId)
	)

	resp := &v2.QueryTraderDerivativeOrdersResponse{
		Orders: q.Keeper.GetAllTraderDerivativeLimitOrders(ctx, marketID, subaccountID),
	}

	return resp, nil
}

func (q queryServer) AccountAddressDerivativeOrders(
	c context.Context, req *v2.QueryAccountAddressDerivativeOrdersRequest,
) (*v2.QueryAccountAddressDerivativeOrdersResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "AccountAddressDerivativeOrders")()

	marketID := common.HexToHash(req.MarketId)

	accountAddress, err := sdk.AccAddressFromBech32(req.AccountAddress)
	if err != nil {
		return nil, types.ErrInvalidAddress
	}

	resp := &v2.QueryAccountAddressDerivativeOrdersResponse{
		Orders: q.Keeper.GetDerivativeLimitOrdersByAddress(ctx, marketID, accountAddress),
	}

	return resp, nil
}

func (q queryServer) DerivativeOrdersByHashes(
	c context.Context, req *v2.QueryDerivativeOrdersByHashesRequest,
) (*v2.QueryDerivativeOrdersByHashesResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "DerivativeOrdersByHashes")()

	var (
		marketID     = common.HexToHash(req.MarketId)
		subaccountID = common.HexToHash(req.SubaccountId)
		orders       = make([]*v2.TrimmedDerivativeLimitOrder, 0, len(req.OrderHashes))
	)

	for _, hash := range req.OrderHashes {
		order := q.Keeper.GetDerivativeLimitOrderBySubaccountIDAndHash(ctx, marketID, nil, subaccountID, common.HexToHash(hash))
		if order == nil {
			continue
		}

		// we append found orders only since including a nil element in the slice results in response being redacted
		orders = append(orders, order.ToTrimmed())
	}

	resp := &v2.QueryDerivativeOrdersByHashesResponse{
		Orders: orders,
	}

	return resp, nil
}

func (q queryServer) TraderDerivativeTransientOrders(
	c context.Context, req *v2.QueryTraderDerivativeOrdersRequest,
) (*v2.QueryTraderDerivativeOrdersResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "TraderDerivativeTransientOrders")()

	var (
		marketID     = common.HexToHash(req.MarketId)
		subaccountID = common.HexToHash(req.SubaccountId)
	)

	res := &v2.QueryTraderDerivativeOrdersResponse{
		Orders: q.Keeper.GetAllTransientTraderDerivativeLimitOrders(ctx, marketID, subaccountID),
	}

	return res, nil
}

func (q queryServer) DerivativeMarkets(
	c context.Context, req *v2.QueryDerivativeMarketsRequest,
) (*v2.QueryDerivativeMarketsResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "DerivativeMarkets")()

	var status v2.MarketStatus
	if req.Status == "" {
		status = v2.MarketStatus_Active
	} else {
		status = v2.MarketStatus(v2.MarketStatus_value[req.Status])
	}

	if status == v2.MarketStatus_Unspecified {
		return &v2.QueryDerivativeMarketsResponse{Markets: []*v2.FullDerivativeMarket{}}, nil
	}

	fullMarkets := make([]*v2.FullDerivativeMarket, 0)
	filters := make([]utils.FilterFn[v2.MarketI], 0)
	filters = append(filters, utils.MarketStatusFilter(status))
	if len(req.GetMarketIds()) > 0 {
		filters = append(filters, utils.MarketIDFilter(req.GetMarketIds()...))
	}

	theFilter := utils.ChainFilters(filters...)

	q.Keeper.IterateDerivativeMarkets(ctx, nil, func(m *v2.DerivativeMarket) (stop bool) {
		if !theFilter(m) {
			return false
		}

		fullMarket := &v2.FullDerivativeMarket{Market: m}
		markPrice, err := q.Keeper.GetDerivativeMarketPrice(ctx, m.OracleBase, m.OracleQuote, m.OracleScaleFactor, m.OracleType)
		if err != nil {
			fullMarket.MarkPrice = math.LegacyDec{}
		} else {
			fullMarket.MarkPrice = *markPrice
		}

		if fullMarket.GetMarket().IsPerpetual {
			fullMarket.Info = &v2.FullDerivativeMarket_PerpetualInfo{
				PerpetualInfo: &v2.PerpetualMarketState{
					MarketInfo:  q.Keeper.GetPerpetualMarketInfo(ctx, m.MarketID()),
					FundingInfo: q.Keeper.GetPerpetualMarketFunding(ctx, m.MarketID()),
				},
			}
		} else {
			fullMarket.Info = &v2.FullDerivativeMarket_FuturesInfo{
				FuturesInfo: q.Keeper.GetExpiryFuturesMarketInfo(ctx, m.MarketID()),
			}
		}

		midPrice, bestBuy, bestSell := q.Keeper.GetDerivativeMidPriceAndTOB(ctx, fullMarket.GetMarket().MarketID())
		fullMarket.MidPriceAndTob = &v2.MidPriceAndTOB{
			MidPrice:      midPrice,
			BestBuyPrice:  bestBuy,
			BestSellPrice: bestSell,
		}

		fullMarkets = append(fullMarkets, fullMarket)

		return false
	})

	resp := &v2.QueryDerivativeMarketsResponse{
		Markets: fullMarkets,
	}

	return resp, nil
}

func (q queryServer) DerivativeMarket(c context.Context, req *v2.QueryDerivativeMarketRequest) (*v2.QueryDerivativeMarketResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "DerivativeMarket")()

	marketID := common.HexToHash(req.MarketId)
	market := q.Keeper.GetFullDerivativeMarket(ctx, marketID, true)
	if market == nil {
		return nil, types.ErrDerivativeMarketNotFound
	}

	resp := &v2.QueryDerivativeMarketResponse{
		Market: market,
	}

	return resp, nil
}

func (q queryServer) DerivativeMarketAddress(
	c context.Context, req *v2.QueryDerivativeMarketAddressRequest,
) (*v2.QueryDerivativeMarketAddressResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "DerivativeMarketAddress")()

	marketID := common.HexToHash(req.MarketId)

	resp := &v2.QueryDerivativeMarketAddressResponse{
		Address:      types.SubaccountIDToSdkAddress(marketID).String(),
		SubaccountId: types.SdkAddressToSubaccountID(types.SubaccountIDToSdkAddress(marketID)).String(),
	}

	return resp, nil
}

func (q queryServer) SubaccountTradeNonce(
	c context.Context, req *v2.QuerySubaccountTradeNonceRequest,
) (*v2.QuerySubaccountTradeNonceResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "SubaccountTradeNonce")()

	resp := &v2.QuerySubaccountTradeNonceResponse{
		Nonce: q.Keeper.GetSubaccountTradeNonce(ctx, common.HexToHash(req.SubaccountId)).Nonce,
	}

	return resp, nil
}

func (q queryServer) ExchangeModuleState(c context.Context, _ *v2.QueryModuleStateRequest) (*v2.QueryModuleStateResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "ExchangeModuleState")()

	resp := &v2.QueryModuleStateResponse{
		State: q.Keeper.ExportGenesis(ctx),
	}

	return resp, nil
}

func (q queryServer) Positions(c context.Context, _ *v2.QueryPositionsRequest) (*v2.QueryPositionsResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "Positions")()

	resp := &v2.QueryPositionsResponse{
		State: q.Keeper.GetAllPositions(ctx),
	}

	return resp, nil
}

func (q queryServer) SubaccountPositions(
	c context.Context, req *v2.QuerySubaccountPositionsRequest,
) (*v2.QuerySubaccountPositionsResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "SubaccountPositions")()

	subaccountID := common.HexToHash(req.SubaccountId)

	resp := &v2.QuerySubaccountPositionsResponse{
		State: q.Keeper.GetAllActivePositionsBySubaccountID(ctx, subaccountID),
	}

	return resp, nil
}

func (q queryServer) SubaccountPositionInMarket(
	c context.Context, req *v2.QuerySubaccountPositionInMarketRequest,
) (*v2.QuerySubaccountPositionInMarketResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "SubaccountPositionInMarket")()

	marketID := common.HexToHash(req.MarketId)
	subaccountID := common.HexToHash(req.SubaccountId)

	profile, _ := q.Keeper.GetEffectiveSubaccountRiskProfile(ctx, subaccountID)

	resp := &v2.QuerySubaccountPositionInMarketResponse{
		State:    q.Keeper.GetPosition(ctx, marketID, subaccountID),
		RiskMode: profile.Mode,
	}

	return resp, nil
}

func (q queryServer) SubaccountEffectivePositionInMarket(
	c context.Context, req *v2.QuerySubaccountEffectivePositionInMarketRequest,
) (*v2.QuerySubaccountEffectivePositionInMarketResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "SubaccountEffectivePositionInMarket")()

	marketID := common.HexToHash(req.MarketId)
	subaccountID := common.HexToHash(req.SubaccountId)
	position := q.Keeper.GetPosition(ctx, marketID, subaccountID)

	profile, _ := q.Keeper.GetEffectiveSubaccountRiskProfile(ctx, subaccountID)

	if position == nil {
		return &v2.QuerySubaccountEffectivePositionInMarketResponse{
			State:    nil,
			RiskMode: profile.Mode,
		}, nil
	}

	funding := q.Keeper.GetPerpetualMarketFunding(ctx, marketID)
	_, markPrice := q.Keeper.GetDerivativeOrBinaryOptionsMarketWithMarkPrice(ctx, marketID, true)

	effectivePosition := v2.EffectivePosition{
		IsLong:          position.IsLong,
		EntryPrice:      position.EntryPrice,
		Quantity:        position.Quantity,
		EffectiveMargin: position.GetEffectiveMargin(funding, markPrice),
	}

	resp := &v2.QuerySubaccountEffectivePositionInMarketResponse{
		State:    &effectivePosition,
		RiskMode: profile.Mode,
	}

	return resp, nil
}

func (q queryServer) PerpetualMarketInfo(
	c context.Context, req *v2.QueryPerpetualMarketInfoRequest,
) (*v2.QueryPerpetualMarketInfoResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "PerpetualMarketInfo")()

	if req.MarketId == "" {
		return nil, errors.New("MarketId must be specified")
	}

	info := q.Keeper.GetPerpetualMarketInfo(ctx, common.HexToHash(req.MarketId))
	if info == nil {
		return nil, fmt.Errorf("market info for marketId %s doesn't exist", req.MarketId)
	}

	res := &v2.QueryPerpetualMarketInfoResponse{
		Info: *info,
	}

	return res, nil
}

func (q queryServer) ExpiryFuturesMarketInfo(
	c context.Context, req *v2.QueryExpiryFuturesMarketInfoRequest,
) (*v2.QueryExpiryFuturesMarketInfoResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "ExpiryFuturesMarketInfo")()

	if req.MarketId == "" {
		return nil, errors.New("MarketId must be specified")
	}

	info := q.Keeper.GetExpiryFuturesMarketInfo(ctx, common.HexToHash(req.MarketId))
	if info == nil {
		return nil, fmt.Errorf("market info for marketId %s doesn't exist", req.MarketId)
	}

	resp := &v2.QueryExpiryFuturesMarketInfoResponse{
		Info: *info,
	}

	return resp, nil
}

func (q queryServer) PerpetualMarketFunding(
	c context.Context, req *v2.QueryPerpetualMarketFundingRequest,
) (*v2.QueryPerpetualMarketFundingResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "PerpetualMarketFunding")()

	if req.MarketId == "" {
		return nil, errors.New("MarketId must be specified")
	}

	state := q.Keeper.GetPerpetualMarketFunding(ctx, common.HexToHash(req.MarketId))
	if state == nil {
		return nil, fmt.Errorf("market info for marketId %s doesn't exist", req.MarketId)
	}

	resp := &v2.QueryPerpetualMarketFundingResponse{
		State: *state,
	}

	return resp, nil
}

func (q queryServer) SubaccountOrderMetadata(
	c context.Context, req *v2.QuerySubaccountOrderMetadataRequest,
) (*v2.QuerySubaccountOrderMetadataResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "SubaccountOrderMetadata")()

	derivativeMarkets := q.Keeper.GetAllDerivativeMarkets(ctx)
	binaryOptionsMarkets := q.Keeper.GetAllBinaryOptionsMarkets(ctx)
	markets := make([]v2.DerivativeMarketI, 0, len(derivativeMarkets)+len(binaryOptionsMarkets))

	for _, m := range derivativeMarkets {
		markets = append(markets, m)
	}

	for _, m := range binaryOptionsMarkets {
		markets = append(markets, m)
	}

	subaccountOrderbookMetadata := make([]v2.SubaccountOrderbookMetadataWithMarket, 0)
	for _, market := range markets {
		subaccountOrderbookMetadata = append(subaccountOrderbookMetadata,
			v2.SubaccountOrderbookMetadataWithMarket{
				Metadata: q.Keeper.GetSubaccountOrderbookMetadata(ctx, market.MarketID(), common.HexToHash(req.SubaccountId), true),
				MarketId: market.MarketID().String(),
				IsBuy:    true,
			},
			v2.SubaccountOrderbookMetadataWithMarket{
				Metadata: q.Keeper.GetSubaccountOrderbookMetadata(ctx, market.MarketID(), common.HexToHash(req.SubaccountId), false),
				MarketId: market.MarketID().String(),
				IsBuy:    false,
			},
		)
	}

	resp := &v2.QuerySubaccountOrderMetadataResponse{
		Metadata: subaccountOrderbookMetadata,
	}

	return resp, nil
}

func (q queryServer) TradeRewardPoints(
	c context.Context, req *v2.QueryTradeRewardPointsRequest,
) (*v2.QueryTradeRewardPointsResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "TradeRewardPoints")()

	accounts := make([]sdk.AccAddress, 0, len(req.Accounts))
	for _, accountStr := range req.Accounts {
		account, err := sdk.AccAddressFromBech32(accountStr)
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, account)
	}

	accountPoints := make([]math.LegacyDec, 0, len(accounts))

	for _, account := range accounts {
		points := q.Keeper.GetCampaignTradingRewardPoints(ctx, account)
		accountPoints = append(accountPoints, points)
	}

	resp := &v2.QueryTradeRewardPointsResponse{
		AccountTradeRewardPoints: accountPoints,
	}

	return resp, nil
}

func (q queryServer) PendingTradeRewardPoints(
	c context.Context, req *v2.QueryTradeRewardPointsRequest,
) (*v2.QueryTradeRewardPointsResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "PendingTradeRewardPoints")()

	accounts := make([]sdk.AccAddress, 0, len(req.Accounts))
	for _, accountStr := range req.Accounts {
		account, err := sdk.AccAddressFromBech32(accountStr)
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, account)
	}

	accountPoints := make([]math.LegacyDec, 0, len(accounts))

	for _, account := range accounts {
		points := q.Keeper.GetCampaignTradingRewardPendingPoints(ctx, account, req.PendingPoolTimestamp)
		accountPoints = append(accountPoints, points)
	}

	resp := &v2.QueryTradeRewardPointsResponse{
		AccountTradeRewardPoints: accountPoints,
	}

	return resp, nil
}

func (q queryServer) TradeRewardCampaign(
	c context.Context, _ *v2.QueryTradeRewardCampaignRequest,
) (*v2.QueryTradeRewardCampaignResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "TradeRewardCampaign")()

	resp := &v2.QueryTradeRewardCampaignResponse{
		TradingRewardCampaignInfo:                q.Keeper.GetCampaignInfo(ctx),
		TradingRewardPoolCampaignSchedule:        q.Keeper.GetAllCampaignRewardPools(ctx),
		TotalTradeRewardPoints:                   q.Keeper.GetTotalTradingRewardPoints(ctx),
		PendingTradingRewardPoolCampaignSchedule: q.Keeper.GetAllCampaignRewardPendingPools(ctx),
		PendingTotalTradeRewardPoints:            make([]math.LegacyDec, 0),
	}

	for _, campaign := range resp.PendingTradingRewardPoolCampaignSchedule {
		totalPoints := q.Keeper.GetTotalTradingRewardPendingPoints(ctx, campaign.StartTimestamp)
		resp.PendingTotalTradeRewardPoints = append(resp.PendingTotalTradeRewardPoints, totalPoints)
	}

	return resp, nil
}

func (q queryServer) FeeDiscountAccountInfo(
	c context.Context, req *v2.QueryFeeDiscountAccountInfoRequest,
) (*v2.QueryFeeDiscountAccountInfoResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "FeeDiscountAccountInfo")()

	account, err := sdk.AccAddressFromBech32(req.Account)
	if err != nil {
		return nil, err
	}

	schedule := q.Keeper.GetFeeDiscountSchedule(ctx)
	if schedule == nil {
		return nil, types.ErrInvalidFeeDiscountSchedule
	}

	currBucketStartTimestamp := q.Keeper.GetFeeDiscountCurrentBucketStartTimestamp(ctx)
	oldestBucketStartTimestamp := q.Keeper.GetOldestBucketStartTimestamp(ctx)
	isFirstFeeCycleFinished := q.Keeper.GetIsFirstFeeCycleFinished(ctx)
	maxTTLTimestamp := currBucketStartTimestamp
	nextTTLTimestamp := maxTTLTimestamp + q.Keeper.GetFeeDiscountBucketDuration(ctx)

	stakingInfo := v2.NewFeeDiscountStakingInfo(
		schedule,
		currBucketStartTimestamp,
		oldestBucketStartTimestamp,
		maxTTLTimestamp,
		nextTTLTimestamp,
		isFirstFeeCycleFinished,
	)

	config := v2.NewFeeDiscountConfig(true, stakingInfo)
	feeDiscountRates, tierLevel, _, effectiveGrant := q.Keeper.GetAccountFeeDiscountRates(ctx, account, config)
	effectiveStakedAmount := q.Keeper.CalculateStakedAmountWithCache(ctx, account, config).Add(effectiveGrant.NetGrantedStake)

	volume := q.Keeper.GetFeeDiscountTotalAccountVolume(ctx, account, currBucketStartTimestamp)
	feeDiscountTierTTL := q.Keeper.GetFeeDiscountAccountTierInfo(ctx, account)

	resp := &v2.QueryFeeDiscountAccountInfoResponse{
		TierLevel: tierLevel,
		AccountInfo: &v2.FeeDiscountTierInfo{
			MakerDiscountRate: feeDiscountRates.MakerDiscountRate,
			TakerDiscountRate: feeDiscountRates.TakerDiscountRate,
			StakedAmount:      effectiveStakedAmount,
			Volume:            volume,
		},
		AccountTtl: feeDiscountTierTTL,
	}

	return resp, nil
}

func (q queryServer) FeeDiscountSchedule(
	c context.Context, _ *v2.QueryFeeDiscountScheduleRequest,
) (*v2.QueryFeeDiscountScheduleResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "FeeDiscountSchedule")()

	resp := &v2.QueryFeeDiscountScheduleResponse{
		FeeDiscountSchedule: q.Keeper.GetFeeDiscountSchedule(ctx),
	}

	return resp, nil
}

func (q queryServer) BalanceMismatches(
	c context.Context, req *v2.QueryBalanceMismatchesRequest,
) (*v2.QueryBalanceMismatchesResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "BalanceMismatches")()

	balancesWithBalanceHolds := q.Keeper.GetAllBalancesWithBalanceHolds(ctx)
	balanceMismatches := make([]*v2.BalanceMismatch, 0)

	for _, balanceWithBalanceHold := range balancesWithBalanceHolds {
		balanceHold := balanceWithBalanceHold.BalanceHold
		expectedTotalBalance := balanceWithBalanceHold.Available.Add(balanceHold)

		isMatching := expectedTotalBalance.Sub(balanceWithBalanceHold.Total).Abs().LT(math.LegacySmallestDec().MulInt64(req.DustFactor))

		if !isMatching {
			balanceMismatches = append(balanceMismatches, &v2.BalanceMismatch{
				SubaccountId:  balanceWithBalanceHold.SubaccountId,
				Denom:         balanceWithBalanceHold.Denom,
				Available:     balanceWithBalanceHold.Available,
				Total:         balanceWithBalanceHold.Total,
				BalanceHold:   balanceHold,
				ExpectedTotal: expectedTotalBalance,
				Difference:    expectedTotalBalance.Sub(balanceWithBalanceHold.Total),
			})
		}
	}

	resp := &v2.QueryBalanceMismatchesResponse{
		BalanceMismatches: balanceMismatches,
	}

	return resp, nil
}

func (q queryServer) BalanceWithBalanceHolds(
	c context.Context, _ *v2.QueryBalanceWithBalanceHoldsRequest,
) (*v2.QueryBalanceWithBalanceHoldsResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "BalanceWithBalanceHolds")()

	resp := &v2.QueryBalanceWithBalanceHoldsResponse{
		BalanceWithBalanceHolds: q.Keeper.GetAllBalancesWithBalanceHolds(ctx),
	}

	return resp, nil
}

func (q queryServer) FeeDiscountTierStatistics(
	c context.Context, _ *v2.QueryFeeDiscountTierStatisticsRequest,
) (*v2.QueryFeeDiscountTierStatisticsResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "FeeDiscountTierStatistics")()

	var (
		tierCount      = len(q.Keeper.GetFeeDiscountSchedule(ctx).TierInfos)
		tierStatistics = make(map[uint64]uint64)
		statistics     = make([]*v2.TierStatistic, tierCount)
	)

	for i := 0; i < tierCount; i++ {
		tierStatistics[uint64(i)] = 0
	}

	accountTierInfos := q.Keeper.GetAllFeeDiscountAccountTierInfo(ctx)
	for _, accountTierInfo := range accountTierInfos {
		tierStatistics[accountTierInfo.TierTtl.Tier]++
	}

	for i := 0; i < tierCount; i++ {
		statistics[i] = &v2.TierStatistic{
			Tier:  uint64(i),
			Count: tierStatistics[uint64(i)],
		}
	}

	resp := &v2.QueryFeeDiscountTierStatisticsResponse{
		Statistics: statistics,
	}

	return resp, nil
}

func (q queryServer) MitoVaultInfos(
	c context.Context, _ *v2.MitoVaultInfosRequest,
) (*v2.MitoVaultInfosResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "MitoVaultInfos")()

	var (
		derivativeContractAddresses []string
		masterContractAddresses     []string
		cw20ContractAddresses       []string
		spotContractAddresses       []string
	)

	// TODO fix me, how to find out code ids?
	derivativeCodeID := uint64(2)
	masterCodeID := uint64(3)
	cw20CodeID := uint64(4)
	spotCodeID := uint64(5)

	q.Keeper.wasmViewKeeper.IterateContractsByCode(ctx, derivativeCodeID, func(addr sdk.AccAddress) bool {
		derivativeContractAddresses = append(derivativeContractAddresses, addr.String())
		return false
	})
	q.Keeper.wasmViewKeeper.IterateContractsByCode(ctx, masterCodeID, func(addr sdk.AccAddress) bool {
		masterContractAddresses = append(masterContractAddresses, addr.String())
		return false
	})
	q.Keeper.wasmViewKeeper.IterateContractsByCode(ctx, cw20CodeID, func(addr sdk.AccAddress) bool {
		cw20ContractAddresses = append(cw20ContractAddresses, addr.String())
		return false
	})
	q.Keeper.wasmViewKeeper.IterateContractsByCode(ctx, spotCodeID, func(addr sdk.AccAddress) bool {
		spotContractAddresses = append(spotContractAddresses, addr.String())
		return false
	})

	resp := &v2.MitoVaultInfosResponse{
		MasterAddresses:     masterContractAddresses,
		DerivativeAddresses: derivativeContractAddresses,
		SpotAddresses:       spotContractAddresses,
		Cw20Addresses:       cw20ContractAddresses,
	}

	return resp, nil
}

func (q queryServer) QueryMarketIDFromVault(
	c context.Context, req *v2.QueryMarketIDFromVaultRequest,
) (*v2.QueryMarketIDFromVaultResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "QueryMarketIDFromVault")()

	marketID, err := q.Keeper.QueryMarketID(ctx, req.VaultAddress)
	if err != nil {
		return nil, err
	}

	resp := &v2.QueryMarketIDFromVaultResponse{
		MarketId: marketID.Hex(),
	}

	return resp, nil
}

func (q queryServer) HistoricalTradeRecords(
	c context.Context, req *v2.QueryHistoricalTradeRecordsRequest,
) (*v2.QueryHistoricalTradeRecordsResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "HistoricalTradeRecords")()

	resp := &v2.QueryHistoricalTradeRecordsResponse{}

	if req.MarketId != "" {
		records, _ := q.Keeper.GetHistoricalTradeRecords(ctx, common.HexToHash(req.MarketId), 0)
		resp.TradeRecords = []*v2.TradeRecords{records}
	} else {
		resp.TradeRecords = q.Keeper.GetAllHistoricalTradeRecords(ctx)
	}

	return resp, nil
}

func (q queryServer) IsOptedOutOfRewards(
	c context.Context, req *v2.QueryIsOptedOutOfRewardsRequest,
) (*v2.QueryIsOptedOutOfRewardsResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "IsOptedOutOfRewards")()

	account, err := sdk.AccAddressFromBech32(req.Account)
	if err != nil {
		return nil, err
	}

	resp := &v2.QueryIsOptedOutOfRewardsResponse{
		IsOptedOut: q.Keeper.GetIsOptedOutOfRewards(ctx, account),
	}

	return resp, nil
}

func (q queryServer) OptedOutOfRewardsAccounts(
	c context.Context, _ *v2.QueryOptedOutOfRewardsAccountsRequest,
) (*v2.QueryOptedOutOfRewardsAccountsResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "OptedOutOfRewardsAccounts")()

	resp := &v2.QueryOptedOutOfRewardsAccountsResponse{
		Accounts: q.Keeper.GetAllOptedOutRewardAccounts(ctx),
	}

	return resp, nil
}

func (q queryServer) MarketVolatility(
	c context.Context, req *v2.QueryMarketVolatilityRequest,
) (*v2.QueryMarketVolatilityResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "MarketVolatility")()

	vol, rawHistory, meta := q.Keeper.GetMarketVolatility(
		ctx,
		common.HexToHash(req.MarketId),
		req.TradeHistoryOptions,
	)

	resp := &v2.QueryMarketVolatilityResponse{
		Volatility:      vol,
		HistoryMetadata: meta,
		RawHistory:      rawHistory,
	}

	return resp, nil
}

func (q queryServer) BinaryOptionsMarkets(
	c context.Context, req *v2.QueryBinaryMarketsRequest,
) (*v2.QueryBinaryMarketsResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "BinaryOptionsMarkets")()

	m := q.Keeper.GetAllBinaryOptionsMarkets(ctx)

	var status v2.MarketStatus
	if req.Status == "" {
		status = v2.MarketStatus_Active
	} else {
		status = v2.MarketStatus(v2.MarketStatus_value[req.Status])
	}

	markets := make([]*v2.BinaryOptionsMarket, 0, len(m))
	if status != v2.MarketStatus_Unspecified {
		for _, market := range m {
			if market.Status == status {
				markets = append(markets, market)
			}
		}
	}

	resp := &v2.QueryBinaryMarketsResponse{
		Markets: markets,
	}

	return resp, nil
}

func (q queryServer) TraderDerivativeConditionalOrders(
	c context.Context, req *v2.QueryTraderDerivativeConditionalOrdersRequest,
) (*v2.QueryTraderDerivativeConditionalOrdersResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "TraderDerivativeConditionalOrders")()

	marketID := common.HexToHash(req.MarketId)
	subaccountID := common.HexToHash(req.SubaccountId)

	resp := &v2.QueryTraderDerivativeConditionalOrdersResponse{
		Orders: q.Keeper.GetAllSubaccountConditionalOrders(ctx, marketID, subaccountID),
	}

	return resp, nil
}

func (q queryServer) MarketAtomicExecutionFeeMultiplier(
	c context.Context, req *v2.QueryMarketAtomicExecutionFeeMultiplierRequest,
) (*v2.QueryMarketAtomicExecutionFeeMultiplierResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "MarketAtomicExecutionFeeMultiplier")()

	marketID := common.HexToHash(req.MarketId)
	marketType, err := q.Keeper.GetMarketType(ctx, marketID, true)
	if err != nil {
		return nil, err
	}

	resp := v2.QueryMarketAtomicExecutionFeeMultiplierResponse{
		Multiplier: q.Keeper.GetMarketAtomicExecutionFeeMultiplier(ctx, marketID, *marketType),
	}

	return &resp, nil
}

func (q queryServer) ActiveStakeGrant(
	c context.Context, req *v2.QueryActiveStakeGrantRequest,
) (*v2.QueryActiveStakeGrantResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "ActiveStakeGrant")()

	grantee, err := sdk.AccAddressFromBech32(req.Grantee)
	if err != nil {
		return nil, err
	}

	resp := &v2.QueryActiveStakeGrantResponse{
		Grant:          q.Keeper.GetActiveGrant(ctx, grantee),
		EffectiveGrant: q.Keeper.GetValidatedEffectiveGrant(ctx, grantee),
	}

	return resp, nil
}

func (q queryServer) GrantAuthorization(
	c context.Context, req *v2.QueryGrantAuthorizationRequest,
) (*v2.QueryGrantAuthorizationResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "GrantAuthorization")()

	granter, err := sdk.AccAddressFromBech32(req.Granter)
	if err != nil {
		return nil, err
	}

	grantee, err := sdk.AccAddressFromBech32(req.Grantee)
	if err != nil {
		return nil, err
	}

	resp := &v2.QueryGrantAuthorizationResponse{
		Amount: q.Keeper.GetGrantAuthorization(ctx, granter, grantee),
	}

	return resp, nil
}

func (q queryServer) GrantAuthorizations(
	c context.Context, req *v2.QueryGrantAuthorizationsRequest,
) (*v2.QueryGrantAuthorizationsResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "GrantAuthorizations")()

	granter, err := sdk.AccAddressFromBech32(req.Granter)
	if err != nil {
		return nil, err
	}

	resp := &v2.QueryGrantAuthorizationsResponse{
		TotalGrantAmount: q.Keeper.GetTotalGrantAmount(ctx, granter),
		Grants:           q.Keeper.GetAllGranterAuthorizations(ctx, granter),
	}

	return resp, nil
}

func (q queryServer) MarketBalance(
	c context.Context, req *v2.QueryMarketBalanceRequest,
) (*v2.QueryMarketBalanceResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "MarketBalance")()

	marketID := common.HexToHash(req.MarketId)

	res := &v2.QueryMarketBalanceResponse{
		Balance: &v2.MarketBalance{
			MarketId: req.MarketId,
			Balance:  q.Keeper.GetMarketBalance(ctx, marketID),
		},
	}

	return res, nil
}

func (q queryServer) MarketBalances(
	c context.Context, _ *v2.QueryMarketBalancesRequest,
) (*v2.QueryMarketBalancesResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "MarketBalances")()

	res := &v2.QueryMarketBalancesResponse{
		Balances: q.Keeper.GetAllMarketBalances(ctx),
	}
	return res, nil
}

func (q queryServer) DenomMinNotional(
	c context.Context, req *v2.QueryDenomMinNotionalRequest,
) (*v2.QueryDenomMinNotionalResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "DenomMinNotional")()

	if req.Denom == "" {
		return nil, errors.New("denom is required")
	}

	res := &v2.QueryDenomMinNotionalResponse{
		Amount: q.Keeper.GetMinNotionalForDenom(ctx, req.Denom),
	}

	return res, nil
}

func (q queryServer) DenomMinNotionals(
	c context.Context, _ *v2.QueryDenomMinNotionalsRequest,
) (*v2.QueryDenomMinNotionalsResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "DenomMinNotionals")()

	res := &v2.QueryDenomMinNotionalsResponse{
		DenomMinNotionals: q.Keeper.GetAllDenomMinNotionals(ctx),
	}

	return res, nil
}

func (q queryServer) OpenInterest(
	c context.Context, req *v2.QueryOpenInterestRequest,
) (*v2.QueryOpenInterestResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer q.Keeper.Meter(ctx).FuncTiming(&ctx, "OpenInterest")()

	marketID := common.HexToHash(req.MarketId)

	res := &v2.QueryOpenInterestResponse{
		Amount: &v2.OpenInterest{
			MarketId: req.MarketId,
			Balance:  q.Keeper.GetOpenInterestForMarket(ctx, marketID),
		},
	}

	return res, nil
}
