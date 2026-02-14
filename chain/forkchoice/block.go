package forkchoice

import (
	"fmt"
	"time"

	"github.com/geanlabs/gean/chain/statetransition"
	"github.com/geanlabs/gean/observability/metrics"
	"github.com/geanlabs/gean/types"
)

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

	if _, ok := c.Storage.GetBlock(blockHash); ok {
		return nil // already known
	}

	parentState, ok := c.Storage.GetState(block.ParentRoot)
	if !ok {
		return fmt.Errorf("parent state not found for %x", block.ParentRoot)
	}

	state, err := statetransition.StateTransition(parentState, block)
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

	c.Storage.PutBlock(blockHash, block)
	c.Storage.PutSignedBlock(blockHash, envelope)
	c.Storage.PutState(blockHash, state)

	// Step 2: Process body attestations as on-chain votes.
	// Pair each body attestation with its signature from the envelope.
	for i, att := range block.Body.Attestations {
		sa := &types.SignedAttestation{
			Message:   att,
			Signature: envelope.Signature[i],
		}
		c.processAttestationLocked(sa, true)
	}

	// Step 3: Update head.
	c.updateHeadLocked()

	// Step 4: Process proposer attestation as gossip vote (is_from_block=false).
	if envelope.Message.ProposerAttestation != nil {
		proposerSA := &types.SignedAttestation{
			Message:   envelope.Message.ProposerAttestation,
			Signature: envelope.Signature[numBodyAtts], // always last
		}
		c.processAttestationLocked(proposerSA, false)
	}

	metrics.ForkChoiceBlockProcessingTime.Observe(time.Since(start).Seconds())
	metrics.StateTransitionTime.Observe(time.Since(start).Seconds())
	return nil
}
