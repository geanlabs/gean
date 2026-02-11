// ticker.go contains slot/interval driving and local proposal/attestation production.
package node

import (
	"time"

	"github.com/devylongs/gean/types"
)

func (n *Node) slotTicker() {
	defer n.wg.Done()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-n.ctx.Done():
			return
		case <-ticker.C:
			n.onTick()
		}
	}
}

// onTick is called every second to drive the slot pipeline.
//
// At each slot:
//   - Interval 0: proposer produces a block (if assigned)
//   - Interval 1: non-proposers produce attestations
func (n *Node) onTick() {
	currentTime := uint64(time.Now().Unix())

	// Don't do anything before genesis
	if currentTime < n.config.GenesisTime {
		return
	}

	n.store.AdvanceTime(currentTime, false)

	slot := n.store.CurrentSlot()
	interval := n.store.CurrentInterval()

	// Log slot progression at start of each slot
	if interval == 0 {
		finalized := n.store.GetLatestFinalized()
		n.logger.Debug("slot",
			"slot", slot,
			"head", n.store.GetHead().Short(),
			"justified", n.store.GetLatestJustified().Slot,
			"finalized", finalized.Slot,
			"peers", n.PeerCount(),
		)
	}

	// Interval 0: Proposer produces block (skip slot 0 - that's genesis)
	if interval == 0 && slot > 0 {
		if slot <= n.lastProposedSlot {
			return
		}
		proposerIndex := uint64(slot) % n.config.ValidatorCount
		if proposerIndex == n.config.ValidatorIndex {
			n.lastProposedSlot = slot
			n.proposeBlock(slot)
		}
	}

	// Interval 1: Validators attest (skip slot 0 - no block to attest on yet)
	if interval == 1 && slot > 0 {
		// Proposer already includes and processes its attestation at interval 0.
		proposerIndex := uint64(slot) % n.config.ValidatorCount
		if proposerIndex == n.config.ValidatorIndex {
			return
		}
		n.produceAttestation(slot)
	}
}

// proposeBlock produces and publishes a block for the given slot.
// Uses the iterative attestation collection algorithm (see forkchoice.Store.ProduceBlock).
func (n *Node) proposeBlock(slot types.Slot) {
	validatorIndex := types.ValidatorIndex(n.config.ValidatorIndex)

	// ProduceBlock iteratively collects attestations and computes state root
	block, err := n.store.ProduceBlock(slot, validatorIndex)
	if err != nil {
		n.logger.Warn("produce block failed", "slot", slot, "error", err)
		return
	}

	// Build proposer attestation data
	attData := n.store.ProduceAttestationData(slot)
	proposerAtt := types.Attestation{
		ValidatorID: n.config.ValidatorIndex,
		Data:        *attData,
	}

	// Create signed block envelope.
	// TODO: attach XMSS signatures once key management is implemented.
	signedBlock := &types.SignedBlockWithAttestation{
		Message: types.BlockWithAttestation{
			Block:               *block,
			ProposerAttestation: proposerAtt,
		},
		// Signature list is empty until XMSS signing is wired.
	}

	// Process proposer attestation locally as pending gossip-stage vote.
	// Proposers skip interval-1 attestation to avoid double-attesting.
	proposerSigned := &types.SignedAttestation{Message: proposerAtt}
	if err := n.store.ProcessAttestation(proposerSigned); err != nil {
		n.logger.Warn("failed to process own proposer attestation",
			"slot", slot,
			"validator", proposerSigned.Message.ValidatorID,
			"error", err,
		)
	}

	if err := n.net.PublishBlock(n.ctx, signedBlock); err != nil {
		n.logger.Error("failed to publish block", "slot", slot, "error", err)
		return
	}

	n.logger.Info("proposed block", "slot", slot, "attestations", len(block.Body.Attestations))
}

// produceAttestation creates and publishes an attestation, then processes it locally.
func (n *Node) produceAttestation(slot types.Slot) {
	validatorIndex := types.ValidatorIndex(n.config.ValidatorIndex)

	// Produce attestation data
	attData := n.store.ProduceAttestationData(slot)

	att := &types.SignedAttestation{
		Message: types.Attestation{
			ValidatorID: uint64(validatorIndex),
			Data:        *attData,
		},
		// Signature is zero until XMSS signing is wired.
	}

	if err := n.net.PublishAttestation(n.ctx, att); err != nil {
		n.logger.Error("failed to publish attestation", "slot", slot, "error", err)
		return
	}

	// Process our own attestation
	if err := n.store.ProcessAttestation(att); err != nil {
		n.logger.Error("failed to process own attestation", "slot", slot, "error", err)
		return
	}

	n.logger.Debug("produced attestation", "slot", slot)
}
