package keeper

import (
	"cosmossdk.io/store/prefix"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"

	"github.com/InjectiveLabs/injective-core/injective-chain/modules/oracle/assistant/pyth"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/oracle/types"
	chaintypes "github.com/InjectiveLabs/injective-core/injective-chain/types"
)

// EmitPythPricesUpdated emits a typed event for updated Pyth price states.
func (k *Keeper) EmitPythPricesUpdated(ctx sdk.Context, states []*types.PythPriceState) {
	if len(states) == 0 {
		return
	}
	defer k.Meter(ctx).FuncTiming(&ctx, "EmitPythPricesUpdated")()
	// nolint:errcheck //ignored on purpose
	ctx.EventManager().EmitTypedEvent(&types.EventSetPythPrices{Prices: states})
}

// ProcessPythPriceAttestations sets the pyth price state.
func (k *Keeper) ProcessPythPriceAttestations(ctx sdk.Context, priceAttestations []*types.PriceAttestation) {
	defer k.Meter(ctx).FuncTiming(&ctx, "ProcessPythPriceAttestations")()

	pyth.NewAssistant(k).ProcessPriceAttestations(ctx, priceAttestations)
}

// GetPythPriceState reads the stored pyth price state.
func (k *Keeper) GetPythPriceState(ctx sdk.Context, priceID common.Hash) *types.PythPriceState {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetPythPriceState")()

	var priceState types.PythPriceState
	bz := k.getStore(ctx).Get(types.GetPythPriceStoreKey(priceID))
	if bz == nil {
		return nil
	}

	k.cdc.MustUnmarshal(bz, &priceState)
	priceState.PriceId = priceID.Hex()
	return &priceState
}

// SetPythPriceState sets the pyth price state.
func (k *Keeper) SetPythPriceState(ctx sdk.Context, priceState *types.PythPriceState) {
	defer k.Meter(ctx).FuncTiming(&ctx, "SetPythPriceState")()

	priceID := common.HexToHash(priceState.PriceId)
	toStore := *priceState
	toStore.PriceId = ""
	bz := k.cdc.MustMarshal(&toStore)

	k.getStore(ctx).Set(types.GetPythPriceStoreKey(priceID), bz)

	k.AppendPriceRecord(ctx, types.OracleType_Pyth, priceID.Hex(), &types.PriceRecord{
		Timestamp: priceState.PriceState.Timestamp,
		Price:     priceState.PriceState.Price,
	})
}

// GetAllPythPriceStates fetches all Pyth price states in the store
func (k *Keeper) GetAllPythPriceStates(ctx sdk.Context) []*types.PythPriceState {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetAllPythPriceStates")()

	pythPriceStore := prefix.NewStore(ctx.KVStore(k.storeKey), types.PythPriceKey)

	priceStates := make([]*types.PythPriceState, 0)
	chaintypes.IterateSafe(pythPriceStore.Iterator(nil, nil), func(iterKey, iterVal []byte) bool {
		var priceState types.PythPriceState
		k.cdc.MustUnmarshal(iterVal, &priceState)
		priceState.PriceId = common.BytesToHash(iterKey).Hex()
		priceStates = append(priceStates, &priceState)
		return false
	})

	return priceStates
}
