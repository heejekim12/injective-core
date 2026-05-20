package keeper

import (
	"bytes"
	"fmt"

	"cosmossdk.io/errors"
	sdkmath "cosmossdk.io/math"
	"cosmossdk.io/store/prefix"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"

	"github.com/InjectiveLabs/injective-core/injective-chain/modules/peggy/types"
	chaintypes "github.com/InjectiveLabs/injective-core/injective-chain/types"
)

const OutgoingTxBatchSize = 100

// BuildOutgoingTXBatch starts the following process chain:
//   - find bridged denominator for given voucher type
//   - select available transactions from the outgoing transaction pool sorted by fee desc
//   - persist an outgoing batch object with an incrementing ID = nonce
//   - emit an event
func (k *Keeper) BuildOutgoingTXBatch(ctx sdk.Context, contractAddress common.Address, maxElements int) (*types.OutgoingTxBatch, error) {
	defer k.Meter(ctx).FuncTiming(&ctx, "BuildOutgoingTXBatch")()

	if maxElements == 0 {
		return nil, errors.Wrap(types.ErrInvalid, "max elements value")
	}

	selectedTx, err := k.pickUnbatchedTX(ctx, contractAddress, maxElements)
	if err != nil {
		return nil, err
	}

	if err := k.CheckRateLimit(ctx, contractAddress, selectedTx); err != nil {
		return nil, errors.Wrapf(err, "failed rate limit check")
	}

	nextID := k.AutoIncrementID(ctx, types.KeyLastOutgoingBatchID)
	batch := &types.OutgoingTxBatch{
		BatchNonce:    nextID,
		BatchTimeout:  k.getBatchTimeoutHeight(ctx),
		Transactions:  selectedTx,
		TokenContract: contractAddress.Hex(),
	}
	k.StoreBatch(ctx, batch)

	// Get the checkpoint and store it as a legit past batch
	checkpoint := batch.GetCheckpoint(k.GetPeggyID(ctx))
	k.SetPastEthSignatureCheckpoint(ctx, checkpoint)

	return batch, nil
}

// / This gets the batch timeout height in Ethereum blocks.
func (k *Keeper) getBatchTimeoutHeight(ctx sdk.Context) uint64 {
	defer k.Meter(ctx).FuncTiming(&ctx, "getBatchTimeoutHeight")()

	params := k.GetParams(ctx)
	currentCosmosHeight := ctx.BlockHeight()
	// we store the last observed Cosmos and Ethereum heights, we do not concern ourselves if these values
	// are zero because no batch can be produced if the last Ethereum block height is not first populated by a deposit event.
	heights := k.GetLastObservedEthereumBlockHeight(ctx)
	if heights.CosmosBlockHeight == 0 || heights.EthereumBlockHeight == 0 {
		return 0
	}
	// we project how long it has been in milliseconds since the last Ethereum block height was observed
	projectedMillis := (uint64(currentCosmosHeight) - heights.CosmosBlockHeight) * params.AverageBlockTime
	// we convert that projection into the current Ethereum height using the average Ethereum block time in millis
	projectedCurrentEthereumHeight := (projectedMillis / params.AverageEthereumBlockTime) + heights.EthereumBlockHeight
	// we convert our target time for block timeouts (lets say 12 hours) into a number of blocks to
	// place on top of our projection of the current Ethereum block height.
	blocksToAdd := params.TargetBatchTimeout / params.AverageEthereumBlockTime

	return projectedCurrentEthereumHeight + blocksToAdd
}

// OutgoingTxBatchExecuted is run when the Cosmos chain detects that a batch has been executed on Ethereum
// It frees all the transactions in the batch, then cancels all earlier batches, this function panics instead
// of returning errors because any failure will cause a double spend.
func (k *Keeper) OutgoingTxBatchExecuted(ctx sdk.Context, tokenContract common.Address, nonce uint64) {
	defer k.Meter(ctx).FuncTiming(&ctx, "OutgoingTxBatchExecuted")()

	b := k.GetOutgoingTXBatch(ctx, tokenContract, nonce)
	if b == nil {
		panic(fmt.Sprintf("unknown batch nonce for outgoing tx batch %s %d", tokenContract, nonce))
	}

	isCosmosOriginated, denom := k.ERC20ToDenomLookup(ctx, tokenContract)
	ev := &types.EventWithdrawalsCompleted{
		Denom:       denom,
		Withdrawals: make([]*types.Withdrawal, 0, len(b.Transactions)),
	}

	// cleanup outgoing TX pool, while these transactions where hidden from GetPoolTransactions
	// they still exist in the pool and need to be cleaned up.
	totalAmountWithdrawn := sdkmath.NewInt(0)
	for _, tx := range b.Transactions {
		totalAmountWithdrawn = totalAmountWithdrawn.Add(tx.Erc20Fee.Amount)
		totalAmountWithdrawn = totalAmountWithdrawn.Add(tx.Erc20Token.Amount)
		k.removePoolEntry(ctx, tx.Id)
		ev.Withdrawals = append(ev.Withdrawals, &types.Withdrawal{
			Sender:   tx.Sender,
			Receiver: tx.DestAddress,
			Amount:   tx.Erc20Token.Amount,
		})
	}

	// Iterate through remaining batches
	batchesToCancel := make([]*types.OutgoingTxBatch, 0)
	k.IterateOutgoingTXBatches(ctx, func(_ []byte, batch *types.OutgoingTxBatch) bool {
		// If the iterated batches nonce is lower than the one that was just executed, cancel it
		if batch.BatchNonce < b.BatchNonce && common.HexToAddress(batch.TokenContract) == tokenContract {
			batchesToCancel = append(batchesToCancel, batch)
		}

		return false
	})

	// cancel timed out batches
	for _, batch := range batchesToCancel {
		if err := k.CancelOutgoingTXBatch(ctx, tokenContract, batch.BatchNonce); err != nil {
			panic(fmt.Sprintf("Failed cancel out batch %s %d while trying to execute %s %d with %s", tokenContract, batch.BatchNonce, tokenContract, nonce, err))
		}
	}

	// Delete batch since it is finished
	k.DeleteBatch(ctx, *b)

	k.TrackTokenOutflow(ctx, tokenContract, totalAmountWithdrawn)

	// decrement tracked erc20 mint amount
	if !isCosmosOriginated && denom != chaintypes.InjectiveCoin {
		k.SetMintAmountERC20(ctx, tokenContract, k.GetMintAmountERC20(ctx, tokenContract).Sub(totalAmountWithdrawn))
	}

	_ = ctx.EventManager().EmitTypedEvent(ev)
}

// StoreBatch stores a transaction batch
func (k *Keeper) StoreBatch(ctx sdk.Context, batch *types.OutgoingTxBatch) {
	defer k.Meter(ctx).FuncTiming(&ctx, "StoreBatch")()

	store := ctx.KVStore(k.storeKey)
	// set the current block height when storing the batch
	batch.Block = uint64(ctx.BlockHeight())
	key := types.GetOutgoingTxBatchKey(common.HexToAddress(batch.TokenContract), batch.BatchNonce)
	store.Set(key, k.cdc.MustMarshal(batch))

	blockKey := types.GetOutgoingTxBatchBlockKey(batch.Block)
	store.Set(blockKey, k.cdc.MustMarshal(batch))
}

// StoreBatchUnsafe stores a transaction batch w/o setting the height
func (k *Keeper) StoreBatchUnsafe(ctx sdk.Context, batch *types.OutgoingTxBatch) {
	defer k.Meter(ctx).FuncTiming(&ctx, "StoreBatchUnsafe")()

	store := ctx.KVStore(k.storeKey)
	key := types.GetOutgoingTxBatchKey(common.HexToAddress(batch.TokenContract), batch.BatchNonce)
	store.Set(key, k.cdc.MustMarshal(batch))

	blockKey := types.GetOutgoingTxBatchBlockKey(batch.Block)
	store.Set(blockKey, k.cdc.MustMarshal(batch))

	// make sure transactions are indexed with OutgoingTXPoolKey
	for _, tx := range batch.Transactions {
		if err := k.setPoolEntry(ctx, tx); err != nil {
			panic("cannot index batch tx")
		}
	}
}

// DeleteBatch deletes an outgoing transaction batch
func (k *Keeper) DeleteBatch(ctx sdk.Context, batch types.OutgoingTxBatch) {
	defer k.Meter(ctx).FuncTiming(&ctx, "DeleteBatch")()

	store := ctx.KVStore(k.storeKey)
	store.Delete(types.GetOutgoingTxBatchKey(common.HexToAddress(batch.TokenContract), batch.BatchNonce))
	store.Delete(types.GetOutgoingTxBatchBlockKey(batch.Block))
}

// pickUnbatchedTX find TX in pool and remove from "available" second index
func (k *Keeper) pickUnbatchedTX(ctx sdk.Context, contractAddress common.Address, maxElements int) ([]*types.OutgoingTransferTx, error) {
	defer k.Meter(ctx).FuncTiming(&ctx, "pickUnbatchedTX")()

	unbatchedTxs := make([]*types.OutgoingTransferTx, 0)
	k.IterateTokenTxsByFee(ctx, contractAddress, func(_ sdkmath.Int, txID uint64) bool {
		tx, err := k.getPoolEntry(ctx, txID)
		if err != nil {
			panic("failed to get pool entry but fee is indexed: " + err.Error())
		}

		unbatchedTxs = append(unbatchedTxs, tx)
		return len(unbatchedTxs) == maxElements
	})

	if len(unbatchedTxs) == 0 {
		return nil, types.ErrNoUnbatchedTxsFound
	}

	// clear these out from the fee index
	for _, tx := range unbatchedTxs {
		k.DeleteOutgoingTxFee(ctx, common.HexToAddress(tx.Erc20Fee.Contract), tx.Erc20Fee, tx.Id)
	}

	return unbatchedTxs, nil
}

// GetOutgoingTXBatch loads a batch object. Returns nil when not exists.
func (k *Keeper) GetOutgoingTXBatch(ctx sdk.Context, tokenContract common.Address, nonce uint64) *types.OutgoingTxBatch {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetOutgoingTXBatch")()

	store := ctx.KVStore(k.storeKey)
	key := types.GetOutgoingTxBatchKey(tokenContract, nonce)
	bz := store.Get(key)
	if len(bz) == 0 {
		return nil
	}

	var b types.OutgoingTxBatch
	k.cdc.MustUnmarshal(bz, &b)
	for _, tx := range b.Transactions {
		tx.Erc20Token.Contract = tokenContract.Hex()
		tx.Erc20Fee.Contract = tokenContract.Hex()
	}

	return &b
}

// CancelOutgoingTXBatch releases all TX in the batch and deletes the batch
func (k *Keeper) CancelOutgoingTXBatch(ctx sdk.Context, tokenContract common.Address, nonce uint64) error {
	defer k.Meter(ctx).FuncTiming(&ctx, "CancelOutgoingTXBatch")()

	batch := k.GetOutgoingTXBatch(ctx, tokenContract, nonce)
	if batch == nil {
		return types.ErrUnknown
	}

	for _, tx := range batch.Transactions {
		tx.Erc20Fee.Contract = tokenContract.Hex()
		k.SetOutgoingTxFee(ctx, tokenContract, tx.Erc20Fee, tx.Id)
	}

	// Delete batch since it is finished
	k.DeleteBatch(ctx, *batch)

	// nolint:errcheck //ignored on purpose
	ctx.EventManager().EmitTypedEvent(&types.EventOutgoingBatchCanceled{
		BridgeContract: k.GetBridgeContractAddress(ctx).Hex(),
		BridgeChainId:  k.GetBridgeChainID(ctx),
		BatchId:        nonce,
		Nonce:          nonce,
	})

	return nil
}

// IterateOutgoingTXBatches iterates through all outgoing batches in DESC order.
func (k *Keeper) IterateOutgoingTXBatches(ctx sdk.Context, cb func(key []byte, batch *types.OutgoingTxBatch) bool) {
	defer k.Meter(ctx).FuncTiming(&ctx, "IterateOutgoingTXBatches")()

	outgoingBatchStore := prefix.NewStore(k.getStore(ctx), types.OutgoingTXBatchKey)
	chaintypes.IterateSafe(outgoingBatchStore.ReverseIterator(nil, nil), func(key, value []byte) (stop bool) {
		var batch types.OutgoingTxBatch
		k.cdc.MustUnmarshal(value, &batch)

		return cb(key, &batch)
	})
}

// GetOutgoingTxBatches returns the outgoing tx batches
func (k *Keeper) GetOutgoingTxBatches(ctx sdk.Context) (out []*types.OutgoingTxBatch) {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetOutgoingTxBatches")()

	k.IterateOutgoingTXBatches(ctx, func(_ []byte, batch *types.OutgoingTxBatch) bool {
		out = append(out, batch)
		return false
	})

	return
}

// GetLastOutgoingBatchByTokenType gets the latest outgoing tx batch by token type
func (k *Keeper) GetLastOutgoingBatchByTokenType(ctx sdk.Context, token common.Address) *types.OutgoingTxBatch {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetLastOutgoingBatchByTokenType")()

	batches := k.GetOutgoingTxBatches(ctx)
	var lastBatch *types.OutgoingTxBatch = nil
	lastNonce := uint64(0)

	for _, batch := range batches {
		if bytes.Equal(common.HexToAddress(batch.TokenContract).Bytes(), token.Bytes()) && batch.BatchNonce > lastNonce {
			lastBatch = batch
			lastNonce = batch.BatchNonce
		}
	}

	return lastBatch
}

// SetLastJailedBatchBlock sets the latest jailed batch block height.
func (k *Keeper) SetLastJailedBatchBlock(ctx sdk.Context, blockHeight uint64) {
	defer k.Meter(ctx).FuncTiming(&ctx, "SetLastJailedBatchBlock")()

	store := ctx.KVStore(k.storeKey)
	store.Set(types.LastJailedBatchBlock, types.UInt64Bytes(blockHeight))
}

// GetLastJailedBatchBlock returns the latest jailed batch block.
func (k *Keeper) GetLastJailedBatchBlock(ctx sdk.Context) uint64 {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetLastJailedBatchBlock")()

	store := ctx.KVStore(k.storeKey)
	storedBytes := store.Get(types.LastJailedBatchBlock)

	if len(storedBytes) == 0 {
		return 0
	}

	return types.UInt64FromBytes(storedBytes)
}

// GetUnjailedBatches returns all the unjailed batches in state
func (k *Keeper) GetUnjailedBatches(ctx sdk.Context, maxHeight uint64) (out []*types.OutgoingTxBatch) {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetUnjailedBatches")()

	lastJailedBatchBlock := k.GetLastJailedBatchBlock(ctx)
	k.IterateBatchByJailedBatchBlock(ctx, lastJailedBatchBlock, maxHeight, func(_ []byte, batch *types.OutgoingTxBatch) bool {
		if batch.Block > lastJailedBatchBlock {
			out = append(out, batch)
		}
		return false
	})

	return
}

// IterateBatchByJailedBatchBlock iterates through all Batch by last jailed Batch block in ASC order
func (k *Keeper) IterateBatchByJailedBatchBlock(ctx sdk.Context, lastJailedBatchBlock, maxHeight uint64, cb func([]byte, *types.OutgoingTxBatch) bool) {
	defer k.Meter(ctx).FuncTiming(&ctx, "IterateBatchByJailedBatchBlock")()

	start, end := types.UInt64Bytes(lastJailedBatchBlock), types.UInt64Bytes(maxHeight)
	outgoingBatchByBlockStore := prefix.NewStore(k.getStore(ctx), types.OutgoingTXBatchBlockKey)
	chaintypes.IterateSafe(outgoingBatchByBlockStore.Iterator(start, end),
		func(key, value []byte) (stop bool) {
			var batch types.OutgoingTxBatch
			k.cdc.MustUnmarshal(value, &batch)
			// cb returns true to stop early
			return cb(key, &batch)
		},
	)
}
