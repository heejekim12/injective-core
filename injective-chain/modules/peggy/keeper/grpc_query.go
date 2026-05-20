package keeper

import (
	"context"
	"sort"
	"strings"

	"cosmossdk.io/errors"
	"cosmossdk.io/store/prefix"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	gethcommon "github.com/ethereum/go-ethereum/common"

	"github.com/InjectiveLabs/injective-core/injective-chain/modules/peggy/types"
	chaintypes "github.com/InjectiveLabs/injective-core/injective-chain/types"
)

var _ types.QueryServer = &Keeper{}

const maxValsetRequestsReturned = 5
const MaxResults = 100 // todo: impl pagination

// Params queries the params of the peggy module
func (k *Keeper) Params(c context.Context, _ *types.QueryParamsRequest) (*types.QueryParamsResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer k.Meter(ctx).FuncTiming(&ctx, "Params")()

	return &types.QueryParamsResponse{Params: *k.GetParams(ctx)}, nil
}

// CurrentValset queries the CurrentValset of the peggy module
func (k *Keeper) CurrentValset(c context.Context, req *types.QueryCurrentValsetRequest) (*types.QueryCurrentValsetResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer k.Meter(ctx).FuncTiming(&ctx, "CurrentValset")()

	return &types.QueryCurrentValsetResponse{Valset: k.GetCurrentValset(ctx)}, nil
}

// ValsetRequest queries the ValsetRequest of the peggy module
func (k *Keeper) ValsetRequest(c context.Context, req *types.QueryValsetRequestRequest) (*types.QueryValsetRequestResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer k.Meter(ctx).FuncTiming(&ctx, "ValsetRequest")()

	return &types.QueryValsetRequestResponse{Valset: k.GetValset(ctx, req.Nonce)}, nil
}

// ValsetConfirm queries the ValsetConfirm of the peggy module
func (k *Keeper) ValsetConfirm(c context.Context, req *types.QueryValsetConfirmRequest) (*types.QueryValsetConfirmResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer k.Meter(ctx).FuncTiming(&ctx, "ValsetConfirm")()

	addr, err := sdk.AccAddressFromBech32(req.Address)
	if err != nil {
		return nil, errors.Wrap(sdkerrors.ErrInvalidRequest, "address invalid")
	}

	return &types.QueryValsetConfirmResponse{Confirm: k.GetValsetConfirm(ctx, req.Nonce, addr)}, nil
}

// ValsetConfirmsByNonce queries the ValsetConfirmsByNonce of the peggy module
func (k *Keeper) ValsetConfirmsByNonce(c context.Context, req *types.QueryValsetConfirmsByNonceRequest) (*types.QueryValsetConfirmsByNonceResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer k.Meter(ctx).FuncTiming(&ctx, "ValsetConfirmsByNonce")()

	confirms := make([]*types.MsgValsetConfirm, 0)

	k.IterateValsetConfirmByNonce(ctx, req.Nonce, func(_ []byte, valset *types.MsgValsetConfirm) (stop bool) {
		confirms = append(confirms, valset)

		return false
	})

	return &types.QueryValsetConfirmsByNonceResponse{Confirms: confirms}, nil
}

// LastValsetRequests queries the LastValsetRequests of the peggy module
func (k *Keeper) LastValsetRequests(c context.Context, req *types.QueryLastValsetRequestsRequest) (*types.QueryLastValsetRequestsResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer k.Meter(ctx).FuncTiming(&ctx, "LastValsetRequests")()

	valReq := k.GetValsets(ctx)
	valReqLen := len(valReq)
	retLen := 0

	if valReqLen < maxValsetRequestsReturned {
		retLen = valReqLen
	} else {
		retLen = maxValsetRequestsReturned
	}

	return &types.QueryLastValsetRequestsResponse{Valsets: valReq[0:retLen]}, nil
}

// LastPendingValsetRequestByAddr queries the LastPendingValsetRequestByAddr of the peggy module
func (k *Keeper) LastPendingValsetRequestByAddr(c context.Context, req *types.QueryLastPendingValsetRequestByAddrRequest) (*types.QueryLastPendingValsetRequestByAddrResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer k.Meter(ctx).FuncTiming(&ctx, "LastPendingValsetRequestByAddr")()

	addr, err := sdk.AccAddressFromBech32(req.Address)
	if err != nil {
		return nil, errors.Wrap(sdkerrors.ErrInvalidRequest, "address invalid")
	}

	pendingValsetReq := make([]*types.Valset, 0)
	k.IterateValsets(ctx, func(_ []byte, val *types.Valset) bool {
		// foundConfirm is true if the operatorAddr has signed the valset we are currently looking at
		foundConfirm := k.GetValsetConfirm(ctx, val.Nonce, addr) != nil
		// if this valset has NOT been signed by operatorAddr, store it in pendingValsetReq
		// and exit the loop
		if !foundConfirm {
			pendingValsetReq = append(pendingValsetReq, val)
		}
		// if we have more than 100 unconfirmed requests in
		// our array we should exit, TODO pagination
		if len(pendingValsetReq) > 100 {
			return true
		}
		// return false to continue the loop
		return false
	})

	return &types.QueryLastPendingValsetRequestByAddrResponse{Valsets: pendingValsetReq}, nil
}

// BatchFees queries the batch fees from unbatched pool
func (k *Keeper) BatchFees(c context.Context, req *types.QueryBatchFeeRequest) (*types.QueryBatchFeeResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer k.Meter(ctx).FuncTiming(&ctx, "BatchFees")()

	return &types.QueryBatchFeeResponse{BatchFees: k.GetAllBatchFees(ctx)}, nil
}

// LastPendingBatchRequestByAddr queries the LastPendingBatchRequestByAddr of the peggy module
func (k *Keeper) LastPendingBatchRequestByAddr(c context.Context, req *types.QueryLastPendingBatchRequestByAddrRequest) (*types.QueryLastPendingBatchRequestByAddrResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer k.Meter(ctx).FuncTiming(&ctx, "LastPendingBatchRequestByAddr")()

	addr, err := sdk.AccAddressFromBech32(req.Address)
	if err != nil {
		return nil, errors.Wrap(sdkerrors.ErrInvalidRequest, "address invalid")
	}

	var oldestBatch *types.OutgoingTxBatch
	batchStore := prefix.NewStore(k.getStore(ctx), types.OutgoingTXBatchKey)
	chaintypes.IterateSafe(batchStore.Iterator(nil, nil), func(_, value []byte) (stop bool) {
		var batch types.OutgoingTxBatch
		k.cdc.MustUnmarshal(value, &batch)

		token := gethcommon.HexToAddress(batch.TokenContract)
		if confirm := k.GetBatchConfirm(ctx, batch.BatchNonce, token, addr); confirm == nil {
			// no confirm yet
			oldestBatch = &batch
			return true // stop iterating
		}

		return false
	})

	return &types.QueryLastPendingBatchRequestByAddrResponse{Batch: oldestBatch}, nil
}

// OutgoingTxBatches queries the OutgoingTxBatches of the peggy module
func (k *Keeper) OutgoingTxBatches(c context.Context, req *types.QueryOutgoingTxBatchesRequest) (*types.QueryOutgoingTxBatchesResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer k.Meter(ctx).FuncTiming(&ctx, "OutgoingTxBatches")()

	batches := make([]*types.OutgoingTxBatch, 0)
	k.IterateOutgoingTXBatches(ctx, func(_ []byte, batch *types.OutgoingTxBatch) bool {
		batches = append(batches, batch)
		return len(batches) == MaxResults
	})

	return &types.QueryOutgoingTxBatchesResponse{Batches: batches}, nil
}

// BatchRequestByNonce queries the BatchRequestByNonce of the peggy module
func (k *Keeper) BatchRequestByNonce(c context.Context, req *types.QueryBatchRequestByNonceRequest) (*types.QueryBatchRequestByNonceResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer k.Meter(ctx).FuncTiming(&ctx, "BatchRequestByNonce")()

	if err := types.ValidateEthAddress(req.ContractAddress); err != nil {
		return nil, errors.Wrap(sdkerrors.ErrUnknownRequest, err.Error())
	}

	foundBatch := k.GetOutgoingTXBatch(ctx, gethcommon.HexToAddress(req.ContractAddress), req.Nonce)
	if foundBatch == nil {
		return nil, errors.Wrap(sdkerrors.ErrUnknownRequest, "Can not find tx batch")
	}

	return &types.QueryBatchRequestByNonceResponse{Batch: foundBatch}, nil
}

// BatchConfirms returns the batch confirmations by nonce and token contract
func (k *Keeper) BatchConfirms(c context.Context, req *types.QueryBatchConfirmsRequest) (*types.QueryBatchConfirmsResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer k.Meter(ctx).FuncTiming(&ctx, "BatchConfirms")()

	confirms := make([]*types.MsgConfirmBatch, 0)
	k.IterateBatchConfirmByNonceAndTokenContract(ctx, req.Nonce, gethcommon.HexToAddress(req.ContractAddress),
		func(_ []byte, batch *types.MsgConfirmBatch) (stop bool) {
			confirms = append(confirms, batch)
			return false
		})

	return &types.QueryBatchConfirmsResponse{Confirms: confirms}, nil
}

// LastEventByAddr returns the last event for the given validator address, this allows eth oracles to figure out where they left off
func (k *Keeper) LastEventByAddr(c context.Context, req *types.QueryLastEventByAddrRequest) (*types.QueryLastEventByAddrResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer k.Meter(ctx).FuncTiming(&ctx, "LastEventByAddr")()

	var ret types.QueryLastEventByAddrResponse

	addr, err := sdk.AccAddressFromBech32(req.Address)
	if err != nil {
		return nil, errors.Wrap(sdkerrors.ErrInvalidAddress, req.Address)
	}

	validator, found := k.GetOrchestratorValidator(ctx, addr)
	if !found {
		return nil, errors.Wrap(types.ErrUnknown, "address")
	}

	lastClaimEvent := k.GetLastEventByValidator(ctx, validator)
	if lastClaimEvent.EthereumEventNonce == 0 && lastClaimEvent.EthereumEventHeight == 0 {
		// if peggo happens to query too early without a bonded validator even existing
		return nil, errors.Wrapf(types.ErrNoLastClaimForValidator, "ensure validator has bonded: validator=%v", validator.String())
	}

	ret.LastClaimEvent = &lastClaimEvent

	return &ret, nil
}

// DenomToERC20 queries the Cosmos Denom that maps to an Ethereum ERC20
func (k *Keeper) DenomToERC20(c context.Context, req *types.QueryDenomToERC20Request) (*types.QueryDenomToERC20Response, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer k.Meter(ctx).FuncTiming(&ctx, "DenomToERC20")()

	cosmosOriginated, erc20, err := k.DenomToERC20Lookup(ctx, req.Denom)

	var ret types.QueryDenomToERC20Response
	ret.Erc20 = erc20.Hex()
	ret.CosmosOriginated = cosmosOriginated

	return &ret, err
}

// ERC20ToDenom queries the ERC20 contract that maps to an Ethereum ERC20 if any
func (k *Keeper) ERC20ToDenom(c context.Context, req *types.QueryERC20ToDenomRequest) (*types.QueryERC20ToDenomResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer k.Meter(ctx).FuncTiming(&ctx, "ERC20ToDenom")()

	cosmosOriginated, name := k.ERC20ToDenomLookup(ctx, gethcommon.HexToAddress(req.Erc20))

	var ret types.QueryERC20ToDenomResponse
	ret.Denom = name
	ret.CosmosOriginated = cosmosOriginated

	return &ret, nil
}

func (k *Keeper) GetDelegateKeyByValidator(c context.Context, req *types.QueryDelegateKeysByValidatorAddress) (*types.QueryDelegateKeysByValidatorAddressResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer k.Meter(ctx).FuncTiming(&ctx, "GetDelegateKeyByValidator")()

	valAddress, err := sdk.ValAddressFromBech32(req.ValidatorAddress)
	if err != nil {
		return nil, err
	}

	valAccountAddr := sdk.AccAddress(valAddress.Bytes())
	keys := k.GetOrchestratorAddresses(ctx)

	for _, key := range keys {
		senderAddr, err := sdk.AccAddressFromBech32(key.Sender)
		if err != nil {
			return nil, err
		}
		if valAccountAddr.Equals(senderAddr) {
			return &types.QueryDelegateKeysByValidatorAddressResponse{EthAddress: key.EthAddress, OrchestratorAddress: key.Orchestrator}, nil
		}
	}

	return nil, errors.Wrap(types.ErrInvalid, "No validator")
}

func (k *Keeper) GetDelegateKeyByOrchestrator(c context.Context, req *types.QueryDelegateKeysByOrchestratorAddress) (*types.QueryDelegateKeysByOrchestratorAddressResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer k.Meter(ctx).FuncTiming(&ctx, "GetDelegateKeyByOrchestrator")()

	keys := k.GetOrchestratorAddresses(ctx)

	_, err := sdk.AccAddressFromBech32(req.OrchestratorAddress)
	if err != nil {
		return nil, err
	}

	for _, key := range keys {
		if req.OrchestratorAddress == key.Orchestrator {
			return &types.QueryDelegateKeysByOrchestratorAddressResponse{ValidatorAddress: key.Sender, EthAddress: key.EthAddress}, nil
		}
	}

	return nil, errors.Wrap(types.ErrInvalid, "No validator")
}

func (k *Keeper) GetDelegateKeyByEth(c context.Context, req *types.QueryDelegateKeysByEthAddress) (*types.QueryDelegateKeysByEthAddressResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer k.Meter(ctx).FuncTiming(&ctx, "GetDelegateKeyByEth")()

	keys := k.GetOrchestratorAddresses(ctx)
	if err := types.ValidateEthAddress(req.EthAddress); err != nil {
		return nil, errors.Wrap(err, "invalid eth address")
	}

	for _, key := range keys {
		if req.EthAddress == key.EthAddress {
			return &types.QueryDelegateKeysByEthAddressResponse{
				ValidatorAddress:    key.Sender,
				OrchestratorAddress: key.Orchestrator}, nil
		}
	}

	return nil, errors.Wrap(types.ErrInvalid, "No validator")
}

func (k *Keeper) GetPendingSendToEth(c context.Context, req *types.QueryPendingSendToEth) (*types.QueryPendingSendToEthResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer k.Meter(ctx).FuncTiming(&ctx, "GetPendingSendToEth")()

	batches := k.GetOutgoingTxBatches(ctx)
	unbatchedTx := k.GetPoolTransactions(ctx)
	senderAddress := req.SenderAddress

	res := &types.QueryPendingSendToEthResponse{}
	res.TransfersInBatches = make([]*types.OutgoingTransferTx, 0)
	res.UnbatchedTransfers = make([]*types.OutgoingTransferTx, 0)

	for _, batch := range batches {
		for _, tx := range batch.Transactions {
			if tx.Sender == senderAddress {
				res.TransfersInBatches = append(res.TransfersInBatches, tx)
			}
		}
	}

	for _, tx := range unbatchedTx {
		if strings.EqualFold(tx.Sender, senderAddress) {
			res.UnbatchedTransfers = append(res.UnbatchedTransfers, tx)

		}
	}

	return res, nil
}

func (k *Keeper) PeggyModuleState(c context.Context, req *types.QueryModuleStateRequest) (*types.QueryModuleStateResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer k.Meter(ctx).FuncTiming(&ctx, "PeggyModuleState")()

	var (
		p                               = k.GetParams(ctx)
		batches                         = k.GetOutgoingTxBatches(ctx)
		valsets                         = k.GetValsets(ctx)
		attmap                          = k.GetAttestationMapping(ctx)
		vsconfs                         = []*types.MsgValsetConfirm{}
		batchconfs                      = []*types.MsgConfirmBatch{}
		attestations                    = []*types.Attestation{}
		orchestratorAddresses           = k.GetOrchestratorAddresses(ctx)
		lastObservedEventNonce          = k.GetLastObservedEventNonce(ctx)
		lastObservedEthereumBlockHeight = k.GetLastObservedEthereumBlockHeight(ctx)
		erc20ToDenoms                   = []*types.ERC20ToDenom{}
		unbatchedTransfers              = k.GetPoolTransactions(ctx)
		ethereumBlacklistAddresses      = k.GetAllEthereumBlacklistAddresses(ctx)
		rateLimits                      = k.GetRateLimits(ctx)
		rateLimitTransfers              = k.GetAllRateLimitTransfers(ctx)
	)

	// export valset confirmations from state
	for _, vs := range valsets {
		vsconfs = append(vsconfs, k.GetValsetConfirms(ctx, vs.Nonce)...)
	}

	// export batch confirmations from state
	for _, batch := range batches {
		tokenAddress := gethcommon.HexToAddress(batch.TokenContract)
		confirms := k.GetBatchConfirmByNonceAndTokenContract(ctx, batch.BatchNonce, tokenAddress)
		batchconfs = append(batchconfs, confirms...)
	}

	// sort attestation map keys since map iteration is non-deterministic
	attestationHeights := make([]uint64, 0, len(attmap))
	for k := range attmap {
		attestationHeights = append(attestationHeights, k)
	}
	sort.SliceStable(attestationHeights, func(i, j int) bool {
		return attestationHeights[i] < attestationHeights[j]
	})

	for _, height := range attestationHeights {
		attestations = append(attestations, attmap[height]...)
	}

	// export erc20 to denom relations
	k.IterateERC20ToDenom(ctx, func(_ []byte, erc20ToDenom *types.ERC20ToDenom) bool {
		erc20ToDenoms = append(erc20ToDenoms, erc20ToDenom)
		return false
	})

	lastOutgoingBatchID := k.GetLastOutgoingBatchID(ctx)
	lastOutgoingPoolID := k.GetLastOutgoingPoolID(ctx)
	lastObservedValset := k.GetLastObservedValset(ctx)

	state := types.GenesisState{
		Params:                     p,
		LastObservedNonce:          lastObservedEventNonce,
		LastObservedEthereumHeight: lastObservedEthereumBlockHeight.EthereumBlockHeight,
		Valsets:                    valsets,
		ValsetConfirms:             vsconfs,
		Batches:                    batches,
		BatchConfirms:              batchconfs,
		Attestations:               attestations,
		OrchestratorAddresses:      orchestratorAddresses,
		Erc20ToDenoms:              erc20ToDenoms,
		UnbatchedTransfers:         unbatchedTransfers,
		LastOutgoingBatchId:        lastOutgoingBatchID,
		LastOutgoingPoolId:         lastOutgoingPoolID,
		LastObservedValset:         *lastObservedValset,
		EthereumBlacklist:          ethereumBlacklistAddresses,
		RateLimits:                 rateLimits,
		MintAmounts:                k.GetMintAmounts(ctx),
		RateLimitTransfers:         rateLimitTransfers,
	}

	res := &types.QueryModuleStateResponse{
		State: &state,
	}

	return res, nil
}

func (k *Keeper) MissingPeggoNonces(
	c context.Context,
	_ *types.MissingNoncesRequest,
) (*types.MissingNoncesResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	defer k.Meter(ctx).FuncTiming(&ctx, "MissingPeggoNonces")()

	var res []string

	bondedValidators, err := k.StakingKeeper.GetBondedValidatorsByPower(ctx)
	if err != nil {
		return nil, err
	}
	for i := range bondedValidators {
		val, _ := sdk.ValAddressFromBech32(bondedValidators[i].GetOperator())
		ev := k.GetLastEventByValidator(ctx, val)
		if ev.EthereumEventNonce == 0 && ev.EthereumEventHeight == 0 {
			res = append(res, bondedValidators[i].GetOperator())
		}
	}

	return &types.MissingNoncesResponse{OperatorAddresses: res}, nil
}
