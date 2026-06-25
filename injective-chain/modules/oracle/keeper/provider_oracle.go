package keeper

import (
	"sort"

	"cosmossdk.io/math"
	"cosmossdk.io/store/prefix"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/InjectiveLabs/injective-core/injective-chain/modules/oracle/assistant/provider"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/oracle/types"
	chaintypes "github.com/InjectiveLabs/injective-core/injective-chain/types"
)

// IsProviderRelayer checks that the relayer has been authorized for the given provider.
func (k *Keeper) IsProviderRelayer(ctx sdk.Context, provider string, relayer sdk.AccAddress) bool {
	defer k.Meter(ctx).FuncTiming(&ctx, "IsProviderRelayer")()

	existingProvider, _ := k.getRelayerProvider(ctx, relayer)
	return existingProvider == provider
}

// GetProviderRelayers returns all relayers for a given provider.
func (k *Keeper) GetProviderRelayers(ctx sdk.Context, provider string) []sdk.AccAddress {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetProviderRelayers")()

	info := k.GetProviderInfo(ctx, provider)
	if info == nil {
		return nil
	}
	relayers := make([]sdk.AccAddress, 0, len(info.Relayers))
	for _, relayerStr := range info.Relayers {
		relayer, _ := sdk.AccAddressFromBech32(relayerStr)
		relayers = append(relayers, relayer)
	}
	return relayers
}

// DeleteProviderRelayers TODO: for consistency relayers should be of type []sdk.AccAddress
func (k *Keeper) DeleteProviderRelayers(ctx sdk.Context, provider string, relayers []string) error {
	defer k.Meter(ctx).FuncTiming(&ctx, "DeleteProviderRelayers")()

	currentRelayers := k.GetProviderRelayers(ctx, provider)
	if currentRelayers == nil {
		return types.ErrInvalidProvider
	}
	relayersToKeep := make(map[string]sdk.AccAddress)
	for _, r := range currentRelayers {
		relayersToKeep[r.String()] = r
	}

	for _, r := range relayers {
		delete(relayersToKeep, r)
		relayer, _ := sdk.AccAddressFromBech32(r)
		k.deleteProviderIndex(ctx, relayer)
	}

	remainingRelayers := make([]string, 0, len(relayers))
	for _, v := range relayersToKeep {
		remainingRelayers = append(remainingRelayers, v.String())
	}

	sort.SliceStable(remainingRelayers, func(i, j int) bool {
		return remainingRelayers[i] < remainingRelayers[j]
	})

	if err := k.SetProviderInfo(ctx, &types.ProviderInfo{
		Provider: provider,
		Relayers: remainingRelayers,
	}); err != nil {
		return err
	}
	return nil
}

func (k *Keeper) GetProviderInfo(ctx sdk.Context, provider string) *types.ProviderInfo {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetProviderInfo")()

	store := k.getStore(ctx)

	bz := store.Get(types.GetProviderInfoKey(provider))
	if bz == nil {
		return nil
	}
	var info types.ProviderInfo
	k.cdc.MustUnmarshal(bz, &info)
	return &info
}

func (k *Keeper) SetProviderInfo(ctx sdk.Context, providerInfo *types.ProviderInfo) error {
	defer k.Meter(ctx).FuncTiming(&ctx, "SetProviderInfo")()

	if err := types.ValidateReservedProviderID(providerInfo.Provider); err != nil {
		return err
	}

	bz := k.cdc.MustMarshal(providerInfo)

	k.getStore(ctx).Set(types.GetProviderInfoKey(providerInfo.Provider), bz)

	// set the index
	for _, relayerStr := range providerInfo.Relayers {
		relayer, _ := sdk.AccAddressFromBech32(relayerStr)
		// Enforce that the relayer does not already exist (e.g. for a different provider)
		existingProvider, found := k.getRelayerProvider(ctx, relayer)
		if found && existingProvider != providerInfo.Provider {
			return types.ErrRelayerAlreadyExists
		}
		k.setProviderIndex(ctx, providerInfo.Provider, relayer)
	}
	return nil
}

func (k *Keeper) GetAllProviderInfos(ctx sdk.Context) []*types.ProviderInfo {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetAllProviderInfos")()

	providerStore := prefix.NewStore(k.getStore(ctx), types.ProviderInfoPrefix)

	var providerInfos []*types.ProviderInfo
	chaintypes.IterateSafe(providerStore.Iterator(nil, nil), func(_, v []byte) bool {
		var p types.ProviderInfo
		k.cdc.MustUnmarshal(v, &p)
		providerInfos = append(providerInfos, &p)
		return false
	})

	return providerInfos
}

func (k *Keeper) setProviderIndex(ctx sdk.Context, provider string, relayer sdk.AccAddress) {
	defer k.Meter(ctx).FuncTiming(&ctx, "setProviderIndex")()

	relayerKey := types.GetProviderIndexKey(relayer)
	k.getStore(ctx).Set(relayerKey, []byte(provider))
}

func (k *Keeper) deleteProviderIndex(ctx sdk.Context, relayer sdk.AccAddress) {
	defer k.Meter(ctx).FuncTiming(&ctx, "deleteProviderIndex")()

	relayerKey := types.GetProviderIndexKey(relayer)
	k.getStore(ctx).Delete(relayerKey)
}

func (k *Keeper) getRelayerProvider(ctx sdk.Context, relayer sdk.AccAddress) (provider string, found bool) {
	defer k.Meter(ctx).FuncTiming(&ctx, "getRelayerProvider")()

	relayerKey := types.GetProviderIndexKey(relayer)
	bz := k.getStore(ctx).Get(relayerKey)
	if bz == nil {
		return "", false
	}
	return string(bz), true

}

func (k *Keeper) GetProviderPriceState(ctx sdk.Context, provider, symbol string) *types.ProviderPriceState {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetProviderPriceState")()

	key := types.GetProviderPriceKey(provider, symbol)
	bz := k.getStore(ctx).Get(key)

	if bz == nil {
		return nil
	}

	return k.unmarshalProviderPriceState(symbol, bz)
}

func (k *Keeper) unmarshalProviderPriceState(symbol string, bz []byte) *types.ProviderPriceState {
	var state types.PriceState
	if err := k.cdc.Unmarshal(bz, &state); err == nil {
		return &types.ProviderPriceState{Symbol: symbol, State: &state}
	}

	var legacyState types.ProviderPriceState
	if err := k.cdc.Unmarshal(bz, &legacyState); err != nil || legacyState.State == nil {
		return nil
	}

	return &types.ProviderPriceState{Symbol: symbol, State: legacyState.State}
}

func (k *Keeper) SetProviderPriceState(ctx sdk.Context, provider string, providerPriceState *types.ProviderPriceState) {
	defer k.Meter(ctx).FuncTiming(&ctx, "SetProviderPriceState")()

	symbol := providerPriceState.Symbol
	priceKey := types.GetProviderPriceKey(provider, symbol)
	bz := k.cdc.MustMarshal(providerPriceState.State)
	k.getStore(ctx).Set(priceKey, bz)

	// a bit of a hack since provider only works for (provider, symbol) and not base/quote
	pair := types.JoinProviderCompoundKey(provider, symbol)
	k.AppendPriceRecord(ctx, types.OracleType_Provider, pair, &types.PriceRecord{
		Timestamp: providerPriceState.State.Timestamp,
		Price:     providerPriceState.State.Price,
	})
}

func (k *Keeper) GetProviderPriceStates(ctx sdk.Context, provider string) []*types.ProviderPriceState {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetProviderPriceStates")()

	priceStore := prefix.NewStore(k.getStore(ctx), types.GetProviderPricePrefix(provider))

	var providerPriceStates []*types.ProviderPriceState
	chaintypes.IterateSafe(priceStore.Iterator(nil, nil), func(key, val []byte) bool {
		if state := k.unmarshalProviderPriceState(string(key), val); state != nil {
			providerPriceStates = append(providerPriceStates, state)
		}
		return false
	})

	return providerPriceStates
}

// GetProviderPrice returns the price for a given symbol for a given provider
func (k *Keeper) GetProviderPrice(ctx sdk.Context, provider, symbol string) *math.LegacyDec {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetProviderPrice")()

	priceState := k.GetProviderPriceState(ctx, provider, symbol)
	if priceState == nil {
		return nil
	}

	return &priceState.State.Price
}

// GetCumulativeProviderPrice returns the cumulative price for a given symbol for a given provider
func (k *Keeper) GetCumulativeProviderPrice(ctx sdk.Context, provider, symbol string) *math.LegacyDec {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetCumulativeProviderPrice")()

	providerPriceState := k.GetProviderPriceState(ctx, provider, symbol)
	if providerPriceState == nil {
		return nil
	}
	return &providerPriceState.State.CumulativePrice
}

func (k *Keeper) GetAllProviderStates(ctx sdk.Context) []*types.ProviderState {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetAllProviderStates")()

	providerInfos := k.GetAllProviderInfos(ctx)
	providerStates := make([]*types.ProviderState, 0, len(providerInfos))
	for _, info := range providerInfos {
		providerStates = append(providerStates, &types.ProviderState{
			ProviderInfo:        info,
			ProviderPriceStates: k.GetProviderPriceStates(ctx, info.Provider),
		})
	}
	return providerStates
}

func (k *Keeper) ProcessProviderPrices(ctx sdk.Context, msg *types.MsgRelayProviderPrices) {
	defer k.Meter(ctx).FuncTiming(&ctx, "ProcessProviderPrices")()

	provider.NewAssistant(k).ProcessProviderPrices(ctx, msg)
}
