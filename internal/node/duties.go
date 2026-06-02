package node

import (
	"context"
	"time"

	"github.com/geanlabs/gean/internal/attestation"
	"github.com/geanlabs/gean/internal/blockbuilder"
	"github.com/geanlabs/gean/internal/blockprocessor"
	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/metrics"
	"github.com/geanlabs/gean/internal/types"
	"github.com/geanlabs/gean/xmss"
)

// maybePropose builds and publishes a block if we're the proposer.
// Uses store.ProduceBlockWithSignatures for greedy attestation selection.
func (e *Engine) maybePropose(slot, validatorID uint64) {
	if e.Keys == nil {
		return
	}

	// Skip if head is already at this slot (another proposer's block was imported).
	if e.Store.HeadSlot() >= slot {
		return
	}

	// Sync-lag duty gate is the sole sync-state check.
	// It closes when local lag > SyncLagThreshold against a producing
	// network, and reopens automatically when the network itself stalls
	// (maxStoredBlockSlot far behind wall-clock). This handles all three
	// cases — synced, syncing-behind-a-live-network, isolated-stalling —
	// in one place.
	if e.DutyGate != nil && !e.DutyGate.Decide("block", slot, e.Store.HeadSlot(), e.Store.MaxStoredBlockSlot()) {
		metrics.IncBlocksSkippedLag()
		return
	}

	logger.Info(logger.Validator, "proposing block slot=%d validator=%d", slot, validatorID)

	// Promote pending attestations and update head immediately before
	// building, so the build reads the freshest head.
	e.Store.PromoteNewToKnown()
	e.updateHead()

	// Build block with greedy attestation selection.
	block, attSigProofs, err := blockbuilder.ProduceBlockWithSignatures(e.Store, slot, validatorID)
	if err != nil {
		logger.Error(logger.Validator, "produce block failed: %v", err)
		return
	}

	// Sign the block root with the PROPOSAL key.
	signStart := time.Now()
	propKey := e.Keys.GetProposalKey(validatorID)
	if propKey == nil {
		logger.Error(logger.Validator, "proposal key not found for validator=%d", validatorID)
		return
	}
	blockRoot, _ := block.HashTreeRoot()
	blockSig, err := propKey.Sign(uint32(slot), blockRoot)
	metrics.ObservePqSigSigningTime(time.Since(signStart).Seconds())
	if err != nil {
		logger.Error(logger.Validator, "sign block failed: %v", err)
		return
	}

	signedBlock := &types.SignedBlock{
		Block: block,
		Signature: &types.BlockSignatures{
			ProposerSignature:     blockSig,
			AttestationSignatures: attSigProofs,
		},
	}

	// Process locally first.
	if err := blockprocessor.OnBlock(e.Store, signedBlock); err != nil {
		logger.Error(logger.Chain, "local block processing failed: %v", err)
		return
	}

	// Register in fork choice.
	bRoot, _ := block.HashTreeRoot()
	e.FC.OnBlock(slot, bRoot, block.ParentRoot)
	e.updateHead()

	// Publish to network.
	if e.P2P != nil {
		if err := e.P2P.PublishBlock(context.Background(), signedBlock); err != nil {
			logger.Error(logger.Network, "publish block failed: %v", err)
		}
	}

	logger.Info(logger.Validator, "proposed block slot=%d block_root=0x%x attestations=%d",
		slot, bRoot, len(block.Body.Attestations))
}

// produceAttestations creates and publishes attestations for all local validators.
func (e *Engine) produceAttestations(slot uint64) {
	if e.Keys == nil {
		return
	}

	// Sync-lag duty gate. Skip the whole batch when the
	// local view is too stale relative to a network that is otherwise
	// making progress. Counter ticks once per skipped slot, not per validator.
	if e.DutyGate != nil && !e.DutyGate.Decide("attestation", slot, e.Store.HeadSlot(), e.Store.MaxStoredBlockSlot()) {
		metrics.IncAttestationsSkippedLag()
		return
	}

	attData := attestation.ProduceAttestationData(e.Store, slot)
	if attData == nil {
		return
	}

	for _, vid := range e.Keys.ValidatorIDs() {
		prodStart := time.Now()

		sStart := time.Now()
		sig, err := e.Keys.SignAttestation(vid, attData)
		metrics.ObservePqSigSigningTime(time.Since(sStart).Seconds())
		if err != nil {
			logger.Error(logger.Validator, "sign attestation failed validator=%d: %v", vid, err)
			continue
		}

		signedAtt := &types.SignedAttestation{
			ValidatorID: vid,
			Data:        attData,
			Signature:   sig,
		}

		logger.Info(logger.Validator, "produced attestation slot=%d validator=%d", slot, vid)

		// Self-deliver for aggregation if we are the aggregator.
		// Skip signature verification — we just signed it ourselves.
		if e.AggCtl.Get() {
			dataRoot, _ := attData.HashTreeRoot()
			sigHandle, parseErr := xmss.ParseSignature(sig[:])
			e.Store.AttestationSignatures.InsertWithHandle(dataRoot, attData, vid, sig, sigHandle, parseErr)
		}

		// Publish to subnet.
		if e.P2P != nil {
			if err := e.P2P.PublishAttestation(context.Background(), signedAtt, e.CommitteeCount); err != nil {
				logger.Error(logger.Network, "publish attestation failed validator=%d: %v", vid, err)
			} else {
				logger.Info(logger.Network, "published attestation to network slot=%d validator=%d", slot, vid)
			}
		}

		// Sign failures hit `continue` above and are not sampled — the
		// histogram only records iterations that actually produced an
		// validator. Publish failures still count as produced (the
		// attestation exists; only delivery to gossip failed).
		metrics.ObserveAttestationsProductionTime(time.Since(prodStart).Seconds())
	}
}

// getOurProposer checks if any of our validators is the proposer for this slot.
func (e *Engine) getOurProposer(slot uint64) (uint64, bool) {
	if e.Keys == nil {
		return 0, false
	}
	headState := e.Store.GetState(e.Store.Head())
	if headState == nil {
		return 0, false
	}
	numValidators := headState.NumValidators()

	for _, vid := range e.Keys.ValidatorIDs() {
		if types.IsProposer(slot, vid, numValidators) {
			return vid, true
		}
	}
	return 0, false
}
