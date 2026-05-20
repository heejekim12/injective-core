---
sidebar_position: 5
title: Validator Penalties
---

# Validator Penalties

This file keeps the historical `PEGGYSLASH-*` names for continuity, but the current Peggy implementation does not currently slash validator stake in these flows:

- Missing valset confirmations lead to jailing, not stake reduction.
- Missing batch confirmations lead to jailing, not stake reduction.
- Accepted bad-signature evidence currently jails the offending validator and records the fake checkpoint so the same evidence cannot be applied twice.
- Claim-based penalties remain disabled.
- The `slash_fraction_*` parameters are still part of `Params`, but the current Peggy penalty paths do not call `StakingKeeper.Slash`.

### Security Concerns

The **Validator Set** is the actual set of keys with stake behind them, which are slashed for double-signs or other
misbehavior. We typically consider the security of a chain to be the security of a _Validator Set_. This varies on
each chain, but is our gold standard. Even IBC offers no more security than the minimum of both involved Validator Sets.

The **Eth bridge relayer** is a binary run alongside the main `injectived` daemon by the validator set. It exists purely as a matter of code organization and is in charge of signing Ethereum transactions, as well as observing events on Ethereum and bringing them into the Injective Chain state. It signs transactions bound for Ethereum with an Ethereum key, and signs over events coming from Ethereum with an Injective Chain account key. Historically this spec described multiple slashable Peggy behaviors. The current code keeps the same high-level threat model, but the implemented penalties are narrower and currently use jailing rather than stake reduction.

Below are the validator-penalty conditions currently modeled in Peggy.

## PEGGYSLASH-01: Bad signature evidence

This penalty path is intended to stop validators from signing a valset or transaction batch that never existed on the Injective Chain. It works via an evidence mechanism where anyone can submit a message containing the signature of a validator over a fake checkpoint.

```go
// This call allows anyone to submit evidence that a
// validator has signed a valset or batch that never
// existed. Subject contains the batch or valset.
type MsgSubmitBadSignatureEvidence struct {
	Subject   *types1.Any 
	Signature string      
	Sender    string      
}
```

**Current implementation:**

- The handler currently accepts evidence over `Valset` and `OutgoingTxBatch`.
- Peggy computes the Ethereum checkpoint for the submitted subject and checks whether that checkpoint was previously recorded as legitimate.
- Legitimate checkpoints are recorded when Peggy creates a valset request or stores a batch.
- If the checkpoint is unknown, Peggy derives the signer Ethereum address from the submitted signature, maps that address back to a validator, and jails that validator if they are not already jailed.
- Peggy then records the `(fake checkpoint, validator)` pair so the same evidence cannot be applied twice.
- The current code does not reduce stake in this flow.

Currently automatic evidence submission is not implemented in the relayer. By the time a signature is visible on Ethereum it is already too late for this penalty path to prevent bridge hijacking or theft of funds. The most realistic use of this mechanism is still to make validator collusion harder by giving defectors a way to submit evidence against a fake valset or batch.

The theft would involve exchanging of slashable Ethereum signatures and open up the possibility of a manual submission of this message by any defector in the group.

Currently this is implemented as an ever-growing archive of legitimate checkpoints plus a record of fake checkpoints that have already been handled for a given validator.

## PEGGYSLASH-02: Failure to sign tx batch (implemented as jailing)

This penalty path is triggered when a validator does not confirm a transaction batch within `SignedBatchesWindow` after the batch is created by the Peggy module.

The current EndBlocker logic:

- Only checks batches that are old enough to have passed the configured window.
- Only checks bonded validators.
- Skips validators that joined after the batch was created.
- Jails validators who still have not submitted `MsgConfirmBatch`.
- Emits `EventValidatorJailed` with reason `MissingBatchConfirm`.
- Tracks progress with `LastJailedBatchBlock`.
- Does not apply `SlashFractionBatch`.

This prevents two bad scenarios:

1. A validator simply does not bother to keep the correct binaries running on their system,
2. A cartel of >1/3 validators refuses to sign updates, preventing batches from getting enough signatures to be submitted to the Peggy Ethereum contract.

If a batch has already executed on Ethereum and been removed from Peggy state before the jailing pass runs, Peggy no longer jails validators for missing that batch confirmation.

## PEGGYSLASH-03: Failure to sign validator set update (implemented as jailing)

This penalty path is triggered when a validator does not confirm a validator set update within `SignedValsetsWindow` after the Peggy module creates it.

The current EndBlocker logic:

- Only checks valsets that are old enough to have passed the configured window.
- Checks bonded validators that either were already active when the valset was created or were members of that valset.
- Also checks unbonding validators for up to `UnbondSlashingValsetsWindow` blocks after the relevant valset height.
- Jails validators who still have not submitted `MsgValsetConfirm`.
- Emits `EventValidatorJailed` with reason `MissingValsetConfirm`.
- Tracks progress with `LastJailedValsetNonce`.
- Does not apply `SlashFractionValset`.

This prevents two bad scenarios:

1. A validator simply does not bother to keep the correct binaries running on their system,
2. A cartel of >1/3 validators unbonds and then refuses to sign updates, preventing validator set updates from getting enough signatures to be submitted to the Peggy Ethereum contract. If they prevent validator set updates for longer than the Injective Chain unbonding period, they can no longer be punished for fake-signature evidence on the bridge side.

To deal with scenario 2, Peggy also checks validators who are no longer bonded but are still in the unbonding period for up to `UnbondSlashingValsetsWindow` blocks. In practice, that means a validator leaving the set still needs to keep signing long enough to confirm a valset that excludes them.

The current value of `UnbondSlashingValsetsWindow` is 25,000 blocks, or about 12-14 hours. We have determined this to be a safe value based on the following logic. So long as every validator leaving the validator set signs at least one validator set update that they are not contained in then it is guaranteed to be possible for a relayer to produce a chain of validator set updates to transform the current state on the chain into the present state.

It should be noted that both PEGGYSLASH-02 and PEGGYSLASH-03 could be eliminated with no loss of security if it were possible to perform the Ethereum signatures inside the consensus code. This would make Peggy far less dependent on jailing validators for missed bridge confirmations.

## PEGGYSLASH-04: Submitting incorrect Eth oracle claim (Disabled for now)

The Ethereum oracle code is a key part of Peggy. It allows the Peggy module to learn about events that have occurred on Ethereum, such as deposits and executed batches. This historical penalty path is intended to punish validators who submit a claim for an event that never happened on Ethereum.

In the current implementation this path is disabled. `ClaimSlashingEnabled` exists in `Params`, but EndBlocker does not currently execute a claim-penalty routine.

**Implementation considerations**

The only way we know whether an event has happened on Ethereum is through the Ethereum event oracle itself. So if this penalty path were enabled, it would need to act on validators who submitted a different claim at the same nonce as an event that was observed by >2/3 of validators.

Although well-intentioned, this penalty path is likely not advisable for most applications of Peggy. This is because it ties the functioning of the Injective Chain to the correct functioning of the Ethereum chain. If there is a serious fork of the Ethereum chain, different validators behaving honestly may see different events at the same event nonce and be penalized through no fault of their own.

Maybe PEGGYSLASH-04 is not necessary at all:

The real utility of this condition is to make it so that, if >2/3 of the validators form a cartel to all submit a fake event at a certain nonce, some number of them can defect from the cartel and submit the real event at that nonce. If there are enough defecting cartel members that the real event becomes observed, then the remaining cartel members could in principle be penalized. However, this would require >1/2 of the cartel members to defect in most conditions.

If not enough of the cartel defects, then neither event will be observed, and the Ethereum oracle will just halt. This is a much more likely scenario than one in which PEGGYSLASH-04 is actually triggered.

Also, PEGGYSLASH-04 could be triggered against the honest validators in the case of a successful cartel. This could act to make it easier for a forming cartel to threaten validators who do not want to join.

## PEGGYSLASH-05: Failure to submit Eth oracle claims (Disabled for now)

This is similar to PEGGYSLASH-04, but it is triggered against validators who do not submit an oracle claim that has been observed. In contrast to PEGGYSLASH-04, PEGGYSLASH-05 is intended to punish validators who stop participating in the oracle completely.

In the current implementation this path is also disabled. `ClaimSlashingEnabled` is present in params, but the corresponding EndBlocker path is not executed.

**Implementation considerations**

Unfortunately, PEGGYSLASH-05 has the same downsides as PEGGYSLASH-04 in that it ties the correct operation of the Injective Chain to the Ethereum chain. Also, it likely does not incentivize much in the way of correct behavior. To avoid triggering PEGGYSLASH-05, a validator simply needs to copy claims which are close to becoming observed. This copying of claims could be prevented by a commit-reveal scheme, but it would still be easy for a "lazy validator" to simply use a public Ethereum full node or block explorer, with similar effects on security. Therefore, the real usefulness of PEGGYSLASH-05 is likely minimal

PEGGYSLASH-05 also introduces significant risks. Mostly around forks on the Ethereum chain. For example recently OpenEthereum failed to properly handle the Berlin hardfork, the resulting node 'failure' was totally undetectable to automated tools. It didn't crash so there was no restart to perform, blocks where still being produced although extremely slowly. If this had occurred while Peggy was running with PEGGYSLASH-05 active it would have caused those validators to be removed from the set. Possibly resulting in a very chaotic moment for the chain as dozens of validators where removed for little to no fault of their own.

Without PEGGYSLASH-04 and PEGGYSLASH-05, the Ethereum event oracle only continues to function if >2/3 of the validators voluntarily submit correct claims. Although the arguments against PEGGYSLASH-04 and PEGGYSLASH-05 are convincing, we must decide whether we are comfortable with this fact. Alternatively we must be comfortable with the Injective Chain potentially halting entirely due to Ethereum generated factors.
