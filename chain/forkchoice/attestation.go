package forkchoice

import (
	"time"

	"github.com/geanlabs/gean/observability/metrics"
	"github.com/geanlabs/gean/types"
)

// ProcessAttestation processes an attestation from the network.
func (c *Store) ProcessAttestation(sa *types.SignedAttestation) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.processAttestationLocked(sa.Message, false)
}

func (c *Store) processAttestationLocked(att *types.Attestation, isFromBlock bool) {
	start := time.Now()
	data := att.Data
	validatorID := att.ValidatorID

	source := "gossip"
	if isFromBlock {
		source = "block"
	}

	if !c.validateAttestationLocked(att) {
		return
	}

	if isFromBlock {
		// On-chain: update known votes if this is newer.
		existing, ok := c.LatestKnownVotes[validatorID]
		if !ok || existing.Slot < data.Slot {
			c.LatestKnownVotes[validatorID] = data.Target
		}
		// Remove from new votes if superseded.
		newVote, ok := c.LatestNewVotes[validatorID]
		if ok && newVote.Slot <= data.Target.Slot {
			delete(c.LatestNewVotes, validatorID)
		}
	} else {
		// Network gossip attestation processing.
		currentSlot := c.Time / types.IntervalsPerSlot
		if data.Slot > currentSlot {
			return
		}

		// Network gossip: update new votes if this is newer.
		existing, ok := c.LatestNewVotes[validatorID]
		if !ok || existing.Slot < data.Target.Slot {
			c.LatestNewVotes[validatorID] = data.Target
		}
	}

	metrics.AttestationsValid.WithLabelValues(source).Inc()
	metrics.AttestationValidationTime.Observe(time.Since(start).Seconds())
}

// validateAttestationLocked performs leanSpec devnet0 attestation checks.
func (c *Store) validateAttestationLocked(att *types.Attestation) bool {
	data := att.Data

	sourceBlock, ok := c.Storage.GetBlock(data.Source.Root)
	if !ok {
		return false
	}
	targetBlock, ok := c.Storage.GetBlock(data.Target.Root)
	if !ok {
		return false
	}

	if sourceBlock.Slot > targetBlock.Slot {
		return false
	}
	if data.Source.Slot > data.Target.Slot {
		return false
	}
	if sourceBlock.Slot != data.Source.Slot {
		return false
	}
	if targetBlock.Slot != data.Target.Slot {
		return false
	}

	currentSlot := c.Time / types.IntervalsPerSlot
	if data.Slot > currentSlot+1 {
		return false
	}

	return true
}
