package keeper

import (
	"encoding/binary"
	"math/big"
	"sort"

	"cosmossdk.io/errors"
	"cosmossdk.io/math"
	"cosmossdk.io/store/prefix"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"

	"github.com/InjectiveLabs/injective-core/injective-chain/modules/peggy/types"
	chaintypes "github.com/InjectiveLabs/injective-core/injective-chain/types"
)

// AddToOutgoingPool
// - checks a counterpart denominator exists for the given voucher type
// - burns the voucher for transfer amount and fees
// - persists an OutgoingTx
// - adds the TX to the `available` TX pool via a second index
func (k *Keeper) AddToOutgoingPool(ctx sdk.Context, sender sdk.AccAddress, counterpartReceiver common.Address, amount, fee sdk.Coin) (uint64, error) {
	defer k.Meter(ctx).FuncTiming(&ctx, "AddToOutgoingPool")()

	totalAmount := amount.Add(fee)
	totalInVouchers := sdk.Coins{totalAmount}

	// If the coin is a peggy voucher, burn the coins. If not, check if there is a deployed ERC20 contract representing it.
	// If there is, lock the coins.

	isCosmosOriginated, tokenContract, err := k.DenomToERC20Lookup(ctx, totalAmount.Denom)
	if err != nil {
		return 0, err
	}

	// If it is a cosmos-originated asset we lock it
	if isCosmosOriginated {
		// lock coins in module
		return 0, errors.Wrap(types.ErrUnsupported, "withdrawing Injective-native tokens is disabled")
	} else {
		// If it is an ethereum-originated asset we burn it
		// send coins to module in prep for burn
		if err := k.bankKeeper.SendCoinsFromAccountToModule(ctx, sender, types.ModuleName, totalInVouchers); err != nil {
			return 0, err
		}

		// burn vouchers to send them back to ETH
		if err := k.bankKeeper.BurnCoins(ctx, types.ModuleName, totalInVouchers); err != nil {
			panic(err)
		}
	}

	// get next tx id from keeper
	nextID := k.AutoIncrementID(ctx, types.KeyLastTXPoolID)
	erc20Fee := types.NewSDKIntERC20Token(fee.Amount, tokenContract)

	// construct outgoing tx, as part of this process we represent
	// the token as an ERC20 token since it is preparing to go to ETH
	// rather than the denom that is the input to this function.
	outgoing := &types.OutgoingTransferTx{
		Id:          nextID,
		Sender:      sender.String(),
		DestAddress: counterpartReceiver.Hex(),
		Erc20Token:  types.NewSDKIntERC20Token(amount.Amount, tokenContract),
		Erc20Fee:    erc20Fee,
	}

	// set the outgoing tx in the pool index
	if err := k.setPoolEntry(ctx, outgoing); err != nil {
		return 0, err
	}

	// add a fee index entry so unbatched txs can be selected by fee
	k.SetOutgoingTxFee(ctx, tokenContract, erc20Fee, nextID)

	return nextID, nil
}

// RemoveFromOutgoingPoolAndRefund
// - checks that the provided tx actually exists
// - deletes the unbatched tx from the pool
// - issues the tokens back to the sender
func (k *Keeper) RemoveFromOutgoingPoolAndRefund(ctx sdk.Context, txId uint64, sender sdk.AccAddress) error {
	defer k.Meter(ctx).FuncTiming(&ctx, "RemoveFromOutgoingPoolAndRefund")()

	// check that we actually have a tx with that id and what it's details are
	tx, err := k.getPoolEntry(ctx, txId)
	if err != nil {
		return err
	}

	txSender, err := sdk.AccAddressFromBech32(tx.Sender)
	if err != nil {
		return err
	}

	if !sender.Equals(txSender) {
		return errors.Wrapf(types.ErrInvalid, "Invalid sender address")
	}

	// An inconsistent entry should never enter the store, but this is the ideal place to exploit
	// it such a bug if it did ever occur, so we should double check to be really sure
	if tx.Erc20Fee.Contract != tx.Erc20Token.Contract {
		return errors.Wrapf(types.ErrInvalid, "Inconsistent tokens to cancel!: %s %s", tx.Erc20Fee.Contract, tx.Erc20Token.Contract)
	}

	// delete this tx from both indexes
	tokenContract := common.HexToAddress(tx.Erc20Token.Contract)
	if !k.getStore(ctx).Has(types.GetFeeIndexKey(tokenContract, tx.Erc20Fee, txId)) {
		return errors.Wrapf(types.ErrInvalid, "txId %d is not in fee index", txId)
	}

	k.DeleteOutgoingTxFee(ctx, tokenContract, tx.Erc20Fee, txId)
	k.removePoolEntry(ctx, txId)

	// reissue the amount and the fee
	var totalToRefundCoins sdk.Coins
	isCosmosOriginated, denom := k.ERC20ToDenomLookup(ctx, tokenContract)
	// native cosmos coin denom
	if denom == k.GetCosmosCoinDenom(ctx) || isCosmosOriginated {
		// peggy denom
		totalToRefund := sdk.NewCoin(denom, tx.Erc20Token.Amount)
		totalToRefund.Amount = totalToRefund.Amount.Add(tx.Erc20Fee.Amount)
		totalToRefundCoins = sdk.NewCoins(totalToRefund)
	} else {
		// peggy denom
		totalToRefund := tx.Erc20Token.PeggyCoin()
		totalToRefund.Amount = totalToRefund.Amount.Add(tx.Erc20Fee.Amount)
		totalToRefundCoins = sdk.NewCoins(totalToRefund)
	}

	// If it is a cosmos-originated the coins are in the module (see AddToOutgoingPool) so we can just take them out
	if isCosmosOriginated {
		if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, sender, totalToRefundCoins); err != nil {
			return err
		}
	} else {
		// If it is an ethereum-originated asset we have to mint it (see Handle in attestation_handler.go)
		// mint coins in module for prep to send
		if err := k.bankKeeper.MintCoins(ctx, types.ModuleName, totalToRefundCoins); err != nil {
			return errors.Wrapf(err, "mint vouchers coins: %s", totalToRefundCoins)
		}
		if err = k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, sender, totalToRefundCoins); err != nil {
			return errors.Wrap(err, "transfer vouchers")
		}
	}

	// nolint:errcheck //ignored on purpose
	ctx.EventManager().EmitTypedEvent(&types.EventBridgeWithdrawCanceled{
		BridgeContract: k.GetBridgeContractAddress(ctx).Hex(),
		BridgeChainId:  k.GetBridgeChainID(ctx),
	})

	return nil
}

func (k *Keeper) setPoolEntry(ctx sdk.Context, outgoingTransferTx *types.OutgoingTransferTx) error {
	defer k.Meter(ctx).FuncTiming(&ctx, "setPoolEntry")()

	bz, err := k.cdc.Marshal(outgoingTransferTx)
	if err != nil {
		return err
	}

	store := ctx.KVStore(k.storeKey)
	store.Set(types.GetOutgoingTxPoolKey(outgoingTransferTx.Id), bz)

	return nil
}

// getPoolEntry grabs an entry from the tx pool, this *does* include transactions in batches
// so check the UnbatchedTxIndex or call GetPoolTransactions for that purpose
func (k *Keeper) getPoolEntry(ctx sdk.Context, id uint64) (*types.OutgoingTransferTx, error) {
	defer k.Meter(ctx).FuncTiming(&ctx, "getPoolEntry")()

	store := ctx.KVStore(k.storeKey)

	bz := store.Get(types.GetOutgoingTxPoolKey(id))
	if bz == nil {
		return nil, types.ErrUnknown
	}

	var r types.OutgoingTransferTx
	err := k.cdc.Unmarshal(bz, &r)

	if err != nil {
		return nil, err
	}

	return &r, nil
}

// removePoolEntry removes an entry from the tx pool, this *does* include transactions in batches
// so you will need to run it when cleaning up after a executed batch
func (k *Keeper) removePoolEntry(ctx sdk.Context, id uint64) {
	defer k.Meter(ctx).FuncTiming(&ctx, "removePoolEntry")()

	store := ctx.KVStore(k.storeKey)
	store.Delete(types.GetOutgoingTxPoolKey(id))
}

// GetPoolTransactions grabs unbatched transactions from the tx pool, useful for queries or genesis save/load.
// Transactions already placed in batches remain in the pool store, but are removed from the fee index.
func (k *Keeper) GetPoolTransactions(ctx sdk.Context) []*types.OutgoingTransferTx {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetPoolTransactions")()

	feeStore := prefix.NewStore(k.getStore(ctx), types.FeeIndexOutgoingTxKey)

	txs := make([]*types.OutgoingTransferTx, 0)
	chaintypes.IterateKeysSafe(feeStore.ReverseIterator(nil, nil), func(key []byte) (stop bool) {
		txID := binary.BigEndian.Uint64(key[52:])
		tx, err := k.getPoolEntry(ctx, txID)
		if err != nil {
			panic("failed to get pool entry but fee index exists: " + err.Error())
		}

		txs = append(txs, tx)
		return false
	})

	return txs
}

// GetAllBatchFees creates a fee entry for every batch type currently in the store
// this can be used by relayers to determine what batch types are desirable to request
func (k *Keeper) GetAllBatchFees(ctx sdk.Context) (batchFees []*types.BatchFees) {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetAllBatchFees")()

	batchFeesMap := k.createBatchFees(ctx)
	// create array of batchFees
	for _, batchFee := range batchFeesMap {
		batchFees = append(batchFees, batchFee)
	}

	// quick sort by token to make this function safe for use
	// in consensus computations
	sort.SliceStable(batchFees, func(i, j int) bool {
		return batchFees[i].Token < batchFees[j].Token
	})

	return batchFees
}

// CreateBatchFees iterates over the outgoing pool and creates batch token fee map
func (k *Keeper) createBatchFees(ctx sdk.Context) map[common.Address]*types.BatchFees {
	defer k.Meter(ctx).FuncTiming(&ctx, "createBatchFees")()

	batchFeesMap := make(map[common.Address]*types.BatchFees)
	batchSizeMap := make(map[common.Address]int)

	iter := prefix.NewStore(k.getStore(ctx), types.FeeIndexOutgoingTxKey)
	chaintypes.IterateKeysSafe(iter.ReverseIterator(nil, nil), func(key []byte) (stop bool) {
		token := common.BytesToAddress(key[:20])
		if batchSizeMap[token] >= OutgoingTxBatchSize {
			return false // skip
		}

		amount, ok := batchFeesMap[token]
		if !ok {
			amount = &types.BatchFees{
				Token:     token.Hex(),
				TotalFees: math.ZeroInt(),
			}
			batchFeesMap[token] = amount
		}

		feeAmount := math.NewIntFromBigInt(big.NewInt(0).SetBytes(key[20:52]))
		newAmount, err := amount.TotalFees.SafeAdd(feeAmount)
		if err != nil {
			k.Logger(ctx).Error("failed to add fee amount when creating batch fees",
				"token", token,
				"amount", amount.TotalFees.String(),
				"fee", feeAmount.String(),
				"err", err,
			)

			return true
		}

		amount.TotalFees = newAmount
		batchSizeMap[token]++

		return false
	})

	return batchFeesMap
}

func (k *Keeper) AutoIncrementID(ctx sdk.Context, idKey []byte) uint64 {
	defer k.Meter(ctx).FuncTiming(&ctx, "AutoIncrementID")()

	store := ctx.KVStore(k.storeKey)
	bz := store.Get(idKey)

	var id uint64
	if bz != nil {
		id = binary.BigEndian.Uint64(bz) + 1
	}

	bz = sdk.Uint64ToBigEndian(id)
	store.Set(idKey, bz)

	return id
}

func (k *Keeper) GetLastOutgoingBatchID(ctx sdk.Context) uint64 {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetLastOutgoingBatchID")()

	store := ctx.KVStore(k.storeKey)
	key := types.KeyLastOutgoingBatchID
	var id uint64
	bz := store.Get(key)
	if bz != nil {
		id = binary.BigEndian.Uint64(bz)
	}
	return id
}

func (k *Keeper) SetLastOutgoingBatchID(ctx sdk.Context, lastOutgoingBatchID uint64) {
	defer k.Meter(ctx).FuncTiming(&ctx, "SetLastOutgoingBatchID")()

	store := ctx.KVStore(k.storeKey)
	key := types.KeyLastOutgoingBatchID
	bz := sdk.Uint64ToBigEndian(lastOutgoingBatchID)
	store.Set(key, bz)
}

func (k *Keeper) GetLastOutgoingPoolID(ctx sdk.Context) uint64 {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetLastOutgoingPoolID")()

	store := ctx.KVStore(k.storeKey)
	key := types.KeyLastTXPoolID
	var id uint64
	bz := store.Get(key)
	if bz != nil {
		id = binary.BigEndian.Uint64(bz)
	}
	return id
}

func (k *Keeper) SetLastOutgoingPoolID(ctx sdk.Context, lastOutgoingPoolID uint64) {
	defer k.Meter(ctx).FuncTiming(&ctx, "SetLastOutgoingPoolID")()

	store := ctx.KVStore(k.storeKey)
	key := types.KeyLastTXPoolID
	bz := sdk.Uint64ToBigEndian(lastOutgoingPoolID)
	store.Set(key, bz)
}

func (k *Keeper) SetOutgoingTxFee(ctx sdk.Context, token common.Address, fee *types.ERC20Token, txID uint64) {
	defer k.Meter(ctx).FuncTiming(&ctx, "SetOutgoingTxFee")()

	store := ctx.KVStore(k.storeKey)
	store.Set(types.GetFeeIndexKey(token, fee, txID), []byte{1})
}

func (k *Keeper) DeleteOutgoingTxFee(ctx sdk.Context, token common.Address, fee *types.ERC20Token, txID uint64) {
	defer k.Meter(ctx).FuncTiming(&ctx, "DeleteOutgoingTxFee")()

	store := ctx.KVStore(k.storeKey)
	store.Delete(types.GetFeeIndexKey(token, fee, txID))
}

func (k *Keeper) IterateTokenTxsByFee(ctx sdk.Context, token common.Address, cb func(fee math.Int, txID uint64) bool) {
	defer k.Meter(ctx).FuncTiming(&ctx, "IterateTokenTxsByFee")()

	feeStore := prefix.NewStore(k.getStore(ctx), types.FeeIndexOutgoingTxKey)
	chaintypes.IterateKeysSafe(feeStore.ReverseIterator(PrefixRange(token.Bytes())), func(key []byte) (stop bool) {
		feeAmount := math.NewIntFromBigInt(big.NewInt(0).SetBytes(key[20:52]))
		txID := binary.BigEndian.Uint64(key[52:])

		return cb(feeAmount, txID)
	})
}
