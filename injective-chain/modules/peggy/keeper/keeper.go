package keeper

import (
	"bytes"
	"context"
	"encoding/binary"
	gomath "math"
	"sort"

	"cosmossdk.io/errors"
	"cosmossdk.io/log"
	"cosmossdk.io/math"
	"cosmossdk.io/store/prefix"
	storetypes "cosmossdk.io/store/types"
	"github.com/InjectiveLabs/metrics/v2"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/auth/keeper"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	distrkeeper "github.com/cosmos/cosmos-sdk/x/distribution/keeper"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/ethereum/go-ethereum/common"

	exchangekeeper "github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/keeper"
	exchangetypes "github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/types"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/peggy/types"
	chaintypes "github.com/InjectiveLabs/injective-core/injective-chain/types"
)

// Keeper maintains the link to storage and exposes getter/setter methods for the various parts of the state machine
type Keeper struct {
	cdc      codec.Codec         // The wire codec for binary encoding/decoding.
	storeKey storetypes.StoreKey // Unexposed key to access store from sdk.Context

	StakingKeeper     types.StakingKeeper
	bankKeeper        types.BankKeeper
	DistKeeper        distrkeeper.Keeper
	SlashingKeeper    types.SlashingKeeper
	exchangeMsgServer exchangetypes.MsgServer
	OracleKeeper      types.OracleKeeper

	AttestationHandler interface {
		Handle(sdk.Context, types.EthereumClaim) error
	}

	meter metrics.Meter

	// address authorized to execute MsgUpdateParams. Default: gov module
	authority     string
	accountKeeper keeper.AccountKeeper
}

// NewKeeper returns a new instance of the peggy keeper
func NewKeeper(
	cdc codec.Codec,
	storeKey storetypes.StoreKey,
	stakingKeeper types.StakingKeeper,
	bankKeeper types.BankKeeper,
	slashingKeeper types.SlashingKeeper,
	distKeeper distrkeeper.Keeper,
	exchangeKeeper *exchangekeeper.Keeper,
	oracleKeeper types.OracleKeeper,
	authority string,
	accountKeeper keeper.AccountKeeper,
) Keeper {
	k := Keeper{
		cdc:               cdc,
		storeKey:          storeKey,
		StakingKeeper:     stakingKeeper,
		bankKeeper:        bankKeeper,
		DistKeeper:        distKeeper,
		SlashingKeeper:    slashingKeeper,
		OracleKeeper:      oracleKeeper,
		exchangeMsgServer: exchangekeeper.NewV1MsgServerImpl(exchangeKeeper, exchangekeeper.NewMsgServerImpl(exchangeKeeper)),
		authority:         authority,
		accountKeeper:     accountKeeper,
	}

	k.AttestationHandler = NewAttestationHandler(bankKeeper, k)

	return k
}

func (*Keeper) Logger(ctx sdk.Context) log.Logger {
	return ctx.Logger().With("module", types.ModuleName)
}

func (k *Keeper) Meter(ctx context.Context) metrics.Meter {
	if k.meter == nil {
		k.meter = sdk.UnwrapSDKContext(ctx).Meter().SubMeter(types.ModuleName, metrics.Tag("svc", types.ModuleName))
	}

	return k.meter
}

/////////////////////////////
//     VALSET REQUESTS     //
/////////////////////////////

// SetValsetRequest returns a new instance of the Peggy BridgeValidatorSet
// i.e. {"nonce": 1, "memebers": [{"eth_addr": "foo", "power": 11223}]}
func (k *Keeper) SetValsetRequest(ctx sdk.Context) *types.Valset {
	defer k.Meter(ctx).FuncTiming(&ctx, "SetValsetRequest")()

	valset := k.GetCurrentValset(ctx)

	// If none of the bonded validators has registered eth key, then valset.Members = 0.
	if len(valset.Members) == 0 {
		return nil
	}

	k.StoreValset(ctx, valset)
	// Store the checkpoint as a legit past valset
	checkpoint := valset.GetCheckpoint(k.GetPeggyID(ctx))
	k.SetPastEthSignatureCheckpoint(ctx, checkpoint)

	// nolint:errcheck //ignored on purpose
	ctx.EventManager().EmitTypedEvent(&types.EventValsetUpdateRequest{
		ValsetNonce:   valset.Nonce,
		ValsetHeight:  valset.Height,
		ValsetMembers: valset.Members,
		RewardAmount:  valset.RewardAmount,
		RewardToken:   valset.RewardToken,
	})

	return valset
}

// StoreValset is for storing a valiator set at a given height
func (k *Keeper) StoreValset(ctx sdk.Context, valset *types.Valset) {
	defer k.Meter(ctx).FuncTiming(&ctx, "StoreValset")()

	store := ctx.KVStore(k.storeKey)
	valset.Height = uint64(ctx.BlockHeight())
	store.Set(types.GetValsetKey(valset.Nonce), k.cdc.MustMarshal(valset))
	k.SetLatestValsetNonce(ctx, valset.Nonce)
}

// SetLatestValsetNonce sets the latest valset nonce
func (k *Keeper) SetLatestValsetNonce(ctx sdk.Context, nonce uint64) {
	defer k.Meter(ctx).FuncTiming(&ctx, "SetLatestValsetNonce")()

	store := ctx.KVStore(k.storeKey)
	store.Set(types.LatestValsetNonce, types.UInt64Bytes(nonce))
}

// StoreValsetUnsafe is for storing a valiator set at a given height
func (k *Keeper) StoreValsetUnsafe(ctx sdk.Context, valset *types.Valset) {
	defer k.Meter(ctx).FuncTiming(&ctx, "StoreValsetUnsafe")()

	store := ctx.KVStore(k.storeKey)
	store.Set(types.GetValsetKey(valset.Nonce), k.cdc.MustMarshal(valset))
	k.SetLatestValsetNonce(ctx, valset.Nonce)
}

// HasValsetRequest returns true if a valset defined by a nonce exists
func (k *Keeper) HasValsetRequest(ctx sdk.Context, nonce uint64) bool {
	defer k.Meter(ctx).FuncTiming(&ctx, "HasValsetRequest")()

	store := ctx.KVStore(k.storeKey)
	return store.Has(types.GetValsetKey(nonce))
}

// DeleteValset deletes the valset at a given nonce from state
func (k *Keeper) DeleteValset(ctx sdk.Context, nonce uint64) {
	defer k.Meter(ctx).FuncTiming(&ctx, "DeleteValset")()

	ctx.KVStore(k.storeKey).Delete(types.GetValsetKey(nonce))
}

// GetLatestValsetNonce returns the latest valset nonce
func (k *Keeper) GetLatestValsetNonce(ctx sdk.Context) uint64 {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetLatestValsetNonce")()

	store := ctx.KVStore(k.storeKey)
	bytes := store.Get(types.LatestValsetNonce)

	if len(bytes) == 0 {
		return 0
	}

	return types.UInt64FromBytes(bytes)
}

// GetValset returns a valset by nonce
func (k *Keeper) GetValset(ctx sdk.Context, nonce uint64) *types.Valset {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetValset")()

	store := ctx.KVStore(k.storeKey)
	bz := store.Get(types.GetValsetKey(nonce))
	if bz == nil {
		return nil
	}

	var valset types.Valset
	k.cdc.MustUnmarshal(bz, &valset)

	return &valset
}

// IterateValsets retruns all valsetRequests
func (k *Keeper) IterateValsets(ctx sdk.Context, cb func(key []byte, val *types.Valset) bool) {
	defer k.Meter(ctx).FuncTiming(&ctx, "IterateValsets")()

	valsetStore := prefix.NewStore(k.getStore(ctx), types.ValsetRequestKey)
	chaintypes.IterateSafe(valsetStore.ReverseIterator(nil, nil), func(key, value []byte) (stop bool) {
		var vs types.Valset
		k.cdc.MustUnmarshal(value, &vs)

		return cb(key, &vs)
	})
}

// GetValsets returns all the validator sets in state
func (k *Keeper) GetValsets(ctx sdk.Context) (out []*types.Valset) {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetValsets")()

	k.IterateValsets(ctx, func(_ []byte, val *types.Valset) bool {
		out = append(out, val)
		return false
	})

	sort.Sort(types.Valsets(out))

	return
}

// GetLatestValset returns the latest validator set in state
func (k *Keeper) GetLatestValset(ctx sdk.Context) (out *types.Valset) {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetLatestValset")()

	latestValsetNonce := k.GetLatestValsetNonce(ctx)
	out = k.GetValset(ctx, latestValsetNonce)

	return
}

func (k *Keeper) SetLastJailedValsetNonce(ctx sdk.Context, nonce uint64) {
	defer k.Meter(ctx).FuncTiming(&ctx, "SetLastJailedValsetNonce")()

	store := ctx.KVStore(k.storeKey)
	store.Set(types.LastJailedValsetNonce, types.UInt64Bytes(nonce))
}

// GetLastJailedValsetNonce returns the latest jailed valset nonce
func (k *Keeper) GetLastJailedValsetNonce(ctx sdk.Context) uint64 {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetLastJailedValsetNonce")()

	store := ctx.KVStore(k.storeKey)
	bz := store.Get(types.LastJailedValsetNonce)

	if len(bz) == 0 {
		return 0
	}

	return types.UInt64FromBytes(bz)
}

// SetLastUnbondingBlockHeight sets the last unbonding block height
func (k *Keeper) SetLastUnbondingBlockHeight(ctx sdk.Context, unbondingBlockHeight uint64) {
	defer k.Meter(ctx).FuncTiming(&ctx, "SetLastUnbondingBlockHeight")()

	store := ctx.KVStore(k.storeKey)
	store.Set(types.LastUnbondingBlockHeight, types.UInt64Bytes(unbondingBlockHeight))
}

// GetLastUnbondingBlockHeight returns the last unbonding block height
func (k *Keeper) GetLastUnbondingBlockHeight(ctx sdk.Context) uint64 {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetLastUnbondingBlockHeight")()

	store := ctx.KVStore(k.storeKey)
	bytes := store.Get(types.LastUnbondingBlockHeight)

	if len(bytes) == 0 {
		return 0
	}

	return types.UInt64FromBytes(bytes)
}

func (k *Keeper) GetUnjailedValsets(ctx sdk.Context, maxHeight uint64) (out []*types.Valset) {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetUnjailedValsets")()

	lastJailedValsetNonce := k.GetLastJailedValsetNonce(ctx)
	k.IterateValsetByJailedValsetNonce(ctx, lastJailedValsetNonce, maxHeight, func(_ []byte, valset *types.Valset) bool {
		if valset.Nonce > lastJailedValsetNonce {
			out = append(out, valset)
		}
		return false
	})

	return
}

// IterateValsetByJailedValsetNonce iterates through all valset by last jailed valset nonce in ASC order
func (k *Keeper) IterateValsetByJailedValsetNonce(
	ctx sdk.Context,
	lastJailedValsetNonce uint64,
	maxHeight uint64,
	cb func(k []byte, v *types.Valset) (stop bool),
) {
	defer k.Meter(ctx).FuncTiming(&ctx, "IterateValsetBySlashedValsetNonce")()

	start, end := types.UInt64Bytes(lastJailedValsetNonce), types.UInt64Bytes(maxHeight)
	valsetStore := prefix.NewStore(ctx.KVStore(k.storeKey), types.ValsetRequestKey)
	chaintypes.IterateSafe(valsetStore.Iterator(start, end), func(key, value []byte) (stop bool) {
		var vs types.Valset
		k.cdc.MustUnmarshal(value, &vs)

		return cb(key, &vs)
	})
}

/////////////////////////////
//     VALSET CONFIRMS     //
/////////////////////////////

// GetValsetConfirm returns a valset confirmation by a nonce and validator address
func (k *Keeper) GetValsetConfirm(ctx sdk.Context, nonce uint64, validator sdk.AccAddress) *types.MsgValsetConfirm {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetValsetConfirm")()

	store := ctx.KVStore(k.storeKey)
	entity := store.Get(types.GetValsetConfirmKey(nonce, validator))
	if entity == nil {
		return nil
	}

	valset := types.MsgValsetConfirm{}
	k.cdc.MustUnmarshal(entity, &valset)

	return &valset
}

// SetValsetConfirm sets a valset confirmation
func (k *Keeper) SetValsetConfirm(ctx sdk.Context, valset *types.MsgValsetConfirm) []byte {
	defer k.Meter(ctx).FuncTiming(&ctx, "SetValsetConfirm")()

	store := ctx.KVStore(k.storeKey)
	addr, err := sdk.AccAddressFromBech32(valset.Orchestrator)
	if err != nil {
		panic(err)
	}

	key := types.GetValsetConfirmKey(valset.Nonce, addr)
	store.Set(key, k.cdc.MustMarshal(valset))

	return key
}

// GetValsetConfirms returns all validator set confirmations by nonce
func (k *Keeper) GetValsetConfirms(ctx sdk.Context, nonce uint64) (valsetConfirms []*types.MsgValsetConfirm) {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetValsetConfirms")()

	valsetConfirmStore := prefix.NewStore(k.getStore(ctx), types.ValsetConfirmKey)
	chaintypes.IterateSafe(valsetConfirmStore.Iterator(PrefixRange(types.UInt64Bytes(nonce))), func(_, value []byte) (stop bool) {
		var confirm types.MsgValsetConfirm
		k.cdc.MustUnmarshal(value, &confirm)

		valsetConfirms = append(valsetConfirms, &confirm)
		return false
	})

	return valsetConfirms
}

// IterateValsetConfirmByNonce iterates through all valset confirms by validator set nonce in ASC order
func (k *Keeper) IterateValsetConfirmByNonce(ctx sdk.Context, nonce uint64, cb func(k []byte, v *types.MsgValsetConfirm) (stop bool)) {
	defer k.Meter(ctx).FuncTiming(&ctx, "IterateValsetConfirmByNonce")()

	valsetConfirmStore := prefix.NewStore(ctx.KVStore(k.storeKey), types.ValsetConfirmKey)
	chaintypes.IterateSafe(valsetConfirmStore.Iterator(PrefixRange(types.UInt64Bytes(nonce))), func(key, value []byte) (stop bool) {
		var confirm types.MsgValsetConfirm
		k.cdc.MustUnmarshal(value, &confirm)

		return cb(key, &confirm)
	})
}

/////////////////////////////
//      BATCH CONFIRMS     //
/////////////////////////////

// GetBatchConfirm returns a batch confirmation given its nonce, the token contract, and a validator address
func (k *Keeper) GetBatchConfirm(ctx sdk.Context, nonce uint64, tokenContract common.Address, validator sdk.AccAddress) *types.MsgConfirmBatch {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetBatchConfirm")()

	store := ctx.KVStore(k.storeKey)
	entity := store.Get(types.GetBatchConfirmKey(tokenContract, nonce, validator))
	if entity == nil {
		return nil
	}

	batch := types.MsgConfirmBatch{}
	k.cdc.MustUnmarshal(entity, &batch)

	return &batch
}

// SetBatchConfirm sets a batch confirmation by a validator
func (k *Keeper) SetBatchConfirm(ctx sdk.Context, batch *types.MsgConfirmBatch) []byte {
	defer k.Meter(ctx).FuncTiming(&ctx, "SetBatchConfirm")()

	// convert eth signer to hex string lol
	batch.EthSigner = common.HexToAddress(batch.EthSigner).Hex()
	tokenContract := common.HexToAddress(batch.TokenContract)
	store := ctx.KVStore(k.storeKey)

	acc, err := sdk.AccAddressFromBech32(batch.Orchestrator)
	if err != nil {
		panic(err)
	}

	key := types.GetBatchConfirmKey(tokenContract, batch.Nonce, acc)
	store.Set(key, k.cdc.MustMarshal(batch))

	return key
}

// IterateBatchConfirmByNonceAndTokenContract iterates through all batch confirmations
func (k *Keeper) IterateBatchConfirmByNonceAndTokenContract(
	ctx sdk.Context,
	nonce uint64,
	tokenContract common.Address,
	cb func(k []byte, v *types.MsgConfirmBatch) (stop bool),
) {
	defer k.Meter(ctx).FuncTiming(&ctx, "IterateBatchConfirmByNonceAndTokenContract")()

	batchConfirmStore := prefix.NewStore(ctx.KVStore(k.storeKey), types.BatchConfirmKey)
	batchPrefix := append(tokenContract.Bytes(), types.UInt64Bytes(nonce)...)
	chaintypes.IterateSafe(batchConfirmStore.Iterator(PrefixRange(batchPrefix)), func(key, value []byte) (stop bool) {
		var confirm types.MsgConfirmBatch
		k.cdc.MustUnmarshal(value, &confirm)

		return cb(key, &confirm)
	})
}

// GetBatchConfirmByNonceAndTokenContract returns the batch confirms
func (k *Keeper) GetBatchConfirmByNonceAndTokenContract(ctx sdk.Context, nonce uint64, tokenContract common.Address) (out []*types.MsgConfirmBatch) {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetBatchConfirmByNonceAndTokenContract")()

	k.IterateBatchConfirmByNonceAndTokenContract(ctx, nonce, tokenContract, func(_ []byte, msg *types.MsgConfirmBatch) (stop bool) {
		out = append(out, msg)
		return false
	})

	return
}

/////////////////////////////
//    ADDRESS DELEGATION   //
/////////////////////////////

// SetOrchestratorValidator sets the Orchestrator key for a given validator
func (k *Keeper) SetOrchestratorValidator(ctx sdk.Context, val sdk.ValAddress, orch sdk.AccAddress) {
	defer k.Meter(ctx).FuncTiming(&ctx, "SetOrchestratorValidator")()

	store := ctx.KVStore(k.storeKey)
	store.Set(types.GetOrchestratorAddressKey(orch), val.Bytes())
}

// GetOrchestratorValidator returns the validator key associated with an orchestrator key
func (k *Keeper) GetOrchestratorValidator(ctx sdk.Context, orch sdk.AccAddress) (sdk.ValAddress, bool) {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetOrchestratorValidator")()

	store := ctx.KVStore(k.storeKey)

	bz := store.Get(types.GetOrchestratorAddressKey(orch))
	if bz == nil {
		return nil, false
	}

	return sdk.ValAddress(bz), true
}

/////////////////////////////
//       ETH ADDRESS       //
/////////////////////////////

// SetEthAddressForValidator sets the ethereum address for a given validator
func (k *Keeper) SetEthAddressForValidator(ctx sdk.Context, validator sdk.ValAddress, ethAddr common.Address) {
	defer k.Meter(ctx).FuncTiming(&ctx, "SetEthAddressForValidator")()

	store := ctx.KVStore(k.storeKey)
	store.Set(types.GetEthAddressByValidatorKey(validator), ethAddr.Bytes())
	store.Set(types.GetValidatorByEthAddressKey(ethAddr), validator.Bytes())
}

// GetEthAddressByValidator returns the eth address for a given peggy validator
func (k *Keeper) GetEthAddressByValidator(ctx sdk.Context, validator sdk.ValAddress) (common.Address, bool) {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetEthAddressByValidator")()

	store := ctx.KVStore(k.storeKey)

	bz := store.Get(types.GetEthAddressByValidatorKey(validator))
	if bz == nil {
		return common.Address{}, false
	}

	return common.BytesToAddress(bz), true
}

// GetValidatorByEthAddress returns the validator for a given eth address
func (k *Keeper) GetValidatorByEthAddress(ctx sdk.Context, ethAddr common.Address) (validator stakingtypes.Validator, found bool) {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetValidatorByEthAddress")()

	store := ctx.KVStore(k.storeKey)
	valAddr := store.Get(types.GetValidatorByEthAddressKey(ethAddr))
	if valAddr == nil {
		return stakingtypes.Validator{}, false
	}
	validator, err := k.StakingKeeper.GetValidator(ctx, valAddr)
	if err != nil {
		return stakingtypes.Validator{}, false
	}

	return validator, true
}

// GetCurrentValset gets powers from the store and normalizes them
// into an integer percentage with a resolution of uint32 Max meaning
// a given validators 'Peggy power' is computed as
// Cosmos power for that validator / total cosmos power = x / uint32 Max
// where x is the voting power on the Peggy contract. This allows us
// to only use integer division which produces a known rounding error
// from truncation equal to the ratio of the validators
// Cosmos power / total cosmos power ratio, leaving us at uint32 Max - 1
// total voting power. This is an acceptable rounding error since floating
// point may cause consensus problems if different floating point unit
// implementations are involved.
//
// 'total cosmos power' has an edge case, if a validator has not set their
// Ethereum key they are not included in the total. If they where control
// of the bridge could be lost in the following situation.
//
// If we have 100 total power, and 100 total power joins the validator set
// the new validators hold more than 33% of the bridge power, if we generate
// and submit a valset and they don't have their eth keys set they can never
// update the validator set again and the bridge and all its' funds are lost.
// For this reason we exclude validators with unset eth keys from validator sets
func (k *Keeper) GetCurrentValset(ctx sdk.Context) *types.Valset {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetCurrentValset")()

	validators, _ := k.StakingKeeper.GetBondedValidatorsByPower(ctx)
	// allocate enough space for all validators, but len zero, we then append
	// so that we have an array with extra capacity but the correct length depending
	// on how many validators have keys set.
	bridgeValidators := make([]*types.BridgeValidator, 0, len(validators))
	var totalPower uint64
	for i := range validators {
		val, _ := sdk.ValAddressFromBech32(validators[i].GetOperator())
		vp, _ := k.StakingKeeper.GetLastValidatorPower(ctx, val)
		p := uint64(vp)

		if ethAddress, found := k.GetEthAddressByValidator(ctx, val); found {
			bv := &types.BridgeValidator{Power: p, EthereumAddress: ethAddress.Hex()}
			bridgeValidators = append(bridgeValidators, bv)
			totalPower += p
		}
	}

	// normalize power values
	for i := range bridgeValidators {

		bridgeValidators[i].Power = math.NewUint(bridgeValidators[i].Power).MulUint64(gomath.MaxUint32).QuoUint64(totalPower).Uint64()
	}

	// get the reward from the params store
	reward := k.GetParams(ctx).ValsetReward
	var rewardToken common.Address
	var rewardAmount math.Int
	if reward.Denom == "" {
		// the case where a validator has 'no reward'. The 'no reward' value is interpreted as having a zero
		// address for the ERC20 token and a zero value for the reward amount. Since we store a coin with the
		// params, a coin with a blank denom and/or zero amount is interpreted in this way.
		rewardToken = common.Address{0x0000000000000000000000000000000000000000}
		rewardAmount = math.NewIntFromUint64(0)

	} else {
		rewardToken, rewardAmount = k.RewardToERC20Lookup(ctx, reward)
	}
	// TODO: make the nonce an incrementing one (i.e. fetch last nonce from state, increment, set here)
	return types.NewValset(uint64(ctx.BlockHeight()), uint64(ctx.BlockHeight()), bridgeValidators, rewardAmount, rewardToken)
}

/////////////////////////////
//       HELPERS           //
/////////////////////////////

func (k *Keeper) getStore(ctx sdk.Context) storetypes.KVStore {
	return ctx.KVStore(k.storeKey)
}

// SendToCommunityPool handles incorrect SendToCosmos calls to the community pool, since the calls
// have already been made on Ethereum there's nothing we can do to reverse them, and we should at least
// make use of the tokens which would otherwise be lost
func (k *Keeper) SendToCommunityPool(ctx sdk.Context, coins sdk.Coins) error {
	defer k.Meter(ctx).FuncTiming(&ctx, "SendToCommunityPool")()

	if err := k.bankKeeper.SendCoinsFromModuleToModule(ctx, types.ModuleName, distrtypes.ModuleName, coins); err != nil {
		return errors.Wrap(err, "transfer to community pool failed")
	}
	feePool, err := k.DistKeeper.FeePool.Get(ctx)

	if err != nil {
		return err
	}

	feePool.CommunityPool = feePool.CommunityPool.Add(sdk.NewDecCoinsFromCoins(coins...)...)
	err = k.DistKeeper.FeePool.Set(ctx, feePool)

	return err
}

/////////////////////////////
//       PARAMS        //
/////////////////////////////

// GetParams returns the parameters from the store
func (k *Keeper) GetParams(ctx sdk.Context) *types.Params {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetParams")()

	store := k.getStore(ctx)
	bz := store.Get(types.ParamKey)
	if bz == nil {
		return nil
	}

	params := &types.Params{}
	k.cdc.MustUnmarshal(bz, params)

	return params
}

// SetParams sets the parameters in the store
func (k *Keeper) SetParams(ctx sdk.Context, params *types.Params) {
	defer k.Meter(ctx).FuncTiming(&ctx, "SetParams")()

	store := k.getStore(ctx)
	bz := k.cdc.MustMarshal(params)
	store.Set(types.ParamKey, bz)
}

// GetBridgeContractAddress returns the bridge contract address on ETH
func (k *Keeper) GetBridgeContractAddress(ctx sdk.Context) common.Address {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetBridgeContractAddress")()

	params := k.GetParams(ctx)
	if params == nil {
		return common.Address{}
	}

	return common.HexToAddress(params.BridgeEthereumAddress)
}

// GetBridgeChainID returns the chain id of the ETH chain we are running against
func (k *Keeper) GetBridgeChainID(ctx sdk.Context) uint64 {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetBridgeChainID")()

	params := k.GetParams(ctx)
	if params == nil {
		return 0
	}

	return params.BridgeChainId
}

func (k *Keeper) GetPeggyID(ctx sdk.Context) string {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetPeggyID")()

	params := k.GetParams(ctx)
	if params == nil {
		return ""
	}

	return params.PeggyId
}

// GetCosmosCoinDenom returns native cosmos coin denom
func (k *Keeper) GetCosmosCoinDenom(ctx sdk.Context) string {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetCosmosCoinDenom")()

	params := k.GetParams(ctx)
	if params == nil {
		return ""
	}

	return params.CosmosCoinDenom
}

// GetCosmosCoinERC20Contract returns the Cosmos coin ERC20 contract address
func (k *Keeper) GetCosmosCoinERC20Contract(ctx sdk.Context) common.Address {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetCosmosCoinERC20Contract")()

	params := k.GetParams(ctx)
	if params == nil {
		return common.Address{}
	}

	return common.HexToAddress(params.CosmosCoinErc20Contract)
}

func (k *Keeper) UnpackAttestationClaim(attestation *types.Attestation) (types.EthereumClaim, error) {
	var msg types.EthereumClaim

	err := k.cdc.UnpackAny(attestation.Claim, &msg)
	if err != nil {
		err = errors.Wrap(err, "failed to unpack EthereumClaim")
		return nil, err
	} else {
		return msg, nil
	}
}

// GetOrchestratorAddresses iterates both the EthAddress and Orchestrator address indexes to produce
// a vector of MsgSetOrchestratorAddresses entires containing all the delgate keys for state
// export / import. This may seem at first glance to be excessively complicated, why not combine
// the EthAddress and Orchestrator address indexes and simply iterate one thing? The answer is that
// even though we set the Eth and Orchestrator address in the same place we use them differently we
// always go from Orchestrator address to Validator address and from validator address to Ethereum address
// we want to keep looking up the validator address for various reasons, so a direct Orchestrator to Ethereum
// address mapping will mean having to keep two of the same data around just to provide lookups.
//
// For the time being this will serve
func (k *Keeper) GetOrchestratorAddresses(ctx sdk.Context) []*types.MsgSetOrchestratorAddresses {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetOrchestratorAddresses")()

	var (
		store                 = k.getStore(ctx)
		ethAddresses          = make(map[string]common.Address)
		orchestratorAddresses = make(map[string]sdk.AccAddress)
	)

	chaintypes.IterateSafe(store.Iterator(PrefixRange(types.EthAddressByValidatorKey)), func(key, value []byte) (stop bool) {
		// the 'key' contains both the prefix and the value, so we need
		// to cut off the starting bytes, if you don't do this a valid
		// cosmos key will be made out of EthAddressByValidatorKey + the startin bytes
		// of the actual key
		validatorKey := key[len(types.EthAddressByValidatorKey):]
		ethAddress := common.BytesToAddress(value)
		validatorAccount := sdk.AccAddress(validatorKey)
		ethAddresses[validatorAccount.String()] = ethAddress
		return false
	})

	chaintypes.IterateSafe(store.Iterator(PrefixRange(types.KeyOrchestratorAddress)), func(key, value []byte) (stop bool) {
		orchestratorKey := key[len(types.KeyOrchestratorAddress):]
		orchestratorAccount := sdk.AccAddress(orchestratorKey)
		validatorAccount := sdk.AccAddress(value)
		orchestratorAddresses[validatorAccount.String()] = orchestratorAccount
		return false
	})

	result := make([]*types.MsgSetOrchestratorAddresses, 0, len(ethAddresses))
	for validatorAccount, ethAddress := range ethAddresses {
		orchestratorAccount, ok := orchestratorAddresses[validatorAccount]
		if !ok {
			panic("cannot find validator account in orchestrator addresses mapping")
		}

		result = append(result, &types.MsgSetOrchestratorAddresses{
			Sender:       validatorAccount,
			Orchestrator: orchestratorAccount.String(),
			EthAddress:   ethAddress.Hex(),
		})
	}

	// we iterated over a map, so now we have to sort to ensure the
	// output here is deterministic, eth address chosen for no particular
	// reason
	sort.SliceStable(result, func(i, j int) bool {
		return result[i].EthAddress < result[j].EthAddress
	})

	return result
}

// DeserializeValidatorIterator returns validators from the validator iterator.
// Adding here in gravity keeper as cdc is not available inside endblocker.
func (k *Keeper) DeserializeValidatorIterator(vals []byte) stakingtypes.ValAddresses {
	validators := stakingtypes.ValAddresses{}
	k.cdc.MustUnmarshal(vals, &validators)
	return validators
}

type PrefixStart []byte
type PrefixEnd []byte

// PrefixRange turns a prefix into a (start, end) range. The start is the given prefix value and
// the end is calculated by adding 1 bit to the start value. Nil is not allowed as prefix.
//
//	Example: []byte{1, 3, 4} becomes []byte{1, 3, 5}
//			 []byte{15, 42, 255, 255} becomes []byte{15, 43, 0, 0}
//
// In case of an overflow the end is set to nil.
//
//	Example: []byte{255, 255, 255, 255} becomes nil
//
// MARK finish-batches: this is where some crazy shit happens
func PrefixRange(proposedPrefix []byte) (PrefixStart, PrefixEnd) {
	if proposedPrefix == nil {
		panic("nil key not allowed")
	}

	// special case: no prefix is whole range
	if len(proposedPrefix) == 0 {
		return nil, nil
	}

	// copy the prefix and update last byte
	end := make([]byte, len(proposedPrefix))
	copy(end, proposedPrefix)
	l := len(end) - 1
	end[l]++

	// wait, what if that overflowed?....
	for end[l] == 0 && l > 0 {
		l--
		end[l]++
	}

	// okay, funny guy, you gave us FFF, no end to this range...
	if l == 0 && end[0] == 0 {
		end = nil
	}

	return proposedPrefix, end
}

// IsOnBlacklist checks that the Ethereum Address is black listed.
func (k *Keeper) IsOnBlacklist(ctx sdk.Context, addr common.Address) bool {
	defer k.Meter(ctx).FuncTiming(&ctx, "IsOnBlacklist")()

	return k.getStore(ctx).Has(types.GetEthereumBlacklistStoreKey(addr))
}

// SetEthereumBlacklistAddress sets the ethereum blacklist address.
func (k *Keeper) SetEthereumBlacklistAddress(ctx sdk.Context, addr common.Address) {
	defer k.Meter(ctx).FuncTiming(&ctx, "SetEthereumBlacklistAddress")()

	// set boolean indicator
	k.getStore(ctx).Set(types.GetEthereumBlacklistStoreKey(addr), []byte{})
}

// GetAllEthereumBlacklistAddresses fetches all etheruem blacklisted addresses.
func (k *Keeper) GetAllEthereumBlacklistAddresses(ctx sdk.Context) []string {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetAllEthereumBlacklistAddresses")()

	blacklistedAddresses := make([]string, 0)
	blacklistAddressStore := prefix.NewStore(k.getStore(ctx), types.EthereumBlacklistKey)
	chaintypes.IterateKeysSafe(blacklistAddressStore.Iterator(nil, nil), func(key []byte) bool {
		blacklistedAddresses = append(blacklistedAddresses, common.BytesToAddress(key).String())
		return false
	})

	return blacklistedAddresses
}

// DeleteEthereumBlacklistAddress deletes the address from blacklist.
func (k *Keeper) DeleteEthereumBlacklistAddress(ctx sdk.Context, addr common.Address) {
	defer k.Meter(ctx).FuncTiming(&ctx, "DeleteEthereumBlacklistAddress")()

	k.getStore(ctx).Delete(types.GetEthereumBlacklistStoreKey(addr))
}

// InvalidSendToEthAddress Returns true if the provided address is invalid to send to Ethereum this could be
// for one of several reasons. (1) it is invalid in general like the Zero address, (2)
// it is invalid for a subset of ERC20 addresses or (3) it is on the governance deposit/withdraw
// blacklist. (2) is not yet implemented
// Blocking some addresses is technically motivated, if any ERC20 transfers in a batch fail the entire batch
// becomes impossible to execute.
func (k *Keeper) InvalidSendToEthAddress(ctx sdk.Context, addr common.Address) bool {
	return k.IsOnBlacklist(ctx, addr) || addr == types.ZeroAddress()
}

func (k *Keeper) isAdmin(ctx sdk.Context, addr string) bool {
	for _, adminAddress := range k.GetParams(ctx).Admins {
		if adminAddress == addr {
			return true
		}
	}
	return false
}

// CreateModuleAccount creates a module account with minting and burning capabilities
func (k *Keeper) CreateModuleAccount(ctx sdk.Context) {
	baseAcc := authtypes.NewEmptyModuleAccount(types.ModuleName, authtypes.Minter, authtypes.Burner)
	moduleAcc := (k.accountKeeper.NewAccount(ctx, baseAcc)).(sdk.ModuleAccountI) // set the account number
	k.accountKeeper.SetModuleAccount(ctx, moduleAcc)
}

// ResetPeggyModuleState is triggered whenever the Peggy.sol contract has moved to another network making
// (most) of the module state obsolete. Valset Updates are preserved since that's coming from Peggy to Ethereum
func (k *Keeper) ResetPeggyModuleState(ctx sdk.Context, params *types.Params) error {
	defer k.Meter(ctx).FuncTiming(&ctx, "ResetPeggyModuleState")()

	height := params.GetBridgeContractStartHeight()
	lastObservedEventNonce := k.GetLastObservedEventNonce(ctx)
	updateValidatorsClaims := func(_ int64, validator stakingtypes.ValidatorI) (stop bool) {
		v, _ := sdk.ValAddressFromBech32(validator.GetOperator())

		if validator.IsBonded() {
			// Align any active validator's last nonce to match the last observed.
			// That way each of them can start up properly because the new Peggy.sol contract
			// should have its state_lastEventNonce set to lastObservedEventNonce
			k.setLastEventByValidator(ctx, v, lastObservedEventNonce, height)
		} else {
			// Inactive validators will have their last claim set properly during AfterValidatorBonded hook
			k.setLastEventByValidator(ctx, v, 0, 0)
		}

		return false
	}

	// 1. update last observed claims for all validators
	if err := k.StakingKeeper.IterateValidators(ctx, updateValidatorsClaims); err != nil {
		return err
	}

	// 2. cancel all batches in outgoing pool
	// (regardless if they might be executed because we're changing Eth networks)
	for _, batch := range k.GetOutgoingTxBatches(ctx) {
		// can't error
		_ = k.CancelOutgoingTXBatch(ctx, common.HexToAddress(batch.TokenContract), batch.BatchNonce)
	}

	// 3. cancel all withdrawals (old peggy denoms are unusable)
	for _, tx := range k.GetPoolTransactions(ctx) {
		// can't error
		_ = k.RemoveFromOutgoingPoolAndRefund(ctx, tx.Id, sdk.MustAccAddressFromBech32(tx.Sender))
	}

	// 4. remove all attestations
	attestationsMapping := k.GetAttestationMapping(ctx)

	sortedNonces := make([]uint64, 0, len(attestationsMapping))
	for k := range attestationsMapping {
		sortedNonces = append(sortedNonces, k)
	}
	sort.SliceStable(sortedNonces, func(i, j int) bool { return sortedNonces[i] < sortedNonces[j] })

	for _, nonce := range sortedNonces {
		for _, attestation := range attestationsMapping[nonce] {
			k.DeleteAttestation(ctx, attestation)
		}
	}

	return nil
}

func (k *Keeper) GetAllRateLimitTransfers(ctx sdk.Context) []*types.RateLimitTransfers {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetAllRateLimitTransfers")()

	var (
		iterInflow   = k.getStore(ctx).Iterator(PrefixRange(types.RateLimitInflowKey))
		iterOutflow  = k.getStore(ctx).Iterator(PrefixRange(types.RateLimitOutflowKey))
		allTransfers = make(map[common.Address]*types.RateLimitTransfers)
	)

	collectRecord := func(k, v []byte, inflow bool) {
		token := common.BytesToAddress(k[1:21])          // prefix byte + contract addr + uint64 block
		blockNumber := binary.BigEndian.Uint64(k[21:29]) // uint64

		var amount math.Int
		if err := amount.Unmarshal(v); err != nil {
			panic("failed to unmarshal amount: " + err.Error())
		}

		transfers, ok := allTransfers[token]
		if !ok {
			transfers = &types.RateLimitTransfers{Token: token.Hex()}
			allTransfers[token] = transfers
		}

		record := &types.BlockTransferRecord{
			BlockNumber: blockNumber,
			Amount:      amount,
		}

		if inflow {
			transfers.Inflows = append(transfers.Inflows, record)
		} else {
			transfers.Outflows = append(transfers.Outflows, record)
		}
	}

	chaintypes.IterateSafe(iterInflow, func(k, v []byte) (stop bool) {
		collectRecord(k, v, true)
		return false
	})

	chaintypes.IterateSafe(iterOutflow, func(k, v []byte) (stop bool) {
		collectRecord(k, v, false)
		return false
	})

	tokens := make([]common.Address, 0, len(allTransfers))
	for token := range allTransfers {
		tokens = append(tokens, token)
	}

	sort.SliceStable(tokens, func(i, j int) bool {
		return bytes.Compare(tokens[i].Bytes(), tokens[j].Bytes()) < 0
	})

	transfers := make([]*types.RateLimitTransfers, 0, len(tokens))
	for _, token := range tokens {
		transfers = append(transfers, allTransfers[token])
	}

	return transfers
}

// For the time being we have a lot of old records, so to avoid a longer Endblocker
// we cap the number keys to collect from the store and keep things running smooth
const maxRecordsToClean = 100

func (k *Keeper) PruneOldValsetConfirms(ctx sdk.Context) {
	defer k.Meter(ctx).FuncTiming(&ctx, "PruneOldValsetConfirms")()

	confirmationsToRemove := make([][]byte, 0, maxRecordsToClean)
	latestValsetNonceToPrune := k.GetLastJailedValsetNonce(ctx)

	vcStore := prefix.NewStore(ctx.KVStore(k.storeKey), types.ValsetConfirmKey)
	chaintypes.IterateKeysSafe(vcStore.Iterator(nil, nil), func(key []byte) bool {
		valsetNonce := binary.BigEndian.Uint64(key[:8])
		if valsetNonce >= latestValsetNonceToPrune {
			return true
		}

		confirmationsToRemove = append(confirmationsToRemove, bytes.Clone(key))

		return len(confirmationsToRemove) == maxRecordsToClean
	})

	for _, key := range confirmationsToRemove {
		vcStore.Delete(key)
	}
}

func (k *Keeper) PruneOldBatchConfirms(ctx sdk.Context) {
	defer k.Meter(ctx).FuncTiming(&ctx, "PruneOldBatchConfirms")()

	// For pruning batch confirms we need to be sure the batch is no longer there.
	// Jailing for missing batch confirms only applies if the batch exists (see batchJailing)
	// In other words, if a batch does not exist for some nonce, it is safe to remove
	confirmationsToRemove := make([][]byte, 0, maxRecordsToClean)
	existingBatchNonces := make(map[uint64]struct{})
	for _, batch := range k.GetOutgoingTxBatches(ctx) {
		existingBatchNonces[batch.BatchNonce] = struct{}{}
	}

	bcStore := prefix.NewStore(ctx.KVStore(k.storeKey), types.BatchConfirmKey)
	chaintypes.IterateKeysSafe(bcStore.Iterator(nil, nil), func(key []byte) bool {
		batchNonce := binary.BigEndian.Uint64(key[20:28])
		if _, ok := existingBatchNonces[batchNonce]; ok {
			return false // skip as this batch still exists
		}

		// batch does not exist -> confirmation can be removed
		confirmationsToRemove = append(confirmationsToRemove, bytes.Clone(key))
		return len(confirmationsToRemove) == maxRecordsToClean
	})

	for _, key := range confirmationsToRemove {
		bcStore.Delete(key)
	}
}
