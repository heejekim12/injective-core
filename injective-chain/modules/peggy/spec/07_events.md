---
sidebar_position: 7
title: Events
---

# Events

The Peggy module emits the following events:

## EndBlocker

### EventAttestationObserved
| Type   | Attribute Key    | Attribute Value           |
|--------|------------------|---------------------------|
| int32  | attestation_type | {attestation_type}        |
| string | bridge_contract  | {bridge_contract_address} |
| uint64 | bridge_chain_id  | {bridge_chain_id}         |
| []byte | attestation_id   | {attestation_id}          |
| uint64 | nonce            | {event_nonce}             |

### EventValidatorJailed
| Type    | Attribute Key       | Attribute Value                                   |
|---------|---------------------|---------------------------------------------------|
| enum    | reason              | `MissingValsetConfirm` or `MissingBatchConfirm`   |
| int64   | power               | {validator_power}                                 |
| string  | consensus_address   | {consensus_addr}                                  |
| string  | operator_address    | {operator_addr}                                   |
| string  | moniker             | {validator_moniker}                               |

  
## Handler

### EventSubmitBadSignatureEvidence
| Type   | Attribute Key             | Attribute Value              |
|--------|---------------------------|------------------------------|
| string | bad_eth_signature         | {bad_eth_signature}          |
| string | bad_eth_signature_subject | {bad_eth_signature_subject}  |

### EventSetOrchestratorAddresses

| Type   | Attribute Key        | Attribute Value     |
|--------|----------------------|---------------------|
| string | validator_address    | {validator_addr}    |
| string | orchestrator_address | {orchestrator_addr} |
| string | operator_eth_address | {eth_addr}          |

### EventSendToEth

| Type     | Attribute Key  | Attribute Value |
|----------|----------------|-----------------|
| message  | outgoing_tx_id | {tx_id}         |
| string   | sender         | {sender_addr}   |
| string   | receiver       | {dest_addr}     |
| sdk.Coin | amount         | {token_amount}  |
| sdk.Coin | bridge_fee     | {token_amount}  |


### EventBridgeWithdrawCanceled
| Type                 | Attribute Key   | Attribute Value   |
|----------------------|-----------------|-------------------|
| withdrawal_cancelled | bridge_contract | {bridge_contract} |
| withdrawal_cancelled | bridge_chain_id | {bridge_chain_id} |


### EventOutgoingBatch

| Type     | Attribute Key        | Attribute Value |
|----------|----------------------|-----------------|
| string   | denom                | {token_denom}   |
| string   | orchestrator_address | {orch_addr}     |
| uint64   | batch_nonce          | {batch_nonce}   |
| uint64   | batch_timeout        | {block_height}  |
| []uint64 | batch_tx_ids         | {ids}           |

### EventOutgoingBatchCanceled
| Type   | Attribute Key   | Attribute Value   |
|--------|-----------------|-------------------|
| string | bridge_contract | {bridge_contract} |
| uint64 | bridge_chain_id | {bridge_chain_id} |
| uint64 | batch_id        | {id}              |
| uint64 | nonce           | {nonce}           |

### EventValsetConfirm

| Type   | Attribute Key        | Attribute Value |
|--------|----------------------|-----------------|
| uint64 | valset_nonce         | {nonce}         |
| string | orchestrator_address | {orch_addr}     |


### EventConfirmBatch

| Type   | Attribute Key        | Attribute Value |
|--------|----------------------|-----------------|
| uint64 | batch_nonce          | {nonce}         |
| string | orchestrator_address | {orch_addr}     |

> Note: `EventValidatorSlash` still exists in the protobuf definitions, but the current Peggy module code does not emit it.

### EventDepositClaim

| Type    | Attribute Key        | Attribute Value   |
|---------|----------------------|-------------------|
| uint64  | event_nonce          | {event_nonce}     |
| uint64  | event_height         | {event_height}    |
| []byte  | attestation_id       | {attestation_key} |
| string  | ethereum_sender      | {sender_addr}     |
| string  | cosmos_receiver      | {receiver_addr}   |
| string  | token_contract       | {contract_addr}   |
| sdk.Int | amount               | {token_amount}    |
| string  | orchestrator_address | {orch_addr}       |
| string  | data                 | {custom_data}     |


### EventWithdrawClaim

| Type   | Attribute Key        | Attribute Value   |
|--------|----------------------|-------------------|
| uint64 | event_nonce          | {event_nonce}     |
| uint64 | event_height         | {event_height}    |
| []byte | attestation_id       | {attestation_key} |
| uint64 | batch_nonce          | {batch_nonce}     |
| string | token_contract       | {contract_addr}   |
| string | orchestrator_address | {orch_addr}       |

### EventERC20DeployedClaim
| Type   | Attribute Key        | Attribute Value        |
|--------|----------------------|------------------------|
| uint64 | event_nonce          | {event_nonce}          |
| uint64 | event_height         | {event_height}         |
| []byte | attestation_id       | {attestation_key}      |
| string | cosmos_denom         | {token_denom}          |
| string | token_contract       | {token_contract_addr}  |
| string | name                 | {token_name}           |
| string | symbol               | {token_symbol}         |
| uint64 | decimals             | {token_decimals}       |
| string | orchestrator_address | {orch_addr}            |

### EventValsetUpdateClaim
| Type               | Attribute Key        | Attribute Value       |
|--------------------|----------------------|-----------------------|
| uint64             | event_nonce          | {event_nonce}         |
| uint64             | event_height         | {event_height}        |
| []byte             | attestation_id       | {attestation_key}     |
| uint64             | valset_nonce         | {valset_nonce}        |
| []*BridgeValidator | valset_members       | {array_of_validators} |
| sdk.Int            | reward_amount        | {amount}              |
| string             | reward_token         | {contract_addr}       |
| string             | orchestrator_address | {orch_addr}           |
