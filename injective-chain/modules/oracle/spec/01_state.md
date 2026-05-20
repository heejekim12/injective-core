---
sidebar_position: 1
title: State
---

# State

## Params

The oracle module parameters are stored at `ParamsKey` (`0x03`) in the module KV store.

```protobuf
message Params {
  string pyth_contract = 1;
  string chainlink_verifier_proxy_contract = 2;
  // field 3 is reserved (previously accept_unverified_chainlink_data_streams_reports)
  uint64 chainlink_data_streams_verification_gas_limit = 4;
}
```

where

- `pyth_contract` is the bech32 address of the Pyth contract that is allowed to relay Pyth prices.
- `chainlink_verifier_proxy_contract` is the Ethereum hex address of the Chainlink verifier proxy contract used to verify Chainlink Data Streams reports.
- `chainlink_data_streams_verification_gas_limit` is the EVM gas limit applied during Chainlink Data Streams report verification.

## PriceState

`PriceState` is a common type used to manage cumulative price and latest price along with timestamp for all oracle types.

```protobuf
message PriceState {
    string price = 1 [(gogoproto.customtype) = "cosmossdk.io/math.LegacyDec", (gogoproto.nullable) = false];
    string cumulative_price = 2 [(gogoproto.customtype) = "cosmossdk.io/math.LegacyDec", (gogoproto.nullable) = false];
    int64 timestamp = 3;
}
```

where

- `price` is the normalized decimal price.
- `cumulative_price` is the cumulative price for a given oracle price feed since the start of the oracle price feed's creation.
- `timestamp` is the block time at which the price state was last updated.

The `cumulative_price` follows the convention set by [Uniswap V2 Oracle](https://uniswap.org/docs/v2/core-concepts/oracles/) and is used to calculate Time-Weighted Average Price (TWAP) between two arbitrary block time intervals (t1, t2):

$\mathrm{TWAP = \frac{CumulativePrice_2 - CumulativePrice_1}{Timestamp_2 - Timestamp_1}}$

## Band (Deprecated)

> **Deprecated.** Band oracle price relaying via the direct Band relayer is no longer supported.

Band price data for a given symbol are stored as follows:

- BandPriceState: `0x01 | []byte(symbol) -> ProtocolBuffer(BandPriceState)`

```protobuf
// DEPRECATED! Oracle price from Band is no longer supported
message BandPriceState {
    string symbol = 1;
    string rate = 2 [(gogoproto.customtype) = "cosmossdk.io/math.Int", (gogoproto.nullable) = false];
    uint64 resolve_time = 3;
    uint64 request_ID = 4;
    PriceState price_state = 5 [(gogoproto.nullable) = false];
}
```

`rate` is the raw USD rate for the `symbol` obtained from Band chain, scaled by 1e9 (e.g. a price of 1.42 is stored as 1420000000), while `price_state` holds the normalized decimal price (e.g. 1.42).

Band relayers are stored by their address:

- BandRelayer: `0x02 | RelayerAddr -> []byte{}`

## Band IBC (Deprecated)

> **Deprecated.** Band IBC oracle price relaying is no longer supported.

- LatestClientID: `0x32 -> Formatted(LatestClientID)` — monotonically increasing ID for Band IBC packets.
- LatestRequestID: `0x36 -> Formatted(LatestRequestID)` — monotonically increasing ID for `BandIBCOracleRequest`s.
- BandIBCPriceState: `0x31 | []byte(symbol) -> ProtocolBuffer(BandPriceState)`
- CalldataRecord: `0x33 | []byte(ClientId) -> ProtocolBuffer(CalldataRecord)`

```protobuf
message CalldataRecord {
  uint64 client_id = 1;
  bytes calldata = 2;
}
```

- BandIBCOracleRequest: `0x34 | []byte(RequestId) -> ProtocolBuffer(BandOracleRequest)`

```protobuf
// DEPRECATED! Oracle price from Band is no longer supported
message BandOracleRequest {
  uint64 request_id = 1;
  int64 oracle_script_id = 2;
  repeated string symbols = 3;
  uint64 ask_count = 4;
  uint64 min_count = 5;
  repeated cosmos.base.v1beta1.Coin fee_limit = 6 [(gogoproto.nullable) = false, (gogoproto.castrepeated) = "github.com/cosmos/cosmos-sdk/types.Coins"];
  uint64 prepare_gas = 7;
  uint64 execute_gas = 8;
  uint64 min_source_count = 9;
}
```

- BandIBCParams: `0x35 -> ProtocolBuffer(BandIBCParams)`

```protobuf
// DEPRECATED! Oracle price from Band is no longer supported
message BandIBCParams {
  bool band_ibc_enabled = 1;
  int64 ibc_request_interval = 2;
  string ibc_source_channel = 3;
  string ibc_version = 4;
  string ibc_port_id = 5;
  repeated int64 legacy_oracle_ids = 6;
}
```

## Coinbase

Coinbase price data for a given symbol (`key`) are stored per (key, timestamp) tuple. Multiple historical entries per symbol are kept to support TWAP calculation.

- CoinbasePriceState: `0x21 | []byte(key) | uint64(timestamp) -> ProtocolBuffer(CoinbasePriceState)`

```protobuf
message CoinbasePriceState {
  // kind should always be "prices"
  string kind = 1;
  // timestamp of when the price was signed by Coinbase
  uint64 timestamp = 2;
  // the symbol of the price, e.g. BTC
  string key = 3;
  // the value of the price scaled by 1e6
  uint64 value = 4;
  // the price state
  PriceState price_state = 5 [(gogoproto.nullable) = false];
}
```

`value` is the raw USD price scaled by 1e6 (e.g. a price of 1.42 is stored as 1420000), while `price_state` holds the normalized decimal price (e.g. 1.42).

More details about the Coinbase price oracle can be found in the [Coinbase API docs](https://docs.pro.coinbase.com/#oracle) as well as this explanatory [blog post](https://blog.coinbase.com/introducing-the-coinbase-price-oracle-6d1ee22c7068).

The `GetCoinbasePrice` query returns a 5-minute TWAP price computed from the stored historical states.

## Pricefeed

Pricefeed price data for a given base/quote pair are stored as follows:

- PriceFeedInfo: `0x11 | Keccak256Hash(base + quote) -> ProtocolBuffer(PriceFeedInfo)`

```protobuf
message PriceFeedInfo {
  string base = 1;
  string quote = 2;
}
```

- PriceFeedPriceState: `0x12 | Keccak256Hash(base + quote) -> ProtocolBuffer(PriceFeedState)`

```protobuf
message PriceFeedState {
  string base = 1;
  string quote = 2;
  PriceState price_state = 3;
  repeated string relayers = 4;
}
```

- PriceFeedRelayer: `0x13 | Keccak256Hash(base + quote) | relayerAddr -> relayerAddr`

## Provider

Provider price feeds are stored as follows:

- ProviderInfo: `0x61 | provider | @@@ -> ProtocolBuffer(ProviderInfo)`

```protobuf
message ProviderInfo {
  string provider = 1;
  repeated string relayers = 2;
}
```

The `@@@` delimiter enforces uniqueness when iterating by provider name prefix.

- ProviderIndex: `0x62 | relayerAddress -> provider` — maps a relayer address back to its provider name.

- ProviderPrices: `0x63 | provider | @@@ | symbol -> ProtocolBuffer(ProviderPriceState)`

```protobuf
message ProviderPriceState {
  string symbol = 1;
  PriceState state = 2;
}
```

## Pyth

Pyth prices are stored as follows:

- PythPriceState: `0x71 | priceID (32-byte hash) -> ProtocolBuffer(PythPriceState)`

```protobuf
message PythPriceState {
  string price_id = 1;
  string ema_price = 2 [(gogoproto.customtype) = "cosmossdk.io/math.LegacyDec", (gogoproto.nullable) = false];
  string ema_conf = 3 [(gogoproto.customtype) = "cosmossdk.io/math.LegacyDec", (gogoproto.nullable) = false];
  string conf = 4 [(gogoproto.customtype) = "cosmossdk.io/math.LegacyDec", (gogoproto.nullable) = false];
  uint64 publish_time = 5;
  PriceState price_state = 6 [(gogoproto.nullable) = false];
}
```

The storage key is the 32-byte value obtained by interpreting `price_id` as a hex-encoded hash (`common.HexToHash(price_id)`). No additional hashing is applied — the price ID itself is already a 32-byte identifier.

## Stork

Stork prices are stored as follows:

- StorkPriceState: `0x81 | []byte(symbol) -> ProtocolBuffer(StorkPriceState)`

```protobuf
message StorkPriceState {
  // timestamp of when the price was signed by Stork (nanoseconds)
  uint64 timestamp = 1;
  // the symbol of the price, e.g. BTC
  string symbol = 2;
  // the value of the price scaled by 1e18
  string value = 3 [
    (gogoproto.customtype) = "cosmossdk.io/math.LegacyDec",
    (gogoproto.nullable) = false
  ];
  // the price state
  PriceState price_state = 5 [(gogoproto.nullable) = false];
}
```

Stork publishers are stored as follows:

- Publisher: `0x82 | stork_publisher -> stork_publisher`

## Chainlink Data Streams

Chainlink Data Streams prices are stored as follows:

- ChainlinkDataStreamsPriceState: `0x91 | []byte(feedID) -> ProtocolBuffer(ChainlinkDataStreamsPriceState)`

```protobuf
message ChainlinkDataStreamsPriceState {
  string feed_id = 1;
  string report_price = 2 [(gogoproto.customtype) = "cosmossdk.io/math.Int", (gogoproto.nullable) = false];
  uint64 valid_from_timestamp = 3;
  uint64 observations_timestamp = 4;
  PriceState price_state = 5 [(gogoproto.nullable) = false];
  uint64 expires_at = 6;
}
```

`report_price` is the raw price value from the Chainlink report; `price_state` holds the normalized decimal price. Each feed is keyed by its `feedID` string.

Reports are submitted via `MsgRelayChainlinkPrices` and verified on-chain using the Chainlink verifier proxy contract configured in module params.

## Legacy Chainlink (Deprecated)

> **Deprecated.** The legacy Chainlink oracle type (`0x41`) has been replaced by Chainlink Data Streams (`0x91`).

## PythPro (Pyth Lazer)

PythPro prices are stored as follows:

- PythProPriceState: `0xA1 | uint32(feedID) (4 bytes BE) -> ProtocolBuffer(PythProPriceState)`

```protobuf
// PythProPriceState holds the verified price state for a single PythPro feed.
message PythProPriceState {
  // feed_id is the uint32 Pyth Lazer feed identifier.
  uint32 feed_id = 1;
  // timestamp is the price timestamp extracted from the verified payload
  // (microseconds from epoch).
  uint64 timestamp = 2;
  PriceState price_state = 3 [(gogoproto.nullable) = false];
}
```

Each feed is keyed by its 4-byte big-endian `feedID` (a `uint32`). Updates are submitted via `MsgRelayPythProPrices` and verified on-chain using the PythLazer verifier EVM contract configured in module params.

## SEDA Fast

SEDA Fast prices are stored as follows:

- SedaFastPriceState: `0xB1 | keccak256(feedID) (32 bytes) -> ProtocolBuffer(SedaFastPriceState)`

```protobuf
// SedaFastPriceState holds the verified price state for a single SEDA Fast feed.
message SedaFastPriceState {
  // feed_id is the hex-encoded execInputs string from the SEDA Fast
  // dataRequest. It is the stable on-chain identity of the feed, invariant
  // across relayer restarts and per-execution result IDs.
  string feed_id = 1;
  // timestamp is the dataResult.blockTimestamp value (milliseconds from
  // epoch) extracted from the verified SEDA Fast response.
  uint64 timestamp = 2;
  PriceState price_state = 3 [(gogoproto.nullable) = false];
}
```

The store key uses `keccak256(feedID)` (32 bytes) for a fixed-length key, allowing safe prefix iteration. The original `feed_id` string is preserved inside the proto value so it can be recovered during iteration without reversing the hash.

`feed_id` is the hex-encoded `execInputs` field from the SEDA Fast data request — the stable, human-readable identifier for the feed that the relayer subscribes to. It is used as the `OracleInfo.Symbol` when creating derivative markets backed by a SEDA Fast feed.

Updates are submitted via `MsgRelaySedaFastPrices` as raw JSON envelopes. Each envelope is validated on-chain:
1. The `drId` is reconstructed from `dataRequest` fields and compared against `dataResult.drId`.
2. A `dataResultId` is derived from `dataResult` fields and the `secp256k1` ECDSA signature is verified against the SEDA Fast public key configured in `SedaFastParams.public_key`.
3. A monotonic-timestamp guard is applied: an envelope is accepted only if `dataResult.blockTimestamp` is strictly greater than the last stored `SedaFastPriceState.timestamp` for that feed. The exchange-visible `price_state.timestamp` records the relay block time (`ctx.BlockTime()`) used for cumulative-price/TWAP accounting.
4. The `result` bytes are decoded using the decoder selected by `execProgramId` (simple ASCII decimal or JSON mantissa/exponent).

See [SEDA Fast documentation](https://docs.seda.xyz/home/for-developers/define-your-delivery-method/seda-fast) for details on the identifier derivation formulas.

## Historical Price Records

The following oracle types append price records to a rolling per-symbol history used for volatility and TWAP queries: PriceFeed, Coinbase, Provider, Pyth, Stork, Chainlink Data Streams, and BandIBC (deprecated). Legacy Band (direct) and legacy Chainlink do not append historical records.

- HistoricalPriceRecords: `0x51 | []byte(oracleType + "_" + symbol) -> ProtocolBuffer(PriceRecords)` — stores up to `MaxHistoricalPriceRecordAge` (300 seconds) of recent price records.
- LastPriceTimestamps: `0x52 -> ProtocolBuffer(LastPriceTimestamps)` — a map of the latest price update timestamp per (oracle type, symbol) pair.

```protobuf
message PriceRecords {
  OracleType oracle = 1;
  string symbol_id = 2;
  repeated PriceRecord latest_price_records = 3;
}

message PriceRecord {
  int64 timestamp = 1;
  string price = 2 [(gogoproto.customtype) = "cosmossdk.io/math.LegacyDec", (gogoproto.nullable) = false];
}

message LastPriceTimestamps {
  repeated SymbolPriceTimestamp last_price_timestamps = 1;
}

message SymbolPriceTimestamp {
  OracleType oracle = 1;
  string symbol_id = 2;
  int64 timestamp = 3;
}
```
