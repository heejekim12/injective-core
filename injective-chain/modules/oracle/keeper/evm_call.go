package keeper

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"

	evmtypes "github.com/InjectiveLabs/injective-core/injective-chain/modules/evm/types"
)

// CallEVMWithValue performs a read-only EVM call with a simulated msg.value.
// from is the EVM sender used for balance checks.
// Errors returned are plain; callers are responsible for wrapping them in the
// appropriate oracle-specific sentinel.
func (k *Keeper) CallEVMWithValue(ctx sdk.Context, from, contractAddr common.Address, data []byte, gasCap uint64, value *big.Int) (ret []byte, err error) {
	defer k.Meter(ctx).FuncTiming(&ctx, "CallEVMWithValue")(&err)

	hexValue := hexutil.Big(*value)
	return k.executeEVMCall(ctx, &from, contractAddr, data, gasCap, &hexValue)
}

// CallEVM performs a read-only EVM call without a sender or value.
// Errors returned are plain; callers are responsible for wrapping them in the
// appropriate oracle-specific sentinel.
func (k *Keeper) CallEVM(ctx sdk.Context, contractAddr common.Address, data []byte, gasCap uint64) (ret []byte, err error) {
	defer k.Meter(ctx).FuncTiming(&ctx, "CallEVM")(&err)

	return k.executeEVMCall(ctx, nil, contractAddr, data, gasCap, nil)
}

func (k *Keeper) executeEVMCall(ctx sdk.Context, from *common.Address, contractAddr common.Address, data []byte, gasCap uint64, value *hexutil.Big) (ret []byte, err error) {
	defer k.Meter(ctx).FuncTiming(&ctx, "executeEVMCall")(&err)

	if k.evmKeeper == nil {
		return nil, errors.New("EVM keeper not available")
	}

	remaining := ctx.GasMeter().GasRemaining()
	if gasCap > remaining {
		gasCap = remaining
	}
	if gasCap == 0 {
		return nil, errors.New("insufficient gas for oracle EVM verification")
	}

	callData := hexutil.Bytes(data)
	txArgs, err := json.Marshal(evmtypes.TransactionArgs{
		From:  from,
		To:    &contractAddr,
		Input: &callData,
		Value: value,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal EVM transaction args: %w", err)
	}
	resp, err := k.evmKeeper.EthCall(ctx, &evmtypes.EthCallRequest{
		Args:   txArgs,
		GasCap: gasCap,
	})
	if err != nil {
		ctx.GasMeter().ConsumeGas(gasCap, "oracle_evm_verification_error")
		return nil, fmt.Errorf("EVM call failed: %w", err)
	}

	// Charge the actual EVM gas against the Cosmos gas meter before inspecting
	// the VM result so that verifier work is always paid for — even on revert.
	// This is critical to prevent price update messages to be an attack vector.
	if resp.GasUsed > 0 {
		ctx.GasMeter().ConsumeGas(resp.GasUsed, "oracle_evm_verification")
	}

	if resp.VmError != "" {
		return nil, fmt.Errorf("EVM call reverted: %s", resp.VmError)
	}
	if len(resp.Ret) == 0 {
		return nil, errors.New("EVM call returned empty response")
	}
	return resp.Ret, nil
}
