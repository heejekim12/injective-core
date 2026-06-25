package backend

import (
	"bytes"
	"fmt"
	"math/big"
	"strconv"

	rpctypes "github.com/InjectiveLabs/injective-core/injective-chain/modules/evm/rpc/types"
	evmtypes "github.com/InjectiveLabs/injective-core/injective-chain/modules/evm/types"
	abci "github.com/cometbft/cometbft/abci/types"
	cmrpcclient "github.com/cometbft/cometbft/rpc/client"
	cmrpctypes "github.com/cometbft/cometbft/rpc/core/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	grpctypes "github.com/cosmos/cosmos-sdk/types/grpc"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// BlockNumber returns the current block number in abci app state. Because abci
// app state could lag behind from tendermint latest block, it's more stable for
// the client to use the latest block number in abci app state than tendermint
// rpc.
func (b *Backend) BlockNumber() (hexutil.Uint64, error) {
	// do any grpc query, ignore the response and use the returned block height
	var header metadata.MD
	_, err := b.queryClient.Params(b.ctx, &evmtypes.QueryParamsRequest{}, grpc.Header(&header))
	if err != nil {
		return hexutil.Uint64(0), err
	}

	blockHeightHeader := header.Get(grpctypes.GRPCBlockHeightHeader)
	if headerLen := len(blockHeightHeader); headerLen != 1 {
		return 0, fmt.Errorf("unexpected '%s' gRPC header length; got %d, expected: %d", grpctypes.GRPCBlockHeightHeader, headerLen, 1)
	}

	height, err := strconv.ParseUint(blockHeightHeader[0], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse block height: %w", err)
	}

	return hexutil.Uint64(height), nil
}

// GetBlockByNumber returns the JSON-RPC compatible Ethereum block identified by
// block number. Depending on fullTx it either returns the full transaction
// objects or if false only the hashes of the transactions.
func (b *Backend) GetBlockByNumber(blockNum rpctypes.BlockNumber, fullTx bool) (map[string]interface{}, error) {
	resBlock, err := b.TendermintBlockByNumber(blockNum)
	if err != nil {
		return nil, nil
	}

	// return if requested block height is greater than the current one
	if resBlock == nil || resBlock.Block == nil {
		return nil, nil
	}

	blockRes, err := b.TendermintBlockResultByNumber(&resBlock.Block.Height)
	if err != nil {
		b.logger.Debug("failed to fetch block result from Tendermint", "height", blockNum, "error", err.Error())
		return nil, nil
	}

	res, err := b.RPCBlockFromTendermintBlock(resBlock, blockRes, fullTx)
	if err != nil {
		b.logger.Debug("GetEthBlockFromTendermint failed", "height", blockNum, "error", err.Error())
		return nil, err
	}

	return res, nil
}

// GetBlockByHash returns the JSON-RPC compatible Ethereum block identified by
// hash.
func (b *Backend) GetBlockByHash(hash common.Hash, fullTx bool) (map[string]interface{}, error) {
	resBlock, err := b.TendermintBlockByHash(hash)
	if err != nil {
		return nil, err
	}

	if resBlock == nil {
		// block not found
		return nil, nil
	}

	blockRes, err := b.TendermintBlockResultByNumber(&resBlock.Block.Height)
	if err != nil {
		b.logger.Debug("failed to fetch block result from Tendermint", "block-hash", hash.String(), "error", err.Error())
		return nil, nil
	}

	res, err := b.RPCBlockFromTendermintBlock(resBlock, blockRes, fullTx)
	if err != nil {
		b.logger.Debug("GetEthBlockFromTendermint failed", "hash", hash, "error", err.Error())
		return nil, err
	}

	return res, nil
}

// GetBlockReceipts returns all ethereum transaction receipts for the provided block.
//
//nolint:revive // legacy receipt assembly path; intentionally preserved behavior with guarded fallbacks.
func (b *Backend) GetBlockReceipts(blockNrOrHash rpctypes.BlockNumberOrHash) ([]map[string]any, error) {
	var (
		resBlock *cmrpctypes.ResultBlock
		err      error
	)

	switch {
	case blockNrOrHash.BlockHash != nil && blockNrOrHash.BlockNumber != nil:
		return nil, errors.New("types BlockHash and BlockNumber cannot be both set")
	case blockNrOrHash.BlockHash != nil:
		resBlock, err = b.TendermintBlockByHash(*blockNrOrHash.BlockHash)
	case blockNrOrHash.BlockNumber != nil:
		resBlock, err = b.TendermintBlockByNumber(*blockNrOrHash.BlockNumber)
	default:
		return nil, errors.New("types BlockHash and BlockNumber cannot be both nil")
	}

	if err != nil {
		return nil, err
	}
	if resBlock == nil || resBlock.Block == nil {
		return nil, nil
	}

	blockRes, err := b.TendermintBlockResultByNumber(&resBlock.Block.Height)
	if err != nil {
		b.logger.Warn("failed to retrieve block results", "height", resBlock.Block.Height, "error", err.Error())
		return nil, nil
	}
	if blockRes == nil {
		return nil, nil
	}

	receipts := make([]map[string]any, 0)
	signer := ethtypes.LatestSignerForChainID(b.ChainID().ToInt())
	blockHash := common.BytesToHash(resBlock.Block.Header.Hash()).Hex()
	blockNumber := hexutil.Uint64(resBlock.Block.Height)

	var (
		ethTxIndex             int32
		cumulativeBlockGasUsed uint64
		baseFee                *big.Int
		baseFeeLoaded          bool
	)

	for txIndex, txBz := range resBlock.Block.Txs {
		if txIndex >= len(blockRes.TxResults) {
			b.logger.Warn("block results shorter than tx list", "height", resBlock.Block.Height, "txIndex", txIndex)
			break
		}

		txResult := blockRes.TxResults[txIndex]
		if txResult == nil {
			b.logger.Warn("missing tx result entry", "height", resBlock.Block.Height, "txIndex", txIndex)
			continue
		}

		tx, err := b.clientCtx.TxConfig.TxDecoder()(txBz)
		if err != nil {
			b.logger.Warn("failed to decode tx in block", "height", resBlock.Block.Height, "txIndex", txIndex, "error", err.Error())
			cumulativeBlockGasUsed += uint64(txResult.GasUsed)
			continue
		}

		parsedTxs, parsedErr := rpctypes.ParseTxResult(txResult, tx)
		if parsedErr != nil && (txResult.Code == abci.CodeTypeOK || txResult.Codespace == evmtypes.ModuleName) {
			b.logger.Warn("failed to parse tx events", "height", resBlock.Block.Height, "txIndex", txIndex, "error", parsedErr.Error())
		}

		var cumulativeTxEthGasUsed uint64
		for msgIndex, msg := range tx.GetMsgs() {
			ethMsg, ok := msg.(*evmtypes.MsgEthereumTx)
			if !ok {
				continue
			}

			txData := ethMsg.AsTransaction()
			if txData == nil {
				return nil, errors.New("failed to unpack tx data")
			}

			txHash := ethMsg.Hash()
			txFailed := false
			txGasUsed := ethMsg.GetGas()

			switch {
			case txResult.Code != abci.CodeTypeOK && txResult.Codespace != evmtypes.ModuleName:
				// Exceeds block gas limit scenario.
				txFailed = true
			case parsedTxs == nil:
				// If tx parsing fails, fall back to the tx result status.
				txFailed = txResult.Code != abci.CodeTypeOK
			default:
				parsedTx := parsedTxs.GetTxByMsgIndex(msgIndex)
				if parsedTx == nil {
					b.logger.Warn("msg index not found in parsed tx result", "height", resBlock.Block.Height, "txIndex", txIndex, "msgIndex", msgIndex)
					txFailed = txResult.Code != abci.CodeTypeOK
				} else {
					if parsedTx.EthTxIndex >= 0 && parsedTx.EthTxIndex != ethTxIndex {
						b.logger.Error("eth tx index don't match", "expect", ethTxIndex, "found", parsedTx.EthTxIndex)
					}
					txHash = parsedTx.Hash
					txGasUsed = parsedTx.GasUsed
					txFailed = parsedTx.Failed
				}
			}

			cumulativeTxEthGasUsed += txGasUsed

			var status hexutil.Uint
			if txFailed {
				status = hexutil.Uint(ethtypes.ReceiptStatusFailed)
			} else {
				status = hexutil.Uint(ethtypes.ReceiptStatusSuccessful)
			}

			from, err := ethMsg.GetSenderLegacy(signer)
			if err != nil {
				return nil, err
			}

			logs, err := evmtypes.DecodeMsgLogs(
				txResult.Data,
				msgIndex,
				uint64(blockRes.Height),
			)
			if err != nil {
				b.logger.Warn("failed to parse logs", "hash", txHash, "error", err.Error())
			}

			if txData.Type() == ethtypes.DynamicFeeTxType && !baseFeeLoaded {
				baseFee, err = b.BaseFee(blockRes)
				if err != nil {
					baseFee = nil
				}
				baseFeeLoaded = true
			}

			receipt := map[string]any{
				// Consensus fields: These fields are defined by the Yellow Paper
				"status":            status,
				"cumulativeGasUsed": hexutil.Uint64(cumulativeBlockGasUsed + cumulativeTxEthGasUsed),
				"logsBloom":         ethtypes.BytesToBloom(evmtypes.LogsBloom(logs)),
				"logs":              logs,

				// Implementation fields: These fields are added by geth when processing a transaction.
				"transactionHash": txHash,
				"contractAddress": nil,
				"gasUsed":         hexutil.Uint64(txGasUsed),

				// Inclusion information
				"blockHash":        blockHash,
				"blockNumber":      blockNumber,
				"transactionIndex": hexutil.Uint64(ethTxIndex),

				// https://github.com/foundry-rs/foundry/issues/7640
				"effectiveGasPrice": (*hexutil.Big)(effectiveGasPrice(txData, baseFee)),

				// sender and receiver (contract or EOA) addresses
				"from": from,
				"to":   txData.To(),
				"type": hexutil.Uint(txData.Type()),
			}

			if logs == nil {
				receipt["logs"] = [][]*ethtypes.Log{}
			}

			// If the ContractAddress is 20 0x0 bytes, assume it is not a contract creation.
			if txData.To() == nil {
				receipt["contractAddress"] = crypto.CreateAddress(from, txData.Nonce())
			}

			receipts = append(receipts, receipt)
			ethTxIndex++
		}

		cumulativeBlockGasUsed += uint64(txResult.GasUsed)
	}

	return receipts, nil
}

// GetBlockTransactionCountByHash returns the number of Ethereum transactions in
// the block identified by hash.
func (b *Backend) GetBlockTransactionCountByHash(hash common.Hash) *hexutil.Uint {
	sc, ok := b.clientCtx.Client.(cmrpcclient.SignClient)
	if !ok {
		b.logger.Error("invalid rpc client")
	}
	block, err := sc.BlockByHash(b.ctx, hash.Bytes())
	if err != nil {
		b.logger.Debug("block not found", "hash", hash.Hex(), "error", err.Error())
		return nil
	} else if block == nil {
		b.logger.Debug("block not found", "hash", hash.Hex())
		return nil
	} else if block.Block == nil {
		b.logger.Debug("block not found", "hash", hash.Hex())
		return nil
	}

	return b.GetBlockTransactionCount(block)
}

// GetBlockTransactionCountByNumber returns the number of Ethereum transactions
// in the block identified by number.
func (b *Backend) GetBlockTransactionCountByNumber(blockNum rpctypes.BlockNumber) *hexutil.Uint {
	block, err := b.TendermintBlockByNumber(blockNum)
	if err != nil {
		b.logger.Debug("block not found", "height", blockNum.Int64(), "error", err.Error())
		return nil
	} else if block == nil {
		b.logger.Debug("block not found", "height", blockNum.Int64())
		return nil
	} else if block.Block == nil {
		b.logger.Debug("block not found", "height", blockNum.Int64())
		return nil
	}

	return b.GetBlockTransactionCount(block)
}

// GetBlockTransactionCount returns the number of Ethereum transactions in a
// given block.
func (b *Backend) GetBlockTransactionCount(block *cmrpctypes.ResultBlock) *hexutil.Uint {
	ethMsgs := b.EthMsgsFromTendermintBlock(block)
	n := hexutil.Uint(len(ethMsgs))
	return &n
}

// TendermintBlockByNumber returns a Tendermint-formatted block for a given
// block number
func (b *Backend) TendermintBlockByNumber(blockNum rpctypes.BlockNumber) (*cmrpctypes.ResultBlock, error) {
	height := blockNum.Int64()
	if height <= 0 {
		// fetch the latest block number from the app state, more accurate than the tendermint block store state.
		n, err := b.BlockNumber()
		if err != nil {
			return nil, err
		}
		height = int64(n)
	}
	resBlock, err := b.clientCtx.Client.Block(b.ctx, &height)
	if err != nil {
		b.logger.Debug("tendermint client failed to get block", "height", height, "error", err.Error())
		return nil, err
	}

	if resBlock.Block == nil {
		b.logger.Debug("TendermintBlockByNumber block not found", "height", height)
		return nil, nil
	}

	return resBlock, nil
}

// TendermintBlockResultByNumber returns a Tendermint-formatted block result
// by block number
func (b *Backend) TendermintBlockResultByNumber(height *int64) (*cmrpctypes.ResultBlockResults, error) {
	sc, ok := b.clientCtx.Client.(cmrpcclient.SignClient)
	if !ok {
		return nil, errors.New("invalid rpc client")
	}
	return sc.BlockResults(b.ctx, height)
}

// TendermintBlockByHash returns a Tendermint-formatted block by block number
func (b *Backend) TendermintBlockByHash(blockHash common.Hash) (*cmrpctypes.ResultBlock, error) {
	sc, ok := b.clientCtx.Client.(cmrpcclient.SignClient)
	if !ok {
		return nil, errors.New("invalid rpc client")
	}
	resBlock, err := sc.BlockByHash(b.ctx, blockHash.Bytes())
	if err != nil {
		b.logger.Debug("tendermint client failed to get block", "blockHash", blockHash.Hex(), "error", err.Error())
		return nil, err
	}

	if resBlock == nil || resBlock.Block == nil {
		b.logger.Debug("TendermintBlockByHash block not found", "blockHash", blockHash.Hex())
		return nil, nil
	}

	return resBlock, nil
}

// BlockNumberFromTendermint returns the BlockNumber from BlockNumberOrHash
func (b *Backend) BlockNumberFromTendermint(blockNrOrHash rpctypes.BlockNumberOrHash) (rpctypes.BlockNumber, error) {
	switch {
	case blockNrOrHash.BlockHash == nil && blockNrOrHash.BlockNumber == nil:
		return rpctypes.EthEarliestBlockNumber, fmt.Errorf("types BlockHash and BlockNumber cannot be both nil")
	case blockNrOrHash.BlockHash != nil:
		blockNumber, err := b.BlockNumberFromTendermintByHash(*blockNrOrHash.BlockHash)
		if err != nil {
			return rpctypes.EthEarliestBlockNumber, err
		}
		return rpctypes.NewBlockNumber(blockNumber), nil
	case blockNrOrHash.BlockNumber != nil:
		return *blockNrOrHash.BlockNumber, nil
	default:
		return rpctypes.EthEarliestBlockNumber, nil
	}
}

// BlockNumberFromTendermintByHash returns the block height of given block hash
func (b *Backend) BlockNumberFromTendermintByHash(blockHash common.Hash) (*big.Int, error) {
	resBlock, err := b.TendermintBlockByHash(blockHash)
	if err != nil {
		return nil, err
	}
	if resBlock == nil {
		return nil, errors.Errorf("block not found for hash %s", blockHash.Hex())
	}
	return big.NewInt(resBlock.Block.Height), nil
}

// EthMsgsFromTendermintBlock returns all real MsgEthereumTxs from a Tendermint block.
func (b *Backend) EthMsgsFromTendermintBlock(
	resBlock *cmrpctypes.ResultBlock,
) []*evmtypes.MsgEthereumTx {
	var result []*evmtypes.MsgEthereumTx
	block := resBlock.Block

	for _, tx := range block.Txs {
		decodedTx, err := b.clientCtx.TxConfig.TxDecoder()(tx)
		if err != nil {
			b.logger.Warn("failed to decode transaction in block", "height", block.Height, "error", err.Error())
			continue
		}

		for _, msg := range decodedTx.GetMsgs() {
			ethMsg, ok := msg.(*evmtypes.MsgEthereumTx)
			if !ok {
				continue
			}

			result = append(result, ethMsg)
		}
	}

	return result
}

// HeaderByNumber returns the block header identified by height.
func (b *Backend) HeaderByNumber(blockNum rpctypes.BlockNumber) (*ethtypes.Header, error) {
	resBlock, err := b.TendermintBlockByNumber(blockNum)
	if err != nil {
		return nil, err
	}

	if resBlock == nil {
		return nil, errors.Errorf("block not found for height %d", blockNum)
	}

	blockRes, err := b.TendermintBlockResultByNumber(&resBlock.Block.Height)
	if err != nil {
		return nil, fmt.Errorf("block result not found for height %d", resBlock.Block.Height)
	}

	bloom, err := b.BlockBloom(blockRes)
	if err != nil {
		b.logger.Debug("HeaderByNumber BlockBloom failed", "height", resBlock.Block.Height)
	}

	baseFee, err := b.BaseFee(blockRes)
	if err != nil {
		// handle the error for pruned node.
		b.logger.Error("failed to fetch Base Fee from pruned block. Check node pruning configuration", "height", resBlock.Block.Height, "error", err)
	}

	ethHeader := rpctypes.EthHeaderFromTendermint(resBlock.Block.Header, bloom, baseFee)
	return ethHeader, nil
}

// HeaderByHash returns the block header identified by hash.
func (b *Backend) HeaderByHash(blockHash common.Hash) (*ethtypes.Header, error) {
	resBlock, err := b.TendermintBlockByHash(blockHash)
	if err != nil {
		return nil, err
	}
	if resBlock == nil {
		return nil, errors.Errorf("block not found for hash %s", blockHash.Hex())
	}

	blockRes, err := b.TendermintBlockResultByNumber(&resBlock.Block.Height)
	if err != nil {
		return nil, errors.Errorf("block result not found for height %d", resBlock.Block.Height)
	}

	bloom, err := b.BlockBloom(blockRes)
	if err != nil {
		b.logger.Debug("HeaderByHash BlockBloom failed", "height", resBlock.Block.Height)
	}

	baseFee, err := b.BaseFee(blockRes)
	if err != nil {
		// handle the error for pruned node.
		b.logger.Error("failed to fetch Base Fee from prunned block. Check node prunning configuration", "height", resBlock.Block.Height, "error", err)
	}

	ethHeader := rpctypes.EthHeaderFromTendermint(resBlock.Block.Header, bloom, baseFee)
	return ethHeader, nil
}

// BlockBloom query block bloom filter from block results
func (b *Backend) BlockBloom(blockRes *cmrpctypes.ResultBlockResults) (ethtypes.Bloom, error) {
	for _, event := range blockRes.FinalizeBlockEvents {
		if event.Type != evmtypes.EventTypeBlockBloom {
			continue
		}

		for _, attr := range event.Attributes {
			if bytes.Equal([]byte(attr.Key), bAttributeKeyEthereumBloom) {
				return ethtypes.BytesToBloom([]byte(attr.Value)), nil
			}
		}
	}

	b.logger.Debug("BlockBloom event not found", "blockRes", blockRes)
	return ethtypes.Bloom{}, errors.New("block bloom event is not found")
}

// RPCBlockFromTendermintBlock returns a JSON-RPC compatible Ethereum block from a
// given Tendermint block and its block result.
func (b *Backend) RPCBlockFromTendermintBlock(
	resBlock *cmrpctypes.ResultBlock,
	blockRes *cmrpctypes.ResultBlockResults,
	fullTx bool,
) (map[string]interface{}, error) {
	ethRPCTxs := []interface{}{}
	block := resBlock.Block

	baseFee, err := b.BaseFee(blockRes)
	if err != nil {
		// handle the error for pruned node.
		b.logger.Error("failed to fetch Base Fee from prunned block. Check node prunning configuration", "height", block.Height, "error", err)
	}

	msgs := b.EthMsgsFromTendermintBlock(resBlock)
	for txIndex, ethMsg := range msgs {
		if !fullTx {
			ethRPCTxs = append(ethRPCTxs, ethMsg.Hash())
			continue
		}

		rpcTx, err := rpctypes.NewRPCTransaction(
			ethMsg,
			common.BytesToHash(block.Hash()),
			uint64(block.Height),
			uint64(txIndex),
			baseFee,
			b.ChainID().ToInt(),
		)
		if err != nil {
			b.logger.Debug("NewTransactionFromData for receipt failed", "hash", ethMsg.Hash, "error", err.Error())
			continue
		}
		ethRPCTxs = append(ethRPCTxs, rpcTx)
	}

	bloom, err := b.BlockBloom(blockRes)
	if err != nil {
		b.logger.Debug("failed to query BlockBloom", "height", block.Height, "error", err.Error())
	}

	req := &evmtypes.QueryValidatorAccountRequest{
		ConsAddress: sdk.ConsAddress(block.Header.ProposerAddress).String(),
	}

	var validatorAccAddr sdk.AccAddress

	ctx := rpctypes.ContextWithHeight(block.Height)
	res, err := b.queryClient.ValidatorAccount(ctx, req)
	if err != nil {
		b.logger.Debug(
			"failed to query validator operator address",
			"height", block.Height,
			"cons-address", req.ConsAddress,
			"error", err.Error(),
		)
		// use zero address as the validator operator address
		validatorAccAddr = sdk.AccAddress(common.Address{}.Bytes())
	} else {
		validatorAccAddr, err = sdk.AccAddressFromBech32(res.AccountAddress)
		if err != nil {
			return nil, err
		}
	}

	validatorAddr := common.BytesToAddress(validatorAccAddr)

	gasLimit, err := rpctypes.BlockMaxGasFromConsensusParams(ctx, b.clientCtx, block.Height)
	if err != nil {
		b.logger.Error("failed to query consensus params", "error", err.Error())
	}

	var gasUsed uint64
	for _, txsResult := range blockRes.TxResults {
		gasUsed += uint64(txsResult.GetGasUsed())
	}

	formattedBlock := rpctypes.FormatBlock(
		block.Header, block.Size(),
		gasLimit, new(big.Int).SetUint64(gasUsed),
		ethRPCTxs, bloom, validatorAddr, baseFee,
	)
	return formattedBlock, nil
}

// EthBlockByNumber returns the Ethereum Block identified by number.
func (b *Backend) EthBlockByNumber(blockNum rpctypes.BlockNumber) (*ethtypes.Block, error) {
	resBlock, err := b.TendermintBlockByNumber(blockNum)
	if err != nil {
		return nil, err
	}
	if resBlock == nil {
		// block not found
		return nil, fmt.Errorf("block not found for height %d", blockNum)
	}

	blockRes, err := b.TendermintBlockResultByNumber(&resBlock.Block.Height)
	if err != nil {
		return nil, fmt.Errorf("block result not found for height %d", resBlock.Block.Height)
	}

	return b.EthBlockFromTendermintBlock(resBlock, blockRes)
}

// EthBlockFromTendermintBlock returns an Ethereum Block type from Tendermint block
// EthBlockFromTendermintBlock
func (b *Backend) EthBlockFromTendermintBlock(
	resBlock *cmrpctypes.ResultBlock,
	blockRes *cmrpctypes.ResultBlockResults,
) (*ethtypes.Block, error) {
	block := resBlock.Block
	height := block.Height
	bloom, err := b.BlockBloom(blockRes)
	if err != nil {
		b.logger.Debug("HeaderByNumber BlockBloom failed", "height", height)
	}

	baseFee, err := b.BaseFee(blockRes)
	if err != nil {
		// handle error for pruned node and log
		b.logger.Error("failed to fetch Base Fee from prunned block. Check node prunning configuration", "height", height, "error", err)
	}

	ethHeader := rpctypes.EthHeaderFromTendermint(block.Header, bloom, baseFee)
	msgs := b.EthMsgsFromTendermintBlock(resBlock)

	txs := make([]*ethtypes.Transaction, len(msgs))
	for i, ethMsg := range msgs {
		txs[i] = ethMsg.AsTransaction()
	}

	// TODO: add tx receipts
	// TODO(max): check if this still needed
	ethBlock := ethtypes.NewBlockWithHeader(ethHeader).WithBody(ethtypes.Body{Transactions: txs})

	return ethBlock, nil
}
