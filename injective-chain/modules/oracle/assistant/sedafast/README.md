# SEDA Fast assistant

This package implements the on-chain side of **SEDA Fast** price relay for Injective: it validates a signed SEDA Fast JSON envelope, reconstructs the deterministic `drId` and `dataResultId` identifiers via `keccak256`, verifies the `secp256k1` ECDSA signature, hex-decodes the signed `data.dataResult.result` bytes into a price, and stores the price in the oracle module.

It is **not** a full SEDA client. Off-chain, a relayer subscribes to the SEDA Fast WebSocket stream and submits `MsgRelaySedaFastPrices` with the raw JSON envelopes. Only on-chain signature verification is trusted for consensus state.

## Official documentation

- [SEDA Fast overview](https://docs.seda.xyz/home/for-developers/define-your-delivery-method/seda-fast)
- [SEDA Fast REST API reference](https://docs.seda.xyz/home/for-developers/define-your-delivery-method/seda-fast/rest-api)
- [SEDA Fast WebSocket reference](https://docs.seda.xyz/home/for-developers/define-your-delivery-method/seda-fast/websocket)
- [Advanced usage / identifier derivation](https://docs.seda.xyz/home/for-developers/define-your-delivery-method/seda-fast/advanced-usage)
- [Agent getting started (for relayer context)](https://docs.seda.xyz/home/for-agents/getting-started)

The SEDA Fast public key (used for signature verification) is obtained from the **`GET /info`** endpoint of the SEDA Fast service. This key is stored in module params as `SedaFastParams.public_key` and must be configured via a governance `MsgUpdateParams` before relaying begins.

## JSON envelope → Go struct mapping

The raw bytes in each `MsgRelaySedaFastPrices.updates` entry are a JSON object matching `SedaFastEnvelope`. The fields below describe the mapping from the SEDA Fast wire format to the Go types used on-chain.

| SEDA Fast JSON path | Go struct field | Notes |
|---|---|---|
| `_tag` | `SedaFastEnvelope.Tag` | e.g. `"feed"` |
| `data.id` | `SedaFastRespData.ID` | request ID in the SEDA system |
| `data.requestId` | `SedaFastRespData.RequestID` | |
| `data.signature` | `SedaFastRespData.Signature` | 65-byte hex-encoded secp256k1 signature |
| `data.result` | *(not captured)* | Off-chain convenience mirror; **not signed**, **not used on-chain** — ignored during JSON decoding |
| `data.dataRequest.version` | `SedaFastDataRequest.Version` | |
| `data.dataRequest.execProgramId` | `SedaFastDataRequest.ExecProgramID` | 64-char hex; used to select the price parser |
| **`data.dataRequest.execInputs`** | `SedaFastDataRequest.FeedID` | **feed identifier** — see "feedId rationale" below |
| `data.dataRequest.execGasLimit` | `SedaFastDataRequest.ExecGasLimit` | string-encoded uint64 |
| `data.dataRequest.tallyProgramId` | `SedaFastDataRequest.TallyProgramID` | 64-char hex |
| `data.dataRequest.tallyInputs` | `SedaFastDataRequest.TallyInputs` | hex |
| `data.dataRequest.tallyGasLimit` | `SedaFastDataRequest.TallyGasLimit` | string-encoded uint64 |
| `data.dataRequest.replicationFactor` | `SedaFastDataRequest.ReplicationFactor` | uint16 |
| `data.dataRequest.consensusFilter` | `SedaFastDataRequest.ConsensusFilter` | hex |
| `data.dataRequest.gasPrice` | `SedaFastDataRequest.GasPrice` | string-encoded big integer (up to 128-bit) |
| `data.dataRequest.memo` | `SedaFastDataRequest.Memo` | string |
| `data.dataResult.version` | `SedaFastDataResult.Version` | |
| `data.dataResult.drId` | `SedaFastDataResult.DrID` | 64-char hex; reconstructed on-chain for integrity |
| `data.dataResult.consensus` | `SedaFastDataResult.Consensus` | must be `true` |
| `data.dataResult.exitCode` | `SedaFastDataResult.ExitCode` | must be `0` |
| **`data.dataResult.result`** | `SedaFastDataResult.Result` | **hex-encoded Oracle Program output; signed** (part of the `dataResultId` preimage); hex-decoded on-chain and passed to the price parser as the authoritative price bytes |
| `data.dataResult.blockHeight` | `SedaFastDataResult.BlockHeight` | always `"0"` for SEDA Fast |
| `data.dataResult.blockTimestamp` | `SedaFastDataResult.BlockTimestamp` | milliseconds since Unix epoch (string) |
| `data.dataResult.gasUsed` | `SedaFastDataResult.GasUsed` | string-encoded big integer (up to 128-bit) |
| `data.dataResult.paybackAddress` | `SedaFastDataResult.PaybackAddress` | hex |
| `data.dataResult.sedaPayload` | `SedaFastDataResult.SedaPayload` | hex |

## feedId rationale

In the SEDA Fast protocol the wire field is called `execInputs`. On-chain the chain uses the name **feedId** everywhere, because `execInputs` is opaque to the chain and its sole purpose is to identify which price feed a result belongs to.

Concretely:
- `SedaFastDataRequest.FeedID` (Go) maps to `json:"execInputs"` (wire).
- The on-chain feed identity key stored in state, emitted in events, and used as the market `OracleInfo.Symbol` is the **canonical** form of the `feedId` bytes.

### Canonical form

The canonical SEDA Fast feed ID is defined as:

- **Non-empty** — an empty feed ID is rejected.
- **No `0x` / `0X` prefix** — any such prefix is stripped during relay and rejected at market creation.
- **Lowercase hex only** (`0–9`, `a–f`) — uppercase characters are rejected at market creation; during relay, mixed/uppercase hex is normalised to lowercase.
- **Even length** — an odd number of characters is not a valid hex byte sequence.

This is exactly the form produced by `hex.EncodeToString(bytes)`.

**Enforcement points:**

| Entry point | Behaviour |
|---|---|
| `processUpdate` (relay assistant) | Strips `0x`/`0X` prefix, hex-decodes, re-encodes to lowercase. Invalid hex rejects the update (best-effort). |
| `SpotMarket` / `DerivativeMarket` / `BinaryOptionsMarket` proposals (`OracleType_SedaFast`) | `ValidateBasic` rejects `OracleBase` / `OracleQuote` that do not satisfy `ValidateCanonicalSedaFastFeedID`. USD quote is always accepted as-is. |

This guarantees that `GetSedaFastPriceStoreKey(feedID)` — which hashes the string with `keccak256` — always receives the same byte sequence for the same logical feed, regardless of how the relayer encoded `execInputs`.

## Identifier derivation (on-chain integrity checks)

All formulae are from the [Advanced usage](https://docs.seda.xyz/home/for-developers/define-your-delivery-method/seda-fast/advanced-usage) SEDA docs page.

### drId

```
drId = keccak256(
  keccak256(version)        ||
  execProgramId (32 bytes)  ||
  keccak256(execInputs)     ||
  execGasLimit  (8 bytes BE)||
  tallyProgramId(32 bytes)  ||
  keccak256(tallyInputs)    ||
  tallyGasLimit (8 bytes BE)||
  replicationFactor(2 bytes BE)||
  keccak256(consensusFilter)||
  gasPrice      (16 bytes BE)||
  keccak256(memo)
)
```

The chain reconstructs `drId` from the `dataRequest` fields and compares it against `dataResult.drId`. A mismatch causes the update to be skipped (best-effort).

### dataResultId

```
dataResultId = keccak256(
  keccak256(version)         ||
  drId           (32 bytes)  ||
  consensus      (1 byte)    ||
  exitCode       (1 byte)    ||
  keccak256(result)          ||
  blockHeight    (8 bytes BE)||
  blockTimestamp (8 bytes BE)||
  gasUsed        (16 bytes BE)||
  keccak256(paybackAddress)  ||
  keccak256(sedaPayload)
)
```

The chain derives `dataResultId` from the `dataResult` fields and verifies the `signature` is a valid secp256k1 ECDSA signature over `dataResultId` by the SEDA Fast public key configured in `SedaFastParams.public_key`. A signature failure aborts the entire batch.

## Price parsers

The `execProgramId` determines how the bytes from the signed `data.dataResult.result` field are interpreted. The chain hex-decodes those bytes and passes them to the selected parser:

| Program list | Parser | Format of decoded bytes |
|---|---|---|
| `SedaFastParams.simple_program_ids` | `simplePriceParser` | UTF-8 ASCII decimal string, e.g. `"384.48255"` |
| `SedaFastParams.json_program_ids` | `jsonPriceParser` | UTF-8 JSON object `{"price":{"mantissa":"<int>","expo":<int>}}` |

The unsigned top-level `data.result` wire field is intentionally **not** captured by the Go struct and plays no role in on-chain consensus. Using it for pricing would allow any relayer to inject arbitrary prices with a recycled valid signature.

For the JSON parser the actual price is computed as `mantissa × 10^expo` using arbitrary precision arithmetic (Cosmos `LegacyDec`). The exponent magnitude is bounded to `MaxSedaFastExponent` (18) — matching `LegacyDec` 18-decimal precision and comfortably exceeding any real-world Pyth/SEDA feed — to prevent unbounded `big.Int` exponentiation.

## Batch failure semantics

- **Signature verification failure** (`ErrSedaFastVerificationFailed`): the entire batch is aborted immediately. No state is written for any update in the message. This prevents a byzantine relayer from slipping a maliciously signed envelope alongside valid ones.
- **All other errors** (malformed envelope JSON, unknown program, non-zero exit code, no consensus, drId or `dataResultId` derivation failure, payload malformed, non-positive price, parse failure): treated as **best-effort**. The failing update is logged at warn level and skipped; sibling updates continue to be processed.

## Security model

- `SedaFastParams.public_key` is the SEC1-encoded secp256k1 public key for signature verification, obtained from `GET /info` on the SEDA Fast service. An empty key **disables** the oracle type until a governance proposal installs it.
- Strict **low-S** enforcement is applied (same policy as Stork and Peggy) to prevent signature malleability.
- Each `MsgRelaySedaFastPrices` is limited to `MaxSedaFastUpdatesPerMsg` (64) updates, each at most `MaxSedaFastUpdateSize` (8 KiB).
- **Signed price source:** price bytes are taken exclusively from `data.dataResult.result` (covered by the signature). The unsigned top-level `data.result` field is not decoded by the chain; it cannot influence stored prices.
- **JSON exponent bound:** JSON feeds reject `|expo| > MaxSedaFastExponent` (18) to prevent attacker-controlled `big.Int` exponentiation via `LegacyDec.Power`.
- **Timestamp handling:** the chain does **not** compare SEDA Fast `dataResult.blockTimestamp` to Injective `ctx.BlockTime()` (validator wall clocks are not a reliable cross-chain reference). There is no on-chain “max age” or staleness parameter for SEDA timestamps.
- **Per-feed monotonic ordering:** for a given feed, an update is written only if `dataResult.blockTimestamp` is **strictly greater** than the timestamp already stored for that feed. If the incoming timestamp is equal or older, the relay still succeeds for that update (no batch error), but state and `EventOraclePriceUpdate` are left unchanged—the relayer simply moves on. The stored `PriceState` block time is still Injective’s `ctx.BlockTime()` when a write occurs.

## Operator runbook

### First-time setup (after v1.20.0 upgrade)

The v1.20.0 upgrade handler pre-configures SEDA Fast with the official SEDA Fast public key (`025316f89e976d2e41b1437f7a6552a547a1e4900e861ca485c842ed3f016a7824`) and the initial simple and JSON program ID allowlists. No manual governance proposal is required to enable relaying after this upgrade.

To confirm the oracle is active after the upgrade, run:

```
injectived query oracle seda-fast-price-states
```

After the relayer begins submitting `MsgRelaySedaFastPrices`, new entries will appear in the output.

### Update public key (e.g. key rotation)

Repeat the governance step above with the new public key.

### Disable SEDA Fast

Submit a governance `MsgUpdateParams` that sets `seda_fast_params.public_key` to an empty byte slice. This causes `MsgRelaySedaFastPrices` to return `ErrSedaFastDisabled` for all submissions.

### Manage allowed program IDs

- Add a new simple program: include its 64-char hex ID in `seda_fast_params.simple_program_ids` via governance `MsgUpdateParams`.
- Add a new JSON program: include its 64-char hex ID in `seda_fast_params.json_program_ids` via governance `MsgUpdateParams`.
- IDs cannot appear in both lists simultaneously; `ValidateSedaFastParams` will reject such params.
