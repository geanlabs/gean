package forkchoice

import (
	"time"

	"github.com/devylongs/gean/observability/metrics"
	"github.com/devylongs/gean/types"
)

// ProcessAttestation processes an attestation from the network.
func (c *Store) ProcessAttestation(sv *types.SignedVote) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.processAttestationLocked(sv, false)
}

func (c *Store) processAttestationLocked(sv *types.SignedVote, isFromBlock bool) {
	start := time.Now()
	vote := sv.Data
	validatorID := vote.ValidatorID

	source := "gossip"
	if isFromBlock {
		source = "block"
	}

	if !c.validateAttestationLocked(sv) {
		return
	}

	if isFromBlock {
		// On-chain: update known votes if this is newer.
		existing, ok := c.LatestKnownVotes[validatorID]
		if !ok || existing.Slot < vote.Slot {
			c.LatestKnownVotes[validatorID] = vote.Target
		}
		// Remove from new votes if superseded.
		newVote, ok := c.LatestNewVotes[validatorID]
		if ok && newVote.Slot <= vote.Target.Slot {
			delete(c.LatestNewVotes, validatorID)
		}
	} else {
		// Network gossip attestation processing.
		currentSlot := c.Time / types.IntervalsPerSlot
		if vote.Slot > currentSlot {
			return
		}

		// Network gossip: update new votes if this is newer.
		existing, ok := c.LatestNewVotes[validatorID]
		if !ok || existing.Slot < vote.Target.Slot {
			c.LatestNewVotes[validatorID] = vote.Target
		}
	}

	metrics.AttestationsValid.WithLabelValues(source).Inc()
	metrics.AttestationValidationTime.Observe(time.Since(start).Seconds())
}

// validateAttestationLocked performs leanSpec devnet0 attestation checks.
func (c *Store) validateAttestationLocked(sv *types.SignedVote) bool {
	vote := sv.Data

	sourceBlock, ok := c.Storage.GetBlock(vote.Source.Root)
	if !ok {
		return false
	}
	targetBlock, ok := c.Storage.GetBlock(vote.Target.Root)
	if !ok {
		return false
	}

	if sourceBlock.Slot > targetBlock.Slot {
		return false
	}
	if vote.Source.Slot > vote.Target.Slot {
		return false
	}
	if sourceBlock.Slot != vote.Source.Slot {
		return false
	}
	if targetBlock.Slot != vote.Target.Slot {
		return false
	}

	currentSlot := c.Time / types.IntervalsPerSlot
	if vote.Slot > currentSlot+1 {
		return false
	}

	return true
}
