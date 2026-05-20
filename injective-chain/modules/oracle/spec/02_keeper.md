---
sidebar_position: 2
title: Keepers
---

# Keepers

The oracle module exposes several keeper interfaces that other modules can use to read price data. Modules should use the least-permissive interface that provides the functionality they require.

## ViewKeeper

The `ViewKeeper` provides read access to prices and cumulative prices for any supported oracle type and pair.

```go
type ViewKeeper interface {
    // GetReferencePrice returns the reference price for a pair and oracle type (oracle assistants when applicable, with legacy Band/BandIBC resolution otherwise).
    GetReferencePrice(ctx sdk.Context, oracletype types.OracleType, base string, quote string) *math.LegacyDec
    // GetCumulativePrice returns the base and quote cumulative prices for TWAP calculation.
    // For USD quotes and PriceFeed oracles, quoteCumulative represents elapsed time (block time).
    GetCumulativePrice(ctx sdk.Context, oracleType types.OracleType, base string, quote string) (baseCumulative, quoteCumulative *math.LegacyDec)
    // GetProviderPrice returns the price for a given provider and symbol.
    GetProviderPrice(ctx sdk.Context, provider string, symbol string) *math.LegacyDec
    // GetCumulativeProviderPrice returns the cumulative price for a given provider and symbol.
    GetCumulativeProviderPrice(ctx sdk.Context, provider string, symbol string) *math.LegacyDec
}
```

Notes:

- `GetReferencePrice` for Coinbase oracles returns the 5-minute TWAP price.
- `GetCumulativePrice` returns two values: the base cumulative price and the quote cumulative price. For USD quotes or PriceFeed oracles, the quote cumulative equals the block timestamp, enabling a unified TWAP formula: `TWAP = (baseCum₂ - baseCum₁) / (quoteCum₂ - quoteCum₁)`.
- For `OracleType_Provider`, `GetProviderPrice` and `GetCumulativeProviderPrice` are the provider-specific accessors; `GetReferencePrice` also resolves provider prices via the provider oracle assistant when given the same base/quote layout as exchange markets.

## Band (Deprecated)

> **Deprecated.** Band oracle is no longer supported.

The `BandKeeper` provided the ability to create/modify/read/delete BandPricefeed and BandRelayer state.

```go
type BandKeeper interface {
    GetBandPriceState(ctx sdk.Context, symbol string) *types.BandPriceState
    GetAllBandPriceStates(ctx sdk.Context) []*types.BandPriceState
    GetBandReferencePrice(ctx sdk.Context, base string, quote string) *math.LegacyDec
    GetAllBandRelayers(ctx sdk.Context) []string
}
```

## Band IBC (Deprecated)

> **Deprecated.** Band IBC oracle is no longer supported.

The `BandIBCKeeper` provided the ability to create/modify/read/delete BandIBC oracle requests, price states, client IDs, and calldata records.

```go
type BandIBCKeeper interface {
    SetBandIBCOracleRequest(ctx sdk.Context, req types.BandOracleRequest)
    GetBandIBCOracleRequest(ctx sdk.Context, requestID uint64) *types.BandOracleRequest
    DeleteBandIBCOracleRequest(ctx sdk.Context, requestID uint64)
    GetAllBandIBCOracleRequests(ctx sdk.Context) []*types.BandOracleRequest

    GetBandIBCPriceState(ctx sdk.Context, symbol string) *types.BandPriceState
    SetBandIBCPriceState(ctx sdk.Context, symbol string, priceState *types.BandPriceState)
    GetAllBandIBCPriceStates(ctx sdk.Context) []*types.BandPriceState
    GetBandIBCReferencePrice(ctx sdk.Context, base string, quote string) *math.LegacyDec

    GetBandIBCLatestClientID(ctx sdk.Context) uint64
    SetBandIBCLatestClientID(ctx sdk.Context, clientID uint64)
    SetBandIBCCallDataRecord(ctx sdk.Context, record *types.CalldataRecord)
    GetBandIBCCallDataRecord(ctx sdk.Context, clientID uint64) *types.CalldataRecord
}
```

## Coinbase

The `CoinbaseKeeper` provides the ability to create, modify, and read Coinbase price state data.

```go
type CoinbaseKeeper interface {
    GetCoinbasePrice(ctx sdk.Context, base string, quote string) *math.LegacyDec
    HasCoinbasePriceState(ctx sdk.Context, key string) bool
    GetCoinbasePriceState(ctx sdk.Context, key string) *types.CoinbasePriceState
    SetCoinbasePriceState(ctx sdk.Context, priceData *types.CoinbasePriceState) error
    GetAllCoinbasePriceStates(ctx sdk.Context) []*types.CoinbasePriceState
}
```

`GetCoinbasePrice` returns the 5-minute TWAP price computed from stored historical `CoinbasePriceState` entries based on `Timestamp` values.

## PriceFeeder

The `PriceFeederKeeper` provides the ability to create/modify/read/delete PriceFeed price states and relayers.

```go
type PriceFeederKeeper interface {
    IsPriceFeedRelayer(ctx sdk.Context, oracleBase, oracleQuote string, relayer sdk.AccAddress) bool
    GetAllPriceFeedStates(ctx sdk.Context) []*types.PriceFeedState
    GetAllPriceFeedRelayers(ctx sdk.Context, baseQuoteHash common.Hash) []string
    SetPriceFeedRelayer(ctx sdk.Context, oracleBase, oracleQuote string, relayer sdk.AccAddress)
    SetPriceFeedRelayerFromBaseQuoteHash(ctx sdk.Context, baseQuoteHash common.Hash, relayer sdk.AccAddress)
    DeletePriceFeedRelayer(ctx sdk.Context, oracleBase, oracleQuote string, relayer sdk.AccAddress)
    HasPriceFeedInfo(ctx sdk.Context, priceFeedInfo *types.PriceFeedInfo) bool
    GetPriceFeedInfo(ctx sdk.Context, baseQuoteHash common.Hash) *types.PriceFeedInfo
    SetPriceFeedInfo(ctx sdk.Context, priceFeedInfo *types.PriceFeedInfo)
    GetPriceFeedPriceState(ctx sdk.Context, base string, quote string) *types.PriceState
    SetPriceFeedPriceState(ctx sdk.Context, oracleBase, oracleQuote string, priceState *types.PriceState)
    GetPriceFeedPrice(ctx sdk.Context, base string, quote string) *math.LegacyDec
}
```

## Provider

The `ProviderKeeper` provides the ability to manage provider info, relayers, and per-symbol price states for provider-based oracles.

```go
type ProviderKeeper interface {
    IsProviderRelayer(ctx sdk.Context, provider string, relayer sdk.AccAddress) bool
    GetProviderRelayers(ctx sdk.Context, provider string) []sdk.AccAddress
    DeleteProviderRelayers(ctx sdk.Context, provider string, relayers []string) error
    GetProviderInfo(ctx sdk.Context, provider string) *types.ProviderInfo
    SetProviderInfo(ctx sdk.Context, providerInfo *types.ProviderInfo) error
    GetAllProviderInfos(ctx sdk.Context) []*types.ProviderInfo
    GetProviderPriceState(ctx sdk.Context, provider, symbol string) *types.ProviderPriceState
    SetProviderPriceState(ctx sdk.Context, provider string, priceState *types.ProviderPriceState)
    GetProviderPriceStates(ctx sdk.Context, provider string) []*types.ProviderPriceState
    GetProviderPrice(ctx sdk.Context, provider, symbol string) *math.LegacyDec
    GetCumulativeProviderPrice(ctx sdk.Context, provider, symbol string) *math.LegacyDec
    GetAllProviderStates(ctx sdk.Context) []*types.ProviderState
    ProcessProviderPrices(ctx sdk.Context, msg *types.MsgRelayProviderPrices)
}
```

## Pyth

The `PythKeeper` provides the ability to relay and read Pyth price attestations.

```go
type PythKeeper interface {
    GetPythPrice(ctx sdk.Context, base, quote string) *math.LegacyDec
    ProcessPythPriceAttestations(ctx sdk.Context, priceAttestations []*types.PriceAttestation)
    SetPythPriceState(ctx sdk.Context, priceState *types.PythPriceState)
    GetPythPriceState(ctx sdk.Context, priceID common.Hash) *types.PythPriceState
    GetAllPythPriceStates(ctx sdk.Context) []*types.PythPriceState
}
```

## Stork

The `StorkKeeper` provides the ability to create/modify/read Stork price states and publishers.

```go
type StorkKeeper interface {
    GetStorkPrice(ctx sdk.Context, base string, quote string) *math.LegacyDec
    IsStorkPublisher(ctx sdk.Context, address string) bool
    SetStorkPublisher(ctx sdk.Context, address string)
    DeleteStorkPublisher(ctx sdk.Context, address string)
    GetAllStorkPublishers(ctx sdk.Context) []string

    SetStorkPriceState(ctx sdk.Context, priceData *types.StorkPriceState)
    GetStorkPriceState(ctx sdk.Context, symbol string) *types.StorkPriceState
    GetAllStorkPriceStates(ctx sdk.Context) []*types.StorkPriceState
}
```

`GetStorkPrice` returns the latest `value` field from the `StorkPriceState`.

## ChainlinkDataStreams

The `ChainlinkDataStreamsKeeper` provides the ability to create/modify/read Chainlink Data Streams price states.

```go
type ChainlinkDataStreamsKeeper interface {
    GetChainlinkDataStreamsPrice(ctx sdk.Context, base, quote string) *math.LegacyDec
    SetChainlinkDataStreamsPriceState(ctx sdk.Context, priceState *types.ChainlinkDataStreamsPriceState)
    GetChainlinkDataStreamsPriceState(ctx sdk.Context, feedID string) *types.ChainlinkDataStreamsPriceState
    GetAllChainlinkDataStreamsPriceStates(ctx sdk.Context) []*types.ChainlinkDataStreamsPriceState
}
```

Reports submitted via `MsgRelayChainlinkPrices` are verified against the Chainlink verifier proxy contract configured in module params before being stored.
