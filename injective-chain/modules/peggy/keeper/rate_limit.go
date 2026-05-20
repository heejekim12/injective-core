package keeper

import (
	"bytes"
	"encoding/binary"
	"errors"

	sdkerrors "cosmossdk.io/errors"
	sdkmath "cosmossdk.io/math"
	"cosmossdk.io/store/prefix"
	sdk "github.com/cosmos/cosmos-sdk/types"
	gethcommon "github.com/ethereum/go-ethereum/common"

	"github.com/InjectiveLabs/injective-core/injective-chain/modules/peggy/types"
	chaintypes "github.com/InjectiveLabs/injective-core/injective-chain/types"
)

var (
	ErrRateLimitOverflow         = errors.New("rate limit overflow")
	ErrAbsoluteMintLimitOverflow = errors.New("absolute mint limit overflow")
)

func (k *Keeper) CheckRateLimit(
	ctx sdk.Context,
	tokenAddress gethcommon.Address,
	newTxs []*types.OutgoingTransferTx,
) error {
	defer k.Meter(ctx).FuncTiming(&ctx, "CheckRateLimit")()

	rateLimit := k.GetRateLimit(ctx, tokenAddress)
	if rateLimit == nil {
		return nil // no-op
	}

	var (
		totalInBatches = sdkmath.ZeroInt()
		totalInNewTxs  = sdkmath.ZeroInt()
		activeOutflow  = k.GetNetOutflow(ctx, tokenAddress)
	)

	k.IterateOutgoingTXBatches(ctx, func(_ []byte, batch *types.OutgoingTxBatch) bool {
		if batch.TokenContract == tokenAddress.String() {
			for _, tx := range batch.Transactions {
				totalInBatches = totalInBatches.Add(tx.Erc20Fee.Amount)
				totalInBatches = totalInBatches.Add(tx.Erc20Token.Amount)
			}
		}

		return false
	})

	for _, tx := range newTxs {
		totalInNewTxs = totalInNewTxs.Add(tx.Erc20Fee.Amount)
		totalInNewTxs = totalInNewTxs.Add(tx.Erc20Token.Amount)
	}

	entireWithdrawAmountSoFar := activeOutflow.Add(totalInBatches).Add(totalInNewTxs)
	if thereIsMoreInThanOutgoing := !entireWithdrawAmountSoFar.IsPositive(); thereIsMoreInThanOutgoing {
		return nil
	}

	quantity := entireWithdrawAmountSoFar.ToLegacyDec()
	quantity = quantity.Quo(sdkmath.LegacyNewDec(10).Power(uint64(rateLimit.TokenDecimals)))

	// Pyth price IDs encode the quote denomination (e.g. a BTC/USD price ID always stores a USD price),
	// so PriceState.Price is already USD-denominated and can be compared directly to RateLimitUsd.
	pythPriceState := k.OracleKeeper.GetPythPriceState(ctx, gethcommon.HexToHash(rateLimit.TokenPriceId))
	if pythPriceState == nil || pythPriceState.PriceState.Price.IsNil() || !pythPriceState.PriceState.Price.IsPositive() {
		// todo(dusan): perform check during MsgServer CreateRateLimit?
		return errors.New("nil Pyth price")
	}

	notional := quantity.Mul(pythPriceState.PriceState.Price)
	if notional.GTE(rateLimit.RateLimitUsd) {
		return sdkerrors.Wrapf(ErrRateLimitOverflow, "configured limit: %sUSD", rateLimit.RateLimitUsd.String())
	}

	return nil
}

func (k *Keeper) TrackTokenInflow(ctx sdk.Context, tokenAddress gethcommon.Address, in sdkmath.Int) {
	defer k.Meter(ctx).FuncTiming(&ctx, "TrackTokenInflow")()

	if k.GetRateLimit(ctx, tokenAddress) == nil {
		return // no-op
	}

	blockNum := uint64(ctx.BlockHeight())
	inflow := k.GetInflowByBlock(ctx, tokenAddress, blockNum)
	k.SetInflowByBlock(ctx, tokenAddress, blockNum, inflow.Add(in))
}

func (k *Keeper) TrackTokenOutflow(ctx sdk.Context, tokenAddress gethcommon.Address, out sdkmath.Int) {
	defer k.Meter(ctx).FuncTiming(&ctx, "TrackTokenOutflow")()

	if k.GetRateLimit(ctx, tokenAddress) == nil {
		return // no-op
	}

	blockNum := uint64(ctx.BlockHeight())
	outflow := k.GetOutflowByBlock(ctx, tokenAddress, blockNum)
	k.SetOutflowByBlock(ctx, tokenAddress, blockNum, outflow.Add(out))
}

func (k *Keeper) SetRateLimit(ctx sdk.Context, rateLimit *types.RateLimit) {
	defer k.Meter(ctx).FuncTiming(&ctx, "SetRateLimit")()

	key := types.GetRateLimitKey(gethcommon.HexToAddress(rateLimit.TokenAddress).Bytes())
	value := k.cdc.MustMarshal(rateLimit)
	k.getStore(ctx).Set(key, value)
}

func (k *Keeper) GetRateLimit(ctx sdk.Context, tokenAddress gethcommon.Address) *types.RateLimit {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetRateLimit")()

	bz := k.getStore(ctx).Get(types.GetRateLimitKey(tokenAddress.Bytes()))
	if len(bz) == 0 {
		return nil
	}

	var rateLimit types.RateLimit
	k.cdc.MustUnmarshal(bz, &rateLimit)

	return &rateLimit
}

func (k *Keeper) HasRateLimit(ctx sdk.Context, token gethcommon.Address) bool {
	defer k.Meter(ctx).FuncTiming(&ctx, "HasRateLimit")()

	return k.getStore(ctx).Has(types.GetRateLimitKey(token.Bytes()))
}

func (k *Keeper) DeleteRateLimit(ctx sdk.Context, tokenAddress gethcommon.Address) {
	defer k.Meter(ctx).FuncTiming(&ctx, "DeleteRateLimit")()

	k.getStore(ctx).Delete(types.GetRateLimitKey(tokenAddress.Bytes()))
}

func (k *Keeper) GetRateLimits(ctx sdk.Context) []*types.RateLimit {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetRateLimits")()

	rateLimits := make([]*types.RateLimit, 0)
	rateLimitsStore := prefix.NewStore(k.getStore(ctx), types.RateLimitKey)
	chaintypes.IterateSafe(rateLimitsStore.Iterator(nil, nil), func(_, value []byte) (stop bool) {
		var rateLimit types.RateLimit
		k.cdc.MustUnmarshal(value, &rateLimit)

		rateLimits = append(rateLimits, &rateLimit)
		return false
	})

	return rateLimits
}

func (k *Keeper) GetNonExistentRateLimitTokenAddresses(ctx sdk.Context) []gethcommon.Address {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetNonExistentRateLimitTokenAddresses")()

	// rate limit that was previously removed still contains its net outflow value (in case it is ever recreated)
	// so the token address can still be picked up from net outflow key store
	tokens := make([]gethcommon.Address, 0)
	netOutflowStore := prefix.NewStore(k.getStore(ctx), types.RateLimitNetOutflowKey)
	chaintypes.IterateKeysSafe(netOutflowStore.Iterator(nil, nil), func(key []byte) (stop bool) {
		if token := gethcommon.BytesToAddress(key); !k.HasRateLimit(ctx, token) {
			tokens = append(tokens, token)
		}

		return false
	})

	return tokens
}

func (k *Keeper) GetNetOutflow(ctx sdk.Context, tokenAddress gethcommon.Address) sdkmath.Int {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetNetOutflow")()

	store := k.getStore(ctx)
	bz := store.Get(types.GetTokenNetOutflowKey(tokenAddress.Bytes()))
	if len(bz) == 0 {
		return sdkmath.ZeroInt()
	}

	var amount sdkmath.Int
	if err := amount.Unmarshal(bz); err != nil {
		panic("failed to unmarshal rate limit total net outflow: " + err.Error())
	}

	return amount
}

func (k *Keeper) SetNetOutflow(ctx sdk.Context, tokenAddress gethcommon.Address, amount sdkmath.Int) {
	defer k.Meter(ctx).FuncTiming(&ctx, "SetNetOutflow")()

	bz, err := amount.Marshal()
	if err != nil {
		panic("failed to marshal rate limit total net outflow: " + err.Error())
	}

	key := types.GetTokenNetOutflowKey(tokenAddress.Bytes())
	k.getStore(ctx).Set(key, bz)
}

func (k *Keeper) GetInflowByBlock(ctx sdk.Context, tokenAddress gethcommon.Address, blockNum uint64) sdkmath.Int {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetInflowByBlock")()

	store := k.getStore(ctx)
	bz := store.Get(types.GetTokenInflowByBlockKey(tokenAddress.Bytes(), blockNum))
	if len(bz) == 0 {
		return sdkmath.ZeroInt()
	}

	var amount sdkmath.Int
	if err := amount.Unmarshal(bz); err != nil {
		panic("failed to unmarshal rate limit outflow: " + err.Error())
	}

	return amount
}

func (k *Keeper) SetInflowByBlock(ctx sdk.Context, tokenAddress gethcommon.Address, blockNum uint64, amount sdkmath.Int) {
	defer k.Meter(ctx).FuncTiming(&ctx, "SetInflowByBlock")()

	store := k.getStore(ctx)
	key := types.GetTokenInflowByBlockKey(tokenAddress.Bytes(), blockNum)
	if amount.IsZero() {
		store.Delete(key)
		return
	}

	bz, err := amount.Marshal()
	if err != nil {
		panic("failed to marshal rate limit outflow: " + err.Error())
	}

	store.Set(key, bz)
}

func (k *Keeper) GetOutflowByBlock(ctx sdk.Context, tokenAddress gethcommon.Address, blockNum uint64) sdkmath.Int {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetOutflowByBlock")()

	store := k.getStore(ctx)
	bz := store.Get(types.GetTokenOutflowByBlockKey(tokenAddress.Bytes(), blockNum))
	if len(bz) == 0 {
		return sdkmath.ZeroInt()
	}

	var amount sdkmath.Int
	if err := amount.Unmarshal(bz); err != nil {
		panic("failed to unmarshal rate limit outflow: " + err.Error())
	}

	return amount
}

func (k *Keeper) SetOutflowByBlock(ctx sdk.Context, tokenAddress gethcommon.Address, blockNum uint64, amount sdkmath.Int) {
	defer k.Meter(ctx).FuncTiming(&ctx, "SetOutflowByBlock")()

	store := k.getStore(ctx)
	key := types.GetTokenOutflowByBlockKey(tokenAddress.Bytes(), blockNum)
	if amount.IsZero() {
		store.Delete(key)
		return
	}

	bz, err := amount.Marshal()
	if err != nil {
		panic("failed to marshal rate limit outflow: " + err.Error())
	}

	store.Set(key, bz)
}

func (k *Keeper) GetOutdatedInflow(ctx sdk.Context, tokenAddress gethcommon.Address, endBlock uint64) sdkmath.Int {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetOutdatedInflow")()

	totalOutdatedInflow := sdkmath.ZeroInt()
	cb := func(blockNum uint64, amount sdkmath.Int) bool {
		if blockNum >= endBlock {
			return true
		}

		totalOutdatedInflow = totalOutdatedInflow.Add(amount)
		return false
	}

	inflowStore := prefix.NewStore(k.getStore(ctx), types.GetTokenInflowPrefix(tokenAddress.Bytes()))
	iterateFlowRecordStore(inflowStore, cb)

	return totalOutdatedInflow
}

func (k *Keeper) GetOutdatedOutflow(ctx sdk.Context, tokenAddress gethcommon.Address, endBlock uint64) sdkmath.Int {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetOutdatedOutflow")()

	totalOutdatedOutflow := sdkmath.ZeroInt()
	cb := func(blockNum uint64, amount sdkmath.Int) bool {
		if blockNum >= endBlock {
			return true
		}

		totalOutdatedOutflow = totalOutdatedOutflow.Add(amount)
		return false
	}

	outflowStore := prefix.NewStore(k.getStore(ctx), types.GetTokenOutflowPrefix(tokenAddress.Bytes()))
	iterateFlowRecordStore(outflowStore, cb)

	return totalOutdatedOutflow
}

func iterateFlowRecordStore(store prefix.Store, cb func(blockNum uint64, amount sdkmath.Int) bool) {
	chaintypes.IterateSafe(store.Iterator(nil, nil), func(k, v []byte) bool {
		var amount sdkmath.Int
		if err := amount.Unmarshal(v); err != nil {
			panic("failed to unmarshal rate limit outflow: " + err.Error())
		}

		return cb(binary.BigEndian.Uint64(k), amount)
	})
}

func (k *Keeper) PruneOldInflows(ctx sdk.Context, tokenAddress gethcommon.Address, endBlock uint64) {
	defer k.Meter(ctx).FuncTiming(&ctx, "PruneOldInflows")()

	store := k.getStore(ctx)
	inflowStore := prefix.NewStore(store, types.GetTokenInflowPrefix(tokenAddress.Bytes()))
	for _, key := range collectOldBlockKeys(inflowStore, endBlock) {
		inflowStore.Delete(key)
	}
}

func (k *Keeper) PruneOldOutflows(ctx sdk.Context, tokenAddress gethcommon.Address, endBlock uint64) {
	defer k.Meter(ctx).FuncTiming(&ctx, "PruneOldOutflows")()

	store := k.getStore(ctx)
	outflowStore := prefix.NewStore(store, types.GetTokenOutflowPrefix(tokenAddress.Bytes()))
	for _, key := range collectOldBlockKeys(outflowStore, endBlock) {
		outflowStore.Delete(key)
	}
}

func collectOldBlockKeys(store prefix.Store, endBlock uint64) [][]byte {
	keysToRemove := make([][]byte, 0)
	chaintypes.IterateKeysSafe(store.Iterator(nil, nil), func(k []byte) (stop bool) {
		blockNum := binary.BigEndian.Uint64(k)
		if blockNum >= endBlock {
			return true
		}

		keysToRemove = append(keysToRemove, bytes.Clone(k))
		return false
	})

	return keysToRemove
}

const maxRecordsToPrune = 100

func (k *Keeper) PruneInflowForNonExistentRateLimitToken(ctx sdk.Context, token gethcommon.Address) {
	defer k.Meter(ctx).FuncTiming(&ctx, "PruneInflowForNonExistentRateLimitToken")()

	var (
		inflowKeysToRemove = make([][]byte, 0, maxRecordsToPrune)
		totalInflow        = sdkmath.ZeroInt()
		tokenInflowStore   = prefix.NewStore(k.getStore(ctx), types.GetTokenInflowPrefix(token.Bytes()))
	)

	chaintypes.IterateSafe(tokenInflowStore.Iterator(nil, nil), func(k, v []byte) (stop bool) {
		var amount sdkmath.Int
		if err := amount.Unmarshal(v); err != nil {
			panic("failed to unmarshal rate limit outflow: " + err.Error())
		}

		totalInflow = totalInflow.Add(amount)
		inflowKeysToRemove = append(inflowKeysToRemove, bytes.Clone(k))

		return len(inflowKeysToRemove) >= maxRecordsToPrune
	})

	for _, key := range inflowKeysToRemove {
		tokenInflowStore.Delete(key)
	}

	// inflow is added here because it was subtracted in the original net outflow calculation (end blocker)
	newNetOutflow := k.GetNetOutflow(ctx, token).Add(totalInflow)
	k.SetNetOutflow(ctx, token, newNetOutflow)
}

func (k *Keeper) PruneOutflowForNonExistingRateLimitToken(ctx sdk.Context, token gethcommon.Address) {
	defer k.Meter(ctx).FuncTiming(&ctx, "PruneOutflowForNonExistingRateLimitToken")()

	var (
		outflowKeysToRemove = make([][]byte, 0, maxRecordsToPrune)
		totalOutflow        = sdkmath.ZeroInt()
		tokenOutflowStore   = prefix.NewStore(k.getStore(ctx), types.GetTokenOutflowPrefix(token.Bytes()))
	)

	chaintypes.IterateSafe(tokenOutflowStore.Iterator(nil, nil), func(k, v []byte) (stop bool) {
		var amount sdkmath.Int
		if err := amount.Unmarshal(v); err != nil {
			panic("failed to unmarshal rate limit outflow: " + err.Error())
		}

		totalOutflow = totalOutflow.Add(amount)
		outflowKeysToRemove = append(outflowKeysToRemove, bytes.Clone(k))

		return len(outflowKeysToRemove) >= maxRecordsToPrune
	})

	for _, key := range outflowKeysToRemove {
		tokenOutflowStore.Delete(key)
	}

	// outflow is subtracted here because it was added in the original net outflow calculation (begin/end blocker)
	newNetOutflow := k.GetNetOutflow(ctx, token).Sub(totalOutflow)
	k.SetNetOutflow(ctx, token, newNetOutflow)
}
