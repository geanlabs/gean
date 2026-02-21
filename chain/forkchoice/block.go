package forkchoice

import (
	"fmt"
	"time"

	"github.com/geanlabs/gean/chain/statetransition"
	"github.com/geanlabs/gean/observability/metrics"
	"github.com/geanlabs/gean/types"
	"github.com/geanlabs/gean/xmss/leansig"
)

func (c *Store) verifyAttestationSignatureWithState(state *types.State, att *types.Attestation, sig [3112]byte) error {
	valID := att.ValidatorID
	if valID >= uint64(len(state.Validators)) {
		return fmt.Errorf("invalid validator index %d", valID)
	}
	pubkey := state.Validators[valID].Pubkey

	messageRoot, err := att.HashTreeRoot()
	if err != nil {
		return fmt.Errorf("failed to hash attestation message: %w", err)
	}

	signingSlot := uint32(att.Data.Slot)

	if err := leansig.Verify(pubkey[:], signingSlot, messageRoot, sig[:]); err != nil {
		log.Warn("attestation signature invalid", "slot", att.Data.Slot, "validator", valID, "err", err)
		return fmt.Errorf("signature verification failed: %w", err)
	}
	log.Info("attestation signature verified (XMSS)", "slot", att.Data.Slot, "validator", valID, "sig_size", fmt.Sprintf("%d bytes", len(sig)))
	return nil
}

// ProcessBlock processes a new signed block envelope and updates chain state.
// Attestation processing follows leanSpec on_block ordering:
//  1. State transition on the bare block.
//  2. Process body attestations as on-chain votes (is_from_block=true).
//  3. Update head.
//  4. Process proposer attestation as gossip vote (is_from_block=false).
func (c *Store) ProcessBlock(envelope *types.SignedBlockWithAttestation) error {
	start := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()

	block := envelope.Message.Block
	blockHash, _ := block.HashTreeRoot()

	if _, ok := c.storage.GetBlock(blockHash); ok {
		return nil // already known
	}

	parentState, ok := c.storage.GetState(block.ParentRoot)
	if !ok {
		return fmt.Errorf("parent state not found for %x", block.ParentRoot)
	}

	stStart := time.Now()
	state, err := statetransition.StateTransition(parentState, block)
	metrics.StateTransitionTime.Observe(time.Since(stStart).Seconds())
	if err != nil {
		return fmt.Errorf("state_transition: %w", err)
	}

	// Validate signature list shape.
	numBodyAtts := len(block.Body.Attestations)
	if envelope.Message.ProposerAttestation != nil {
		// With proposer attestation: exactly len(body_attestations) + 1 signatures.
		if len(envelope.Signature) != numBodyAtts+1 {
			return fmt.Errorf("signature count mismatch: got %d, want %d (body=%d + proposer=1)",
				len(envelope.Signature), numBodyAtts+1, numBodyAtts)
		}
	} else {
		// Without proposer attestation: exactly len(body_attestations) signatures.
		if len(envelope.Signature) != numBodyAtts {
			return fmt.Errorf("signature count mismatch: got %d, want %d (body=%d, no proposer)",
				len(envelope.Signature), numBodyAtts, numBodyAtts)
		}
	}

	c.storage.PutState(blockHash, state)

	// Step 1b: Verify signatures (skipped when skip_sig_verify build tag is set).
	if c.shouldVerifySignatures() {
		// Verify Body Attestations.
		for i, att := range block.Body.Attestations {
			// Use parent state to get validator keys (static validators).
			if err := c.verifyAttestationSignatureWithState(parentState, att, envelope.Signature[i]); err != nil {
				return fmt.Errorf("invalid body attestation signature at index %d: %w", i, err)
			}
		}

		// Verify proposer attestation signature (only when a proposer attestation is present).
		if envelope.Message.ProposerAttestation != nil {
			proposerSig := envelope.Signature[numBodyAtts] // Last signature
			if err := c.verifyAttestationSignatureWithState(parentState, envelope.Message.ProposerAttestation, proposerSig); err != nil {
				return fmt.Errorf("invalid proposer attestation signature: %w", err)
			}
		}
	}

	c.storage.PutBlock(blockHash, block)
	c.storage.PutSignedBlock(blockHash, envelope)
	c.storage.PutState(blockHash, state)

	// Update justified checkpoint from this block's post-state (monotonic).
	if state.LatestJustified.Slot > c.latestJustified.Slot {
		c.latestJustified = state.LatestJustified
	}
	// Update finalized checkpoint from this block's post-state (monotonic).
	if state.LatestFinalized.Slot > c.latestFinalized.Slot {
		c.latestFinalized = state.LatestFinalized
	}

	// Step 2: Process body attestations as on-chain votes.
	// Pair each body attestation with its signature from the envelope.
	for i, att := range block.Body.Attestations {
		sa := &types.SignedAttestation{
			ValidatorID: att.ValidatorID,
			Message:     att.Data,
			Signature:   envelope.Signature[i],
		}
		c.processAttestationLocked(sa, true)
	}

	// Step 3: Update head.
	c.updateHeadLocked()

	// Step 4: Process proposer attestation as gossip vote (is_from_block=false).
	if envelope.Message.ProposerAttestation != nil {
		proposerAtt := envelope.Message.ProposerAttestation
		proposerSA := &types.SignedAttestation{
			ValidatorID: proposerAtt.ValidatorID,
			Message:     proposerAtt.Data,
			Signature:   envelope.Signature[numBodyAtts], // always last
		}
		c.processAttestationLocked(proposerSA, false)
	}

	metrics.ForkChoiceBlockProcessingTime.Observe(time.Since(start).Seconds())
	return nil
}
