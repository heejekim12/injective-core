# `Oracle`

## Abstract

This specification describes the oracle module, which is primarily used by the `exchange` module to obtain external price data.

## Supported Oracle Types

The module currently supports the following active oracle types:

- **PriceFeed** — permissioned relayers submit prices directly for a base/quote pair.
- **Coinbase** — anyone can relay Coinbase-signed price messages (ECDSA verified against the Coinbase oracle address).
- **Provider** — permissioned relayers submit per-symbol prices under a named provider string.
- **Pyth** — the Pyth contract (configured in params) relays price attestations.
- **Stork** — permissioned publishers relay ECDSA-signed price messages.
- **Chainlink Data Streams** — any sender can submit Chainlink reports; each report is verified against the on-chain verifier proxy contract configured in params.
- **PythPro (Pyth Lazer)** — any sender can submit signed Pyth Lazer updates; each update is verified against the on-chain PythLazer verifier contract configured in params.
- **SEDA Fast** — any sender can submit batches of signed SEDA Fast result envelopes; each envelope is verified on-chain via `secp256k1` ECDSA over a `keccak256`-derived `dataResultId`.

> **Note**: Band and Band IBC oracle types are deprecated and are no longer supported.

## Workflow

1. New oracle providers must first be authorized through a governance proposal that grants privileges to a list of relayers or publishers.
   - **PriceFeed**: `GrantPriceFeederPrivilegeProposal`
   - **Provider**: `GrantProviderPrivilegeProposal`
   - **Stork**: `GrantStorkPublisherPrivilegeProposal`
   - **Pyth** uses the module params (`MsgUpdateParams`) to configure the Pyth contract address; only that address can relay prices.
   - **Chainlink Data Streams** uses the module params to configure the verifier proxy contract address; any sender can submit reports, which are verified on-chain.
   - **PythPro** uses the module params to configure the PythLazer verifier contract address; any sender can submit updates, which are verified on-chain.
   - **SEDA Fast** uses the module params to configure the SEDA Fast public key and allowed program ID allowlists; any sender can submit updates, which are verified on-chain via secp256k1 ECDSA. An empty public key disables the oracle until governance configures it.
   - **Coinbase**: no proposal required — anyone can submit messages since they are signed by the Coinbase oracle key.

2. Once authorized, relayers submit oracle data using relay messages specific to their oracle type:
   - `MsgRelayPriceFeedPrice`
   - `MsgRelayProviderPrices`
   - `MsgRelayStorkPrices`
   - `MsgRelayPythPrices`
   - `MsgRelayCoinbaseMessages`
   - `MsgRelayChainlinkPrices`
   - `MsgRelayPythProPrices`
   - `MsgRelaySedaFastPrices`

3. Upon receiving a relay message, the oracle module verifies that the sender is authorized (or performs signature/contract verification), then persists the latest price data in state.

4. Other Cosmos SDK modules fetch the latest price data by querying the oracle module keeper (e.g. via `ViewKeeper.GetReferencePrice`).

5. Module parameters (Pyth contract address, Chainlink verifier proxy contract address, Chainlink verification gas limit, PythPro verifier contract address, PythPro verification gas limit, PythPro verification fee, and the nested `SedaFastParams` block) can be updated by the module authority via `MsgUpdateParams`. The two EVM verification gas limits must each be greater than zero; `Params.Validate` rejects zero so a mistaken governance update cannot disable all Chainlink Data Streams or PythPro verification at the parameter layer. For SEDA Fast, setting `seda_fast_params.public_key` to an empty slice disables the oracle type without a code change.

**Note**: Privileges can be revoked through governance:
- `RevokePriceFeederPrivilegeProposal`
- `RevokeProviderPrivilegeProposal`
- `RevokeStorkPublisherPrivilegeProposal`

## EVM-verified oracles: gas and DoS protection

Both **Chainlink Data Streams** and **PythPro** verify price data via a read-only `eth_call` against an on-chain verifier contract before accepting it. To prevent a single relay tx from forcing arbitrarily expensive verifier executions without paying for them:

- **Verifier gas is charged on Cosmos**: each `eth_call` uses a gas cap of `min(*VerificationGasLimit, remaining Cosmos gas on the tx)`, where `*VerificationGasLimit` is the module param (`ChainlinkDataStreamsVerificationGasLimit` or `PythProVerificationGasLimit`), which must be strictly positive. Under-gassed txs cannot force more verifier work than the sender reserved. On success or `VmError`, the reported `GasUsed` is charged directly on `ctx.GasMeter()`; if accumulated charges exceed the tx gas limit, the SDK's standard `ErrorOutOfGas` panic is raised (recovered by `BaseApp.runTx`). If `EthCall` itself returns an error, the clamped cap is charged instead so error paths are not free.
- **Atomic failure on verifier error**: if the verifier contract reverts or the EVM call fails for any report/update in a batch, the entire relay message is aborted and returns an error. This prevents a "1 valid + N garbage" packing strategy.
- **Best-effort on post-verify errors**: if the verifier succeeds but a subsequent step fails (e.g., payload parse error, stale timestamp, price out of range), that individual item is skipped and sibling items continue to be processed.

## Contents

1. [State](./01_state.md)
2. [Keeper](./02_keeper.md)
3. [Messages](./03_messages.md)
4. [Proposals](./04_proposals.md)
5. [Events](./05_events.md)
6. [Improvements](./06_future_improvements.md)
7. [Errors](./99_errors.md)
