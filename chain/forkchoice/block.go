package forkchoice

import (
	"fmt"
	"time"

	"github.com/geanlabs/gean/chain/statetransition"
	"github.com/geanlabs/gean/observability/metrics"
	"github.com/geanlabs/gean/types"
	"github.com/geanlabs/gean/xmss/leanmultisig"
	"github.com/geanlabs/gean/xmss/leansig"
)

func (c *Store) verifyAttestationSignatureWithState(
	state *types.State,
	validatorID uint64,
	data *types.AttestationData,
	sig [3112]byte,
) error {
	if data == nil {
		return fmt.Errorf("attestation data is nil")
	}
	valID := validatorID
	if valID >= uint64(len(state.Validators)) {
		return fmt.Errorf("invalid validator index %d", valID)
	}
	pubkey := state.Validators[valID].Pubkey

	messageRoot, err := data.HashTreeRoot()
	if err != nil {
		return fmt.Errorf("failed to hash attestation message: %w", err)
	}

	signingSlot := uint32(data.Slot)

	verifyStart := time.Now()
	if err := leansig.Verify(pubkey[:], signingSlot, messageRoot, sig[:]); err != nil {
		metrics.PQSigAttestationVerificationTime.Observe(time.Since(verifyStart).Seconds())
		log.Warn("attestation signature invalid", "slot", data.Slot, "validator", valID, "err", err)
		return fmt.Errorf("signature verification failed: %w", err)
	}
	metrics.PQSigAttestationVerificationTime.Observe(time.Since(verifyStart).Seconds())
	log.Info("attestation signature verified (XMSS)", "slot", data.Slot, "validator", valID, "sig_size", fmt.Sprintf("%d bytes", len(sig)))
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

	if c.NowFn != nil {
		c.advanceTimeLockedMillis(c.NowFn(), false)
	}

	if envelope == nil || envelope.Message == nil || envelope.Message.Block == nil {
		return fmt.Errorf("invalid block envelope")
	}
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

	// Validate signature container shape.
	numBodyAtts := len(block.Body.Attestations)
	if len(envelope.Signature.AttestationSignatures) != numBodyAtts {
		return fmt.Errorf(
			"attestation signature proof count mismatch: got %d, want %d",
			len(envelope.Signature.AttestationSignatures),
			numBodyAtts,
		)
	}
	if envelope.Message.ProposerAttestation == nil || envelope.Message.ProposerAttestation.Data == nil {
		return fmt.Errorf("missing proposer attestation")
	}

	// Step 1b: Verify signatures (skipped when skip_sig_verify build tag is set).
	if c.shouldVerifySignatures() {
		leanmultisig.SetupVerifier()

		// Verify aggregated body attestations and their matching proofs.
		for i, aggregated := range block.Body.Attestations {
			if aggregated == nil || aggregated.Data == nil {
				return fmt.Errorf("invalid body attestation at index %d", i)
			}
			proof := envelope.Signature.AttestationSignatures[i]
			if proof == nil {
				return fmt.Errorf("missing attestation signature proof at index %d", i)
			}
			if !bitlistsEqual(aggregated.AggregationBits, proof.Participants) {
				return fmt.Errorf("participants mismatch for attestation %d", i)
			}

			validatorIDs := bitlistToValidatorIDs(aggregated.AggregationBits)
			if len(validatorIDs) == 0 {
				return fmt.Errorf("empty aggregated attestation participants at index %d", i)
			}

			pubkeys := make([][]byte, 0, len(validatorIDs))
			for _, validatorID := range validatorIDs {
				if validatorID >= uint64(len(parentState.Validators)) {
					return fmt.Errorf("invalid participant index %d at attestation %d", validatorID, i)
				}
				pubkey := parentState.Validators[validatorID].Pubkey
				pubkeys = append(pubkeys, pubkey[:])
			}

			messageRoot, err := aggregated.Data.HashTreeRoot()
			if err != nil {
				return fmt.Errorf("hash aggregated attestation data %d: %w", i, err)
			}
			verifyStart := time.Now()
			if err := leanmultisig.VerifyAggregated(pubkeys, messageRoot, proof.ProofData, uint32(aggregated.Data.Slot)); err != nil {
				metrics.PQSigAggregatedVerificationTime.Observe(time.Since(verifyStart).Seconds())
				metrics.PQSigAggregatedInvalidTotal.Inc()
				return fmt.Errorf("verify aggregated proof %d: %w", i, err)
			}
			metrics.PQSigAggregatedVerificationTime.Observe(time.Since(verifyStart).Seconds())
			metrics.PQSigAggregatedValidTotal.Inc()
			log.Info(
				"attestation aggregate proof verified (leanMultisig)",
				"slot", aggregated.Data.Slot,
				"participants", len(validatorIDs),
				"proof_size", fmt.Sprintf("%d bytes", len(proof.ProofData)),
			)
		}

		// Verify proposer signature (always individual XMSS).
		proposerAtt := envelope.Message.ProposerAttestation
		if err := c.verifyAttestationSignatureWithState(
			parentState,
			proposerAtt.ValidatorID,
			proposerAtt.Data,
			envelope.Signature.ProposerSignature,
		); err != nil {
			return fmt.Errorf("invalid proposer attestation signature: %w", err)
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
	for i, aggregated := range block.Body.Attestations {
		if aggregated == nil || aggregated.Data == nil {
			continue
		}
		proof := envelope.Signature.AttestationSignatures[i]
		for _, validatorID := range bitlistToValidatorIDs(aggregated.AggregationBits) {
			sa := &types.SignedAttestation{
				ValidatorID: validatorID,
				Message:     aggregated.Data,
			}
			c.processAttestationLocked(sa, true)
			c.storeAggregatedPayloadLocked(validatorID, aggregated.Data, proof)
		}
	}

	// Step 3: Update head.
	c.updateHeadLocked()

	// Step 4: Process proposer attestation as gossip vote (is_from_block=false).
	proposerSA := &types.SignedAttestation{
		ValidatorID: envelope.Message.ProposerAttestation.ValidatorID,
		Message:     envelope.Message.ProposerAttestation.Data,
		Signature:   envelope.Signature.ProposerSignature,
	}
	c.processAttestationLocked(proposerSA, false)

	metrics.ForkChoiceBlockProcessingTime.Observe(time.Since(start).Seconds())
	return nil
}
