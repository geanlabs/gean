package node

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/geanlabs/gean/internal/attestationproof"
	"github.com/geanlabs/gean/internal/blockbuilder"
	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/metrics"
	"github.com/geanlabs/gean/internal/statetransition"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
	"github.com/geanlabs/gean/xmss"
)

// A proposal aims to be published before interval 1, when the slot's
// validators attest to the head they have. Its block proof costs about
// proposalBaseCost for the proposer's signature alone, plus proposalProofCost
// for each attestation proof merged with it (measured on leanVM 48a90420: 0.41,
// 1.42, 2.66 and 3.55 s for 0-3 proofs). Started at the previous slot's
// interval 4, that leaves room for one proof.
const (
	proposalBaseCost  = 450 * time.Millisecond
	proposalProofCost = 1100 * time.Millisecond
)

type proposalDuty struct {
	slot        uint64
	validatorID uint64
}

type proposalResult struct {
	duty        proposalDuty
	retryable   bool // Only failures before a signing attempt may release the duty.
	blockRoot   [32]byte
	signedBlock *types.SignedBlock
}

// maybeProposeEarly starts the next slot's proposal during interval 4 of the
// current slot, after the interval-4 promotion, so its proof can finish by
// interval 1 of its own slot. It waits until this slot's block is the head: a
// proposal is signed once, so building on a parent that a late block is about
// to replace would cost the slot. Otherwise the proposal starts at interval 0.
func (e *Engine) maybeProposeEarly(timestampMs uint64) {
	if e.currentInterval(timestampMs) != types.IntervalsPerSlot-1 {
		return
	}
	slot := e.currentSlot(timestampMs)
	if e.Store.HeadSlot() != slot {
		return
	}
	if validatorID, ok := e.getOurProposer(slot + 1); ok {
		e.maybePropose(slot+1, validatorID)
	}
}

// attestationBudget is how many attestation proofs a proposal for slot started
// at nowMs can carry and still be published by interval 1 of slot.
func (e *Engine) attestationBudget(slot, nowMs uint64) int {
	deadlineMs := e.slotStartMs(slot) + types.MillisecondsPerInterval
	if nowMs >= deadlineMs {
		return 0
	}
	spare := time.Duration(deadlineMs-nowMs)*time.Millisecond - proposalBaseCost
	if spare <= 0 {
		return 0
	}
	return int(spare / proposalProofCost)
}

// waitForSlotStart holds a proposal that finished early until its slot begins,
// and reports false if ctx ends first.
func (e *Engine) waitForSlotStart(ctx context.Context, slot uint64) bool {
	wait := time.Until(time.UnixMilli(int64(e.slotStartMs(slot))))
	if wait <= 0 {
		return true
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

func (e *Engine) maybePropose(slot, validatorID uint64) {
	if slot < e.lastProposalDuty.slot || (slot == e.lastProposalDuty.slot && e.proposalReserved) {
		return
	}
	if e.Keys == nil {
		return
	}
	if e.Store.HeadSlot() >= slot {
		return
	}
	if e.DutyGate != nil && !e.DutyGate.Decide("block", slot, e.Store.HeadSlot(), e.networkSeenSlot()) {
		metrics.IncBlocksSkippedLag()
		return
	}
	select {
	case e.ProposalCh <- proposalDuty{slot: slot, validatorID: validatorID}:
		e.lastProposalDuty = proposalDuty{slot: slot, validatorID: validatorID}
		e.proposalReserved = true
		metrics.SetProvingQueueDepth("proposal", len(e.ProposalCh))
	default:
		logger.Warn(logger.Validator, "proposal worker busy slot=%d", slot)
	}
}

func (e *Engine) runProposalWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case duty := <-e.ProposalCh:
			metrics.SetProvingQueueDepth("proposal", len(e.ProposalCh))
			result := e.proveProposal(ctx, duty)
			if result.signedBlock != nil && !e.waitForSlotStart(ctx, duty.slot) {
				return
			}
			select {
			case e.ProposalResultCh <- result:
			case <-ctx.Done():
				return
			}
		}
	}
}

func (e *Engine) proveProposal(ctx context.Context, duty proposalDuty) *proposalResult {
	failed := &proposalResult{duty: duty, retryable: true}
	if head := e.Store.HeadSlot(); head >= duty.slot {
		logger.Warn(logger.Validator, "skipping stale proposal slot=%d head=%d", duty.slot, head)
		metrics.IncProofOperation("proposal", "skipped_stale")
		return failed
	}
	deadline, cancel := context.WithTimeout(ctx, 3*types.MillisecondsPerInterval*time.Millisecond)
	defer cancel()
	if e.ProvingGate != nil {
		waitStart := time.Now()
		acquired := e.ProvingGate.Acquire(deadline, true)
		metrics.ObserveProposalStageDuration("gate", time.Since(waitStart).Seconds())
		if !acquired {
			metrics.IncProofOperation("proposal", "canceled")
			logger.Error(logger.Validator, "proposal prover unavailable slot=%d", duty.slot)
			return failed
		}
		defer e.ProvingGate.Release(true)
	}
	return e.buildProposal(duty.slot, duty.validatorID)
}

func (e *Engine) buildProposal(slot, validatorID uint64) *proposalResult {
	result := &proposalResult{duty: proposalDuty{slot: slot, validatorID: validatorID}, retryable: true}
	logger.Info(logger.Validator, "proposing block slot=%d validator=%d", slot, validatorID)

	budget := e.attestationBudget(slot, uint64(time.Now().UnixMilli()))
	block, attSigProofs, err := e.produceBlockWithSignatures(slot, validatorID, budget)
	if err != nil {
		// Head advanced past the target slot; skip as a slot-boundary miss (matches spec).
		var stale *statetransition.StateSlotIsNewerError
		if errors.As(err, &stale) {
			logger.Warn(logger.Validator, "skipping stale proposal slot=%d head=%d", slot, e.Store.HeadSlot())
			metrics.IncProofOperation("proposal", "skipped_stale")
			return result
		}
		logger.Error(logger.Validator, "produce block failed: %v", err)
		return result
	}

	signStart := time.Now()
	propKey := e.Keys.GetProposalKey(validatorID)
	if propKey == nil {
		logger.Error(logger.Validator, "proposal key not found for validator=%d", validatorID)
		return result
	}
	blockRoot, err := block.HashTreeRoot()
	if err != nil {
		logger.Error(logger.Validator, "block root failed: %v", err)
		return result
	}
	if e.Store.Head() != block.ParentRoot {
		metrics.IncProofOperation("proposal", "skipped_stale")
		logger.Info(logger.Validator, "skipping stale proposal before signing slot=%d", slot)
		return result
	}
	// A signing error must not permit another candidate to use this duty.
	result.retryable = false
	blockSig, err := propKey.Sign(uint32(slot), blockRoot)
	metrics.ObservePqSigSigningTime(time.Since(signStart).Seconds())
	if err != nil {
		logger.Error(logger.Validator, "sign block failed: %v", err)
		return result
	}

	mergeStart := time.Now()
	proof, err := e.mergeBlockProof(block, attSigProofs, propKey, blockSig)
	metrics.ObserveProvingDuration("proposal", time.Since(mergeStart).Seconds())
	if err != nil {
		metrics.IncProofOperation("proposal", "error")
		logger.Error(logger.Validator, "merge block proof failed: %v", err)
		return result
	}
	metrics.ObserveProofMergeComponents(len(attSigProofs) + 1)
	metrics.ObserveProofSize("type2", len(proof))
	return &proposalResult{
		duty:      result.duty,
		blockRoot: blockRoot,
		signedBlock: &types.SignedBlock{
			Block: block,
			Proof: &types.MultiMessageAggregate{Proof: proof},
		},
	}
}

func (e *Engine) acceptProposal(ctx context.Context, result *proposalResult) {
	if result != nil && result.retryable && result.duty == e.lastProposalDuty {
		e.proposalReserved = false
	}
	if result == nil || result.signedBlock == nil || result.signedBlock.Block == nil {
		return
	}
	block := result.signedBlock.Block
	// The slot's key has signed this block and cannot sign another, so publish it
	// even if a late block changed the head while it was proving: it is a valid
	// block competing for the slot, and fork choice decides. Only a head already
	// at or past the slot, or a parent whose state is gone, makes it unusable.
	if e.Store.HeadSlot() >= block.Slot || !e.Store.HasState(block.ParentRoot) {
		metrics.IncProofOperation("proposal", "canceled")
		logger.Warn(logger.Validator, "discarding proposal slot=%d: head moved head_slot=%d head=0x%x parent=0x%x",
			block.Slot, e.Store.HeadSlot(), e.Store.Head(), block.ParentRoot)
		return
	}
	e.onBlock(result.signedBlock)
	if !e.Store.HasState(result.blockRoot) {
		metrics.IncProofOperation("proposal", "error")
		return
	}
	metrics.IncProofOperation("proposal", "success")

	if e.P2P != nil {
		publishCtx, cancel := context.WithTimeout(ctx, types.MillisecondsPerInterval*time.Millisecond)
		defer cancel()
		if err := e.P2P.PublishBlock(publishCtx, result.signedBlock); err != nil {
			logger.Error(logger.Network, "publish block failed: %v", err)
		} else {
			publishedMs := float64(time.Now().UnixMilli()) - float64(e.slotStartMs(block.Slot))
			metrics.ObserveBlockPublishDelay(publishedMs / 1000)
		}
	}

	attestationCount := 0
	if block.Body != nil {
		attestationCount = len(block.Body.Attestations)
		e.reportProposalCoverage(block.Body.Attestations)
	}
	logger.Info(logger.Validator, "proposed block slot=%d block_root=0x%x attestations=%d",
		block.Slot, result.blockRoot, attestationCount)
}

func (e *Engine) mergeBlockProof(
	block *types.Block,
	attestationProofs []*types.SingleMessageAggregate,
	proposerKey *xmss.ValidatorKeyPair,
	proposerSignature [types.SignatureSize]byte,
) ([]byte, error) {
	return e.mergeBlockProofWithProver(block, attestationProofs, proposerKey, proposerSignature, xmss.MergeType1Proofs)
}

func (e *Engine) mergeBlockProofWithProver(
	block *types.Block,
	attestationProofs []*types.SingleMessageAggregate,
	proposerKey *xmss.ValidatorKeyPair,
	proposerSignature [types.SignatureSize]byte,
	merge func([]xmss.Type1Input, []xmss.RawSignature) ([]byte, error),
) ([]byte, error) {
	if block == nil || block.Body == nil || len(block.Body.Attestations) != len(attestationProofs) {
		return nil, fmt.Errorf("attestation proof count mismatch")
	}
	state := e.Store.GetState(block.ParentRoot)
	if state == nil {
		return nil, fmt.Errorf("parent state missing")
	}

	inputs := make([]xmss.Type1Input, 0, len(attestationProofs))
	for i, proof := range attestationProofs {
		if proof == nil {
			return nil, fmt.Errorf("attestation proof %d missing", i)
		}
		keys := make([]xmss.CPubKey, 0, types.BitlistCount(proof.Participants))
		for _, index := range types.BitlistIndices(proof.Participants) {
			if index >= uint64(len(state.Validators)) || state.Validators[index] == nil {
				return nil, fmt.Errorf("attestation proof %d validator %d out of range", i, index)
			}
			key, err := e.Store.PubKeyCache.Get(state.Validators[index].AttestationPubkey)
			if err != nil {
				return nil, fmt.Errorf("attestation proof %d validator %d: %w", i, index, err)
			}
			keys = append(keys, key)
		}
		data := block.Body.Attestations[i].Data
		root, err := data.HashTreeRoot()
		if err != nil {
			return nil, fmt.Errorf("attestation %d data root: %w", i, err)
		}
		inputs = append(inputs, xmss.Type1Input{Pubkeys: keys, Proof: proof.Proof, Message: root, Slot: uint32(data.Slot)})
	}

	signature, err := xmss.ParseSignature(proposerSignature[:])
	if err != nil {
		return nil, err
	}
	defer xmss.FreeSignature(signature)
	blockRoot, err := block.HashTreeRoot()
	if err != nil {
		return nil, err
	}
	// The proposer's signature goes into the merge raw. Proving it alone first and
	// merging that proof costs a whole extra proof, the most expensive part of a
	// proposal after the merge itself.
	proposer := xmss.RawSignature{
		Pubkey:    proposerKey.PublicKey(),
		Signature: signature,
		Message:   blockRoot,
		Slot:      uint32(block.Slot),
	}
	mergeStart := time.Now()
	proof, err := merge(inputs, []xmss.RawSignature{proposer})
	metrics.ObserveProposalStageDuration("merge", time.Since(mergeStart).Seconds())
	return proof, err
}

func (e *Engine) produceBlockWithSignatures(slot, validatorIndex uint64, maxAttestationData int) (*types.Block, []*types.SingleMessageAggregate, error) {
	buildStart := time.Now()
	defer func() { metrics.ObserveBlockBuildingTime(time.Since(buildStart).Seconds()) }()

	headRoot := e.Store.Head()
	headState := e.Store.GetState(headRoot)
	if headState == nil {
		metrics.IncBlockBuildingFailures()
		return nil, nil, fmt.Errorf("head state missing for slot %d", slot)
	}

	numValidators := headState.NumValidators()
	if !types.IsProposer(slot, validatorIndex, numValidators) {
		metrics.IncBlockBuildingFailures()
		return nil, nil, fmt.Errorf("validator %d not proposer for slot %d", validatorIndex, slot)
	}

	result, err := blockbuilder.Build(blockbuilder.Input{
		HeadState:          headState,
		Slot:               slot,
		ProposerIndex:      validatorIndex,
		ParentRoot:         headRoot,
		KnownBlockRoots:    blockbuilder.KnownRootsFunc(e.Store.HasBlockHeader),
		Payloads:           payloadsFromEntries(e.Store.KnownPayloads.Entries()),
		ProofMerger:        attestationproof.NewMerger(e.Store.PubKeyCache),
		MaxAttestationData: maxAttestationData,
	})
	if err != nil {
		metrics.IncBlockBuildingFailures()
		return nil, nil, err
	}
	for _, payloadErr := range result.PayloadErrors {
		if blockbuilder.IsExpectedSkip(payloadErr.Err) {
			continue
		}
		logger.Warn(logger.Validator, "block payload issue root=0x%x: %v", payloadErr.DataRoot, payloadErr.Err)
	}

	metrics.IncBlockBuildingSuccess()
	if result.Block != nil && result.Block.Body != nil {
		metrics.ObserveBlockAggregatedPayloads(len(result.Block.Body.Attestations))
	}
	return result.Block, result.AttestationProofs, nil
}

func payloadsFromEntries(entries map[[32]byte]*store.PayloadEntry) []blockbuilder.AttestationPayload {
	payloads := make([]blockbuilder.AttestationPayload, 0, len(entries))
	for dataRoot, entry := range entries {
		payload := blockbuilder.AttestationPayload{DataRoot: dataRoot}
		if entry != nil {
			payload.Data = entry.Data
			payload.Proofs = make([]*types.SingleMessageAggregate, len(entry.Proofs))
			copy(payload.Proofs, entry.Proofs)
		}
		payloads = append(payloads, payload)
	}
	return payloads
}
