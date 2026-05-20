# Pyth Pro (Pyth Lazer) assistant

This package implements the on-chain side of **Pyth Pro** price relay for Injective: it verifies an update via the configured `PythLazer`-style EVM contract (`verifyUpdate`), parses the **EVM binary payload** returned by that call, applies **mantissa × 10^exponent**, and stores prices in the oracle module.

It is **not** a full Pyth Lazer client. Off-chain, relayers obtain signed updates from Pyth's services and submit `MsgRelayPythProPrices`; only the **verified** byte blob from `verifyUpdate` is trusted on-chain.

## Official documentation

- [WebSocket API](https://docs.pyth.network/price-feeds/pro/api/websocket)
- [Payload reference](https://docs.pyth.network/price-feeds/pro/payload-reference)
- [Subscribe to prices](https://docs.pyth.network/price-feeds/pro/subscribe-to-prices)
- [Consume data on EVM chains](https://docs.pyth.network/price-feeds/pro/integrate-as-consumer/evm)

## Official GitHub (reference implementations)

- [`pyth-network/pyth-crosschain`](https://github.com/pyth-network/pyth-crosschain) — monorepo
- [`lazer/contracts/evm/src/PythLazerLib.sol`](https://github.com/pyth-network/pyth-crosschain/blob/main/lazer/contracts/evm/src/PythLazerLib.sol) — binary layout and parsing
- [`lazer/contracts/evm/src/PythLazerStructs.sol`](https://github.com/pyth-network/pyth-crosschain/blob/main/lazer/contracts/evm/src/PythLazerStructs.sol) — property enum and structs

There is **no official Go SDK** for Pyth Lazer; this parser matches **`PythLazerLib` wire layout** (magic, header, per-feed property stream, big-endian field sizes). It does **not** replicate every Solidity-level semantic check (e.g. enum validity for `MarketSession`, ranges for `PublisherCount`); properties other than those used for price and timestamps are skipped on-chain.

## Relayer subscription

Relayers should subscribe so that the **EVM** payload includes at least:

- `price`
- `exponent`
- `feedUpdateTimestamp` (recommended for per-feed staleness)

Use `formats: ["evm"]` (or equivalent) so the wire format matches `PythLazerLib`. Only **`price`**, **`exponent`**, and optional **`feedUpdateTimestamp`** affect consensus state; other properties (IDs 0–12) are read for cursor advancement only and are not validated for business meaning. Property IDs **greater than 12** are rejected, consistent with `require(propertyId <= 12)` in the Solidity parser.

## EVM payload layout (summary)

| Section | Size | Content |
|--------|------|---------|
| Magic | 4 | `0x93A7B6D5` (uint32 BE) |
| Header timestamp | 8 | `uint64` μs |
| Channel | 1 | ignored |
| `feedsLen` | 1 | number of feeds |

Per feed:

| Field | Size |
|-------|------|
| `feedId` | 4 (`uint32` BE) |
| `numProperties` | 1 |
| Per property | 1 byte property id + value |

Property IDs 0–12 use the **same byte layout** as `PythLazerLib` (e.g. `Price` int64 8 bytes, `Exponent` int16 2 bytes, funding-related properties with a 1-byte exists prefix, `FeedUpdateTimestamp` with exists + optional `uint64`, etc.). Value-level constraints from upstream Solidity are not enforced for skipped properties.

The payload must be **fully consumed** (no trailing bytes), matching `PythLazerLib.parseUpdateFromPayload`.

## Price computation

On-chain stored price uses Cosmos `LegacyDec`:

`actual_price = mantissa × 10^exponent`

`exponent` is typically negative for USD quotes (e.g. mantissa `10^9`, exponent `-9` → `1.0`).

## Timestamps

Staleness and ordering use the per-feed **FeedUpdateTimestamp** when that property is present with `exists != 0`; otherwise the batch **header timestamp** is used.

## Security model and gas / DoS protection

- **Trusted data path:** only the `payload` bytes returned from the verified `verifyUpdate` EVM call are parsed and used for prices.
- **Untrusted:** any JSON or WebSocket "parsed" view used off-chain for debugging must **not** be passed as the relay payload; it is not authoritative for consensus state.

### Verifier gas metering

Every `verifyUpdate` call is a read-only `eth_call` against the configured verifier contract. Before the call, the keeper clamps the `eth_call` gas cap to the relay tx's **remaining Cosmos gas** (`min(PythProVerificationGasLimit, ctx.GasMeter().GasRemaining())`), so the EVM cannot perform more work than the sender already reserved. The EVM gas reported for the call is then charged directly on the Cosmos gas meter via `ctx.GasMeter().ConsumeGas`. If the accumulated charge exceeds the meter limit, the standard SDK `ErrorOutOfGas` panic is raised — the same mechanism used for all over-budget txs, recovered by `BaseApp.runTx`.

If `EthCall` returns a hard error (not a successful response with `VmError`), the keeper charges the clamped cap against the Cosmos meter so that error paths cannot be used as a free DoS channel.

The per-call EVM gas is also bounded above by the `PythProVerificationGasLimit` module parameter (before the remaining-gas clamp). Module parameter validation requires this limit to be strictly greater than zero, so governance cannot set it to zero via `MsgUpdateParams` (which would otherwise hard-disable verification).

This means:
- A relayer batching N updates into one `MsgRelayPythProPrices` pays the verifier gas for all N, regardless of whether individual updates pass or fail.
- There is no "free ride" from packing garbage blobs alongside valid ones.

### Batch failure semantics

- **Verifier revert / EVM error** (`ErrPythProVerificationFailed`): the entire batch is aborted immediately and the tx returns an error. In a real chain context this reverts all state written by earlier updates in the same tx.
- **Post-verify errors** (invalid payload binary, stale timestamp, price out of accepted range, etc.): treated as best-effort. The failing update is skipped and sibling updates continue to be processed. A batch returns success as long as at least one update is accepted.
