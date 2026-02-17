package forkchoice

import (
	"fmt"
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
	data := sa.Message
	validatorID := sa.ValidatorID

	source := "gossip"
	if isFromBlock {
		source = "block"
	}

	if !c.validateAttestationData(data) {
		return
	}

	// Verify signature.
	if err := c.verifyAttestationSignature(sa); err != nil {
		return
	}

	if isFromBlock {
		// On-chain: update known attestations if this is newer.
		existing, ok := c.LatestKnownAttestations[validatorID]
		if !ok || existing.Message.Slot < data.Slot {
			c.LatestKnownAttestations[validatorID] = sa
		}
		// Remove from new attestations if superseded.
		newAtt, ok := c.LatestNewAttestations[validatorID]
		if ok && newAtt.Message.Target.Slot <= data.Target.Slot {
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
		if !ok || existing.Message.Target.Slot < data.Target.Slot {
			c.LatestNewAttestations[validatorID] = sa
		}
	}

	metrics.AttestationsValid.WithLabelValues(source).Inc()
	metrics.AttestationValidationTime.Observe(time.Since(start).Seconds())
}

// verifyAttestationSignature verifies the XMSS signature on the attestation.
func (c *Store) verifyAttestationSignature(sa *types.SignedAttestation) error {
	headState, ok := c.Storage.GetState(c.Head)
	if !ok {
		return fmt.Errorf("head state not found")
	}

	att := &types.Attestation{
		ValidatorID: sa.ValidatorID,
		Data:        sa.Message,
	}
	return c.verifyAttestationSignatureWithState(headState, att, sa.Signature)
}

// validateAttestationData performs attestation validation checks.
func (c *Store) validateAttestationData(data *types.AttestationData) bool {
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
