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
	c.processAttestationLocked(sa, false)
}

func (c *Store) processAttestationLocked(sa *types.SignedAttestation, isFromBlock bool) {
	start := time.Now()
	att := sa.Message
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
		// On-chain: update known attestations if this is newer.
		existing, ok := c.LatestKnownAttestations[validatorID]
		if !ok || existing.Message.Data.Slot < data.Slot {
			c.LatestKnownAttestations[validatorID] = sa
		}
		// Remove from new attestations if superseded.
		newAtt, ok := c.LatestNewAttestations[validatorID]
		if ok && newAtt.Message.Data.Target.Slot <= data.Target.Slot {
			delete(c.LatestNewAttestations, validatorID)
		}
	} else {
		// Network gossip attestation processing.
		currentSlot := c.Time / types.IntervalsPerSlot
		if data.Slot > currentSlot {
			return
		}

		// Network gossip: update new attestations if this is newer.
		existing, ok := c.LatestNewAttestations[validatorID]
		if !ok || existing.Message.Data.Target.Slot < data.Target.Slot {
			c.LatestNewAttestations[validatorID] = sa
		}
	}

	metrics.AttestationsValid.WithLabelValues(source).Inc()
	metrics.AttestationValidationTime.Observe(time.Since(start).Seconds())
}

// validateAttestationLocked performs attestation validation checks.
func (c *Store) validateAttestationLocked(att *types.Attestation) bool {
	data := att.Data

	// Availability check: source, target, and head blocks must exist.
	sourceBlock, ok := c.Storage.GetBlock(data.Source.Root)
	if !ok {
		return false
	}
	targetBlock, ok := c.Storage.GetBlock(data.Target.Root)
	if !ok {
		return false
	}
	if _, ok := c.Storage.GetBlock(data.Head.Root); !ok {
		return false
	}

	// Topology check.
	if sourceBlock.Slot > targetBlock.Slot {
		return false
	}
	if data.Source.Slot > data.Target.Slot {
		return false
	}

	// Consistency check.
	if sourceBlock.Slot != data.Source.Slot {
		return false
	}
	if targetBlock.Slot != data.Target.Slot {
		return false
	}

	// Time check.
	currentSlot := c.Time / types.IntervalsPerSlot
	if data.Slot > currentSlot+1 {
		return false
	}

	return true
}
