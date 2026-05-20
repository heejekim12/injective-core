package peggy

import (
	"sort"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/ethereum/go-ethereum/common"

	"github.com/InjectiveLabs/injective-core/injective-chain/modules/peggy/keeper"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/peggy/types"
	chaintypes "github.com/InjectiveLabs/injective-core/injective-chain/types"
)

var (
	DefaultValsetPowerDiffThreshold = sdkmath.LegacyMustNewDecFromStr("0.05")
)

type BlockHandler struct {
	k *keeper.Keeper
}

func NewBlockHandler(k keeper.Keeper) *BlockHandler {
	return &BlockHandler{
		k: &k,
	}
}

// BeginBlocker is called at the beginning of every block.
func (h *BlockHandler) BeginBlocker(ctx sdk.Context) {
	defer h.k.Meter(ctx).FuncTiming(&ctx, "BeginBlocker")()

	h.excludeRateLimitTransfers(ctx)
}

// EndBlocker is called at the end of every block
func (h *BlockHandler) EndBlocker(ctx sdk.Context) {
	defer h.k.Meter(ctx).FuncTiming(&ctx, "EndBlocker")()

	params := h.k.GetParams(ctx)

	h.jailing(ctx, params)
	h.attestationTally(ctx)
	h.cleanupTimedOutBatches(ctx)
	h.createValsets(ctx)
	h.pruneValsets(ctx, params)
	h.pruneAttestations(ctx)
	h.includeRateLimitTransfers(ctx)
	h.cleanUpOldConfirmations(ctx)
}

func (h *BlockHandler) createValsets(ctx sdk.Context) {
	defer h.k.Meter(ctx).FuncTiming(&ctx, "BlockHandler.createValsets")()

	// Auto ValsetRequest Creation.
	// WARNING: do not use k.GetLastObservedValset in this function, it *will* result in losing control of the bridge
	// 1. If there are no valset requests, create a new one.
	// 2. If there is at least one validator who started unbonding in current block. (we persist last unbonded block height in hooks.go)
	//      This will make sure the unbonding validator has to provide an attestation to a new Valset
	//	    that excludes him before he completely Unbonds.  Otherwise he will be jailed
	// 3. If power change between validators of CurrentValset and latest valset request is > 5%

	// get the last valsets to compare against
	latestValset := h.k.GetLatestValset(ctx)
	lastUnbondingHeight := h.k.GetLastUnbondingBlockHeight(ctx)

	if (latestValset == nil) || (lastUnbondingHeight == uint64(ctx.BlockHeight())) ||
		(types.BridgeValidators(h.k.GetCurrentValset(ctx).Members).PowerDiff(latestValset.Members).GTE(DefaultValsetPowerDiffThreshold)) {
		// if the conditions are true, put in a new validator set request to be signed and submitted to Ethereum
		h.k.SetValsetRequest(ctx)
	}
}

// Iterate over all attestations currently being voted on in order of nonce
// and prune those that are older than the current nonce and no longer have any
// use. This could be combined with create attestation and save some computation
// but (A) pruning keeps the iteration small in the first place and (B) there is
// already enough nuance in the other handler that it's best not to complicate it further
func (h *BlockHandler) pruneAttestations(ctx sdk.Context) {
	defer h.k.Meter(ctx).FuncTiming(&ctx, "BlockHandler.pruneAttestations")()

	attmap := h.k.GetAttestationMapping(ctx)

	// We make a slice with all the event nonces that are in the attestation mapping
	keys := make([]uint64, 0, len(attmap))
	for k := range attmap {
		keys = append(keys, k)
	}
	// Then we sort it
	sort.SliceStable(keys, func(i, j int) bool { return keys[i] < keys[j] })

	lastObservedEventNonce := h.k.GetLastObservedEventNonce(ctx)
	// This iterates over all keys (event nonces) in the attestation mapping. Each value contains
	// a slice with one or more attestations at that event nonce. There can be multiple attestations
	// at one event nonce when validators disagree about what event happened at that nonce.
	for _, nonce := range keys {
		// This iterates over all attestations at a particular event nonce.
		// They are ordered by when the first attestation at the event nonce was received.
		// This order is not important.
		for _, att := range attmap[nonce] {
			// we delete all attestations earlier than the current event nonce
			if nonce < lastObservedEventNonce {
				h.k.DeleteAttestation(ctx, att)
			}
		}
	}
}

func (h *BlockHandler) jailing(ctx sdk.Context, params *types.Params) {
	defer h.k.Meter(ctx).FuncTiming(&ctx, "BlockHandler.jailing")()

	h.valsetJailing(ctx, params)
	h.batchJailing(ctx, params)

	// See https://github.com/Gravity-Bridge/Gravity-Bridge/blob/main/spec/slashing-spec.md#gravslash-05-failure-to-submit-eth-oracle-claims---intentionally-not-implemented
	// if params.ClaimSlashingEnabled {
	//	h.claimsSlashing(ctx, params)
	//}
}

// Iterate over all attestations currently being voted on in order of nonce and
// "Observe" those who have passed the threshold. Break the loop once we see
// an attestation that has not passed the threshold
func (h *BlockHandler) attestationTally(ctx sdk.Context) {
	defer h.k.Meter(ctx).FuncTiming(&ctx, "BlockHandler.attestationTally")()

	attmap := h.k.GetAttestationMapping(ctx)
	// We make a slice with all the event nonces that are in the attestation mapping
	keys := make([]uint64, 0, len(attmap))
	for k := range attmap {
		keys = append(keys, k)
	}
	// Then we sort it
	sort.SliceStable(keys, func(i, j int) bool { return keys[i] < keys[j] })

	// This iterates over all keys (event nonces) in the attestation mapping. Each value contains
	// a slice with one or more attestations at that event nonce. There can be multiple attestations
	// at one event nonce when validators disagree about what event happened at that nonce.
	for _, nonce := range keys {
		// This iterates over all attestations at a particular event nonce.
		// They are ordered by when the first attestation at the event nonce was received.
		// This order is not important.
		for _, attestation := range attmap[nonce] {
			// We check if the event nonce is exactly 1 higher than the last attestation that was
			// observed. If it is not, we just move on to the next nonce. This will skip over all
			// attestations that have already been observed.
			//
			// Once we hit an event nonce that is one higher than the last observed event, we stop
			// skipping over this conditional and start calling tryAttestation (counting votes)
			// Once an attestation at a given event nonce has enough votes and becomes observed,
			// every other attestation at that nonce will be skipped, since the lastObservedEventNonce
			// will be incremented.
			//
			// Then we go to the next event nonce in the attestation mapping, if there is one. This
			// nonce will once again be one higher than the lastObservedEventNonce.
			// If there is an attestation at this event nonce which has enough votes to be observed,
			// we skip the other attestations and move on to the next nonce again.
			// If no attestation becomes observed, when we get to the next nonce, every attestation in
			// it will be skipped. The same will happen for every nonce after that.
			if nonce == h.k.GetLastObservedEventNonce(ctx)+1 {
				h.k.TryAttestation(ctx, attestation)
			}
		}
	}
}

// cleanupTimedOutBatches deletes batches that have passed their expiration on Ethereum
// keep in mind several things when modifying this function
// A) unlike nonces timeouts are not monotonically increasing, meaning batch 5 can have a later timeout than batch 6
//
//	this means that we MUST only cleanup a single batch at a time
//
// B) it is possible for ethereumHeight to be zero if no events have ever occurred, make sure your code accounts for this
// C) When we compute the timeout we do our best to estimate the Ethereum block height at that very second. But what we work with
//
//	here is the Ethereum block height at the time of the last Deposit or Withdraw to be observed. It's very important we do not
//	project, if we do a slowdown on ethereum could cause a double spend. Instead timeouts will *only* occur after the timeout period
//	AND any deposit or withdraw has occurred to update the Ethereum block height.
func (h *BlockHandler) cleanupTimedOutBatches(ctx sdk.Context) {
	defer h.k.Meter(ctx).FuncTiming(&ctx, "BlockHandler.cleanupTimedOutBatches")()

	ethereumHeight := h.k.GetLastObservedEthereumBlockHeight(ctx).EthereumBlockHeight
	batches := h.k.GetOutgoingTxBatches(ctx)

	for _, batch := range batches {
		if batch.BatchTimeout < ethereumHeight {
			err := h.k.CancelOutgoingTXBatch(ctx, common.HexToAddress(batch.TokenContract), batch.BatchNonce)
			if err != nil {
				ctx.Logger().Error("failed to cancel outgoing tx batch", "error", err, "block", batch.Block, "batch_nonce", batch.BatchNonce)
			}
		}
	}
}

func (h *BlockHandler) valsetJailing(ctx sdk.Context, params *types.Params) { //nolint
	defer h.k.Meter(ctx).FuncTiming(&ctx, "BlockHandler.valsetJailing")()

	maxHeight := uint64(0)

	// don't jail in the beginning before there aren't even SignedValsetsWindow blocks yet
	if uint64(ctx.BlockHeight()) > params.SignedValsetsWindow {
		maxHeight = uint64(ctx.BlockHeight()) - params.SignedValsetsWindow
	} else {
		// we can't jail anyone if SignedValsetWindow blocks have not passed
		return
	}

	unjailedValsets := h.k.GetUnjailedValsets(ctx, maxHeight)

	wasInValset := func(addr common.Address, vs *types.Valset) bool {
		for _, member := range vs.Members {
			if addr == common.HexToAddress(member.EthereumAddress) {
				return true
			}
		}

		return false
	}

	// unjailed valsets are sorted by nonce in ASC order
	for _, vs := range unjailedValsets {
		confirms := h.k.GetValsetConfirms(ctx, vs.Nonce)

		// JAIL BONDED VALIDATORS who didn't attest valset request
		currentBondedSet, _ := h.k.StakingKeeper.GetBondedValidatorsByPower(ctx)

		for i := range currentBondedSet {
			consAddr, _ := currentBondedSet[i].GetConsAddr()
			valSigningInfo, err := h.k.SlashingKeeper.GetValidatorSigningInfo(ctx, consAddr)
			valAddr, _ := sdk.ValAddressFromBech32(currentBondedSet[i].GetOperator())
			ethAddress, exists := h.k.GetEthAddressByValidator(ctx, valAddr)

			exist := err == nil
			//  jail validator ONLY if he joined after valset is created
			if exist && (valSigningInfo.StartHeight < int64(vs.Height) || wasInValset(ethAddress, vs)) {
				// Check if validator has confirmed valset or not
				found := false
				for _, conf := range confirms {
					// This may have an issue if the validator changes their eth address
					// TODO this presents problems for delegate key rotation see issue #344
					if exists && common.HexToAddress(conf.EthAddress) == ethAddress {
						found = true
						break
					}
				}

				// jail validators for not confirming valsets
				if !found {
					cons, _ := currentBondedSet[i].GetConsAddr()
					consPower := currentBondedSet[i].ConsensusPower(h.k.StakingKeeper.PowerReduction(ctx))

					if !currentBondedSet[i].IsJailed() {
						_ = h.k.StakingKeeper.Jail(ctx, cons)
						_ = ctx.EventManager().EmitTypedEvent(&types.EventValidatorJailed{
							Reason:           types.JailReason_MissingValsetConfirm,
							Power:            consPower,
							ConsensusAddress: sdk.ConsAddress(consAddr).String(),
							OperatorAddress:  currentBondedSet[i].OperatorAddress,
							Moniker:          currentBondedSet[i].GetMoniker(),
						})
					}
				}
			}
		}

		// JAIL UNBONDING VALIDATORS who didn't attest valset request
		stakingParams, _ := h.k.StakingKeeper.GetParams(ctx)
		blockTime := ctx.BlockTime().Add(stakingParams.UnbondingTime)
		blockHeight := ctx.BlockHeight()

		// a list of lists when validators started unbonding
		unbondedValidators := make([]stakingtypes.ValAddresses, 0)
		unbondingValIterator, _ := h.k.StakingKeeper.ValidatorQueueIterator(ctx, blockTime, blockHeight)
		chaintypes.IterateSafe(unbondingValIterator, func(_, v []byte) (stop bool) {
			unbondedValidators = append(unbondedValidators, h.k.DeserializeValidatorIterator(v))
			return false
		})

		// All unbonding validators
		for _, validators := range unbondedValidators {
			for _, v := range validators.Addresses {
				addr, err := sdk.ValAddressFromBech32(v)
				if err != nil {
					panic(err)
				}

				validator, _ := h.k.StakingKeeper.GetValidator(ctx, addr)
				valConsAddr, _ := validator.GetConsAddr()
				valSigningInfo, err := h.k.SlashingKeeper.GetValidatorSigningInfo(ctx, valConsAddr)
				if err != nil {
					// we do nothing but it is unusual
					h.k.Logger(ctx).Warn("missing signing info for unbonding validator", "validator", v, "err", err)
					continue
				}

				// Only slash validators who joined after valset is created and they are unbonding and UNBOND_SLASHING_WINDOW didn't passed
				wasValidatorBeforeValset := valSigningInfo.StartHeight < int64(vs.Height)
				valsetCaughtByUnbondingWindow := vs.Height < uint64(validator.UnbondingHeight)+params.UnbondSlashingValsetsWindow
				if validator.IsUnbonding() && wasValidatorBeforeValset && valsetCaughtByUnbondingWindow {
					// Check if validator has confirmed valset or not
					found := false
					for _, conf := range confirms {
						ethAddress, exists := h.k.GetEthAddressByValidator(ctx, addr)
						if exists && common.HexToAddress(conf.EthAddress) == ethAddress {
							found = true
							break
						}
					}

					// jail validators for not confirming valsets
					if !found {
						consPower := validator.ConsensusPower(h.k.StakingKeeper.PowerReduction(ctx))
						if !validator.IsJailed() {
							_ = h.k.StakingKeeper.Jail(ctx, valConsAddr)
							_ = ctx.EventManager().EmitTypedEvent(&types.EventValidatorJailed{
								Power:            consPower,
								Reason:           types.JailReason_MissingValsetConfirm,
								ConsensusAddress: validator.String(),
								OperatorAddress:  validator.OperatorAddress,
								Moniker:          validator.GetMoniker(),
							})
						}
					}
				}
			}
		}

		// then we set the latest jailed valset  nonce
		h.k.SetLastJailedValsetNonce(ctx, vs.Nonce)
	}
}

func (h *BlockHandler) batchJailing(ctx sdk.Context, params *types.Params) { //nolint
	defer h.k.Meter(ctx).FuncTiming(&ctx, "BlockHandler.batchJailing")()

	// #2 condition
	// We look through the full bonded set (not just the active set, include unbonding validators)
	// and we jail users who haven't signed a batch confirmation that is >15hrs in blocks old
	maxHeight := uint64(0)

	// don't jail in the beginning before there aren't even SignedBatchesWindow blocks yet
	if uint64(ctx.BlockHeight()) > params.SignedBatchesWindow {
		maxHeight = uint64(ctx.BlockHeight()) - params.SignedBatchesWindow
	} else {
		// we can't jail anyone if this window has not yet passed
		return
	}

	unjailedBatches := h.k.GetUnjailedBatches(ctx, maxHeight)
	for _, batch := range unjailedBatches {
		// JAIL BONDED VALIDATORS who didn't attest batch requests
		currentBondedSet, _ := h.k.StakingKeeper.GetBondedValidatorsByPower(ctx)
		confirms := h.k.GetBatchConfirmByNonceAndTokenContract(ctx, batch.BatchNonce, common.HexToAddress(batch.TokenContract))
		for i := range currentBondedSet {
			// Don't jail validators who joined after batch is created
			consAddr, _ := currentBondedSet[i].GetConsAddr()

			valSigningInfo, err := h.k.SlashingKeeper.GetValidatorSigningInfo(ctx, consAddr)
			if exist := err == nil; exist && valSigningInfo.StartHeight > int64(batch.Block) {
				continue
			}

			found := false
			for _, batchConfirmation := range confirms {
				// TODO this presents problems for delegate key rotation see issue #344
				orchestratorAcc, _ := sdk.AccAddressFromBech32(batchConfirmation.Orchestrator)
				delegatedOperator, delegatedFound := h.k.GetOrchestratorValidator(ctx, orchestratorAcc)
				operatorAddr, _ := sdk.ValAddressFromBech32(currentBondedSet[i].GetOperator())
				if delegatedFound && delegatedOperator.Equals(operatorAddr) {
					found = true
					break
				}
			}

			if !found {
				cons, _ := currentBondedSet[i].GetConsAddr()
				consPower := currentBondedSet[i].ConsensusPower(h.k.StakingKeeper.PowerReduction(ctx))

				if !currentBondedSet[i].IsJailed() {
					_ = h.k.StakingKeeper.Jail(ctx, cons)
					_ = ctx.EventManager().EmitTypedEvent(&types.EventValidatorJailed{
						Power:            consPower,
						Reason:           types.JailReason_MissingBatchConfirm,
						ConsensusAddress: currentBondedSet[i].String(),
						OperatorAddress:  currentBondedSet[i].OperatorAddress,
						Moniker:          currentBondedSet[i].GetMoniker(),
					})
				}
			}
		}

		// then we set the latest jailed batch block
		h.k.SetLastJailedBatchBlock(ctx, batch.Block)
	}
}

func (h *BlockHandler) pruneValsets(ctx sdk.Context, params *types.Params) {
	defer h.k.Meter(ctx).FuncTiming(&ctx, "BlockHandler.pruneValsets")()

	// Validator set pruning
	// prune all validator sets with a nonce less than the
	// last observed nonce, they can't be submitted any longer
	//
	// Only prune valsets after the signed valsets window has passed
	// so that jailing can occur the block before we remove them
	lastObserved := h.k.GetLastObservedValset(ctx)
	currentBlock := uint64(ctx.BlockHeight())
	tooEarly := currentBlock < params.SignedValsetsWindow
	if lastObserved != nil && !tooEarly {
		earliestToPrune := currentBlock - params.SignedValsetsWindow
		sets := h.k.GetValsets(ctx)

		for _, set := range sets {
			if set.Nonce < lastObserved.Nonce && set.Height < earliestToPrune {
				h.k.DeleteValset(ctx, set.Nonce)
			}
		}
	}
}

func (h *BlockHandler) excludeRateLimitTransfers(ctx sdk.Context) {
	defer h.k.Meter(ctx).FuncTiming(&ctx, "BlockHandler.excludeRateLimitTransfers")()

	currentBlock := uint64(ctx.BlockHeight())
	for _, rateLimit := range h.k.GetRateLimits(ctx) {
		if currentBlock <= rateLimit.RateLimitWindow {
			continue
		}

		var (
			endBlock           = currentBlock - rateLimit.RateLimitWindow + 1
			tokenAddress       = common.HexToAddress(rateLimit.TokenAddress)
			outdatedOutflow    = h.k.GetOutdatedOutflow(ctx, tokenAddress, endBlock)
			outdatedInflow     = h.k.GetOutdatedInflow(ctx, tokenAddress, endBlock)
			netOutflowToRemove = outdatedOutflow.Sub(outdatedInflow)
			totalNetOutflow    = h.k.GetNetOutflow(ctx, tokenAddress)
			newNetOutflow      = totalNetOutflow.Sub(netOutflowToRemove)
		)

		h.k.SetNetOutflow(ctx, tokenAddress, newNetOutflow)
		h.k.PruneOldInflows(ctx, tokenAddress, endBlock)
		h.k.PruneOldOutflows(ctx, tokenAddress, endBlock)
	}

	// In case of a removed rate limit there might still be records of inflow/outflow.
	// This pruning is passive by design to avoid excessive computation during RemoveRateLimit.
	for _, token := range h.k.GetNonExistentRateLimitTokenAddresses(ctx) {
		h.k.PruneInflowForNonExistentRateLimitToken(ctx, token)
		h.k.PruneOutflowForNonExistingRateLimitToken(ctx, token)
	}
}

func (h *BlockHandler) includeRateLimitTransfers(ctx sdk.Context) {
	defer h.k.Meter(ctx).FuncTiming(&ctx, "BlockHandler.includeRateLimitTransfers")()

	for _, rateLimit := range h.k.GetRateLimits(ctx) {
		var (
			tokenAddress    = common.HexToAddress(rateLimit.TokenAddress)
			height          = uint64(ctx.BlockHeight())
			inflow          = h.k.GetInflowByBlock(ctx, tokenAddress, height)
			outflow         = h.k.GetOutflowByBlock(ctx, tokenAddress, height)
			netOutflowToAdd = outflow.Sub(inflow)
			totalNetOutflow = h.k.GetNetOutflow(ctx, tokenAddress)
			newNetOutflow   = totalNetOutflow.Add(netOutflowToAdd)
		)

		h.k.SetNetOutflow(ctx, tokenAddress, newNetOutflow)
	}
}

func (h *BlockHandler) cleanUpOldConfirmations(ctx sdk.Context) {
	defer h.k.Meter(ctx).FuncTiming(&ctx, "BlockHandler.cleanUpOldConfirmations")()

	h.k.PruneOldValsetConfirms(ctx)
	h.k.PruneOldBatchConfirms(ctx)
}
