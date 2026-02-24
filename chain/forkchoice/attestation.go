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

	if c.NowFn != nil {
		c.advanceTimeLocked(c.NowFn(), false)
	}

	c.processAttestationLocked(sa, false)
}

func (c *Store) processAttestationLocked(sa *types.SignedAttestation, isFromBlock bool) {
	start := time.Now()
	defer func() {
		metrics.AttestationValidationTime.Observe(time.Since(start).Seconds())
	}()

	data := sa.Message.Data
	validatorID := sa.Message.ValidatorID

	if reason := c.validateAttestationData(data); reason != "" {
		log.Debug("attestation rejected", "reason", reason, "slot", data.Slot, "validator", validatorID)
		metrics.AttestationsInvalid.Inc()
		return
	}

	// Verify signature (skip for on-chain attestations; already verified in ProcessBlock).
	if !isFromBlock && c.shouldVerifySignatures() {
		if err := c.verifyAttestationSignature(sa); err != nil {
			metrics.AttestationsInvalid.Inc()
			return
		}
	}

	if isFromBlock {
		// On-chain: update known attestations if this is newer.
		existing, ok := c.latestKnownAttestations[validatorID]
		if !ok || existing.Message.Data.Slot < data.Slot {
			c.latestKnownAttestations[validatorID] = sa
		}
		// Remove from new attestations if superseded.
		newAtt, ok := c.latestNewAttestations[validatorID]
		if ok && newAtt.Message.Data.Slot <= data.Slot {
			delete(c.latestNewAttestations, validatorID)
		}
	} else {
		// Network gossip attestation processing.
		currentSlot := c.time / types.IntervalsPerSlot
		if data.Slot > currentSlot {
			metrics.AttestationsInvalid.Inc()
			return
		}

		// Network gossip: update new attestations if this is newer.
		existing, ok := c.latestNewAttestations[validatorID]
		if !ok || existing.Message.Data.Slot < data.Slot {
			c.latestNewAttestations[validatorID] = sa
		}
	}

	metrics.AttestationsValid.Inc()
}

// verifyAttestationSignature verifies the XMSS signature on the attestation.
func (c *Store) verifyAttestationSignature(sa *types.SignedAttestation) error {
	headState, ok := c.storage.GetState(c.head)
	if !ok {
		return fmt.Errorf("head state not found")
	}

	return c.verifyAttestationSignatureWithState(headState, sa.Message, sa.Signature)
}

// validateAttestationData performs attestation validation checks.
// Returns an empty string if valid, or a rejection reason.
func (c *Store) validateAttestationData(data *types.AttestationData) string {
	// Availability check: source, target, and head blocks must exist.
	sourceBlock, ok := c.storage.GetBlock(data.Source.Root)
	if !ok {
		return "source block unknown"
	}
	targetBlock, ok := c.storage.GetBlock(data.Target.Root)
	if !ok {
		return "target block unknown"
	}
	if _, ok := c.storage.GetBlock(data.Head.Root); !ok {
		return "head block unknown"
	}

	// Topology check.
	if sourceBlock.Slot > targetBlock.Slot {
		return "source slot > target slot"
	}
	if data.Source.Slot > data.Target.Slot {
		return "source slot > target slot"
	}

	// Consistency check.
	if sourceBlock.Slot != data.Source.Slot {
		return "source checkpoint slot mismatch"
	}
	if targetBlock.Slot != data.Target.Slot {
		return "target checkpoint slot mismatch"
	}

	// Time check.
	currentSlot := c.time / types.IntervalsPerSlot
	if data.Slot > currentSlot+1 {
		return "attestation too far in future"
	}

	return ""
}
