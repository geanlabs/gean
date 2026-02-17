package forkchoice

import (
	"fmt"
	"time"

	"github.com/geanlabs/gean/leansig"
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

	// Verify signature.
	if err := c.verifyAttestationSignature(sa); err != nil {
		// metrics.AttestationsInvalidSignature.Inc()
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

// verifyAttestationSignature verifies the XMSS signature on the attestation.
func (c *Store) verifyAttestationSignature(sa *types.SignedAttestation) error {
	// 1. Get validator public key.
	// We need access to state to get the pubkey.
	// Since verification is stateless w.r.t the specific block, we might not have the "current" state loaded.
	// However, forkchoice store has c.Head state.
	// Validator set is static for Devnet-1.
	// We can get the pubkey from the head state or any state.
	// If dynamic, we'd need the state at the target epoch.

	// c.Storage.GetState(c.Head) is safe for static validators.
	headState, ok := c.Storage.GetState(c.Head)
	if !ok {
		return fmt.Errorf("head state not found")
	}

	valID := sa.Message.ValidatorID
	if valID >= uint64(len(headState.Validators)) {
		return fmt.Errorf("invalid validator index")
	}
	pubkey := headState.Validators[valID].Pubkey

	// 2. Compute signing root (HashTreeRoot of AttestationData).
	dataRoot, err := sa.Message.Data.HashTreeRoot()
	if err != nil {
		return err
	}

	// 3. Verify.
	epoch := uint32(sa.Message.Data.Target.Slot / types.SlotsPerEpoch)

	// sa.Signature is [3112]byte. Transform to slice.
	sig := sa.Signature[:]

	if err := leansig.Verify(pubkey[:], epoch, dataRoot, sig); err != nil {
		log.Warn("attestation signature invalid", "slot", sa.Message.Data.Slot, "validator", valID, "err", err)
		return err
	}
	log.Info("attestation verified (XMSS)", "slot", sa.Message.Data.Slot, "validator", valID, "sig_size", fmt.Sprintf("%d bytes", len(sig)))
	return nil
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
