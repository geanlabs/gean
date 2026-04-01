package forkchoice

import (
	"fmt"
	"time"

	"github.com/geanlabs/gean/observability/metrics"
	"github.com/geanlabs/gean/types"
	"github.com/geanlabs/gean/xmss/leanmultisig"
)

// ProcessAttestation processes an attestation from a block or for direct forkchoice inclusion.
func (c *Store) ProcessAttestation(sa *types.SignedAttestation) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.NowFn != nil {
		c.advanceTimeLockedMillis(c.NowFn(), false)
	}

	c.processAttestationLocked(sa, false)
	if c.isAggregator {
		c.storeGossipSignatureLocked(sa)
	}
}

// ProcessSubnetAttestation processes an individual attestation from the subnet gossip topic.
// It validates and stores the gossip signature for aggregation, but does NOT update
// latestNewAttestations or latestKnownAttestations (no direct forkchoice influence).
func (c *Store) ProcessSubnetAttestation(sa *types.SignedAttestation) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.NowFn != nil {
		c.advanceTimeLockedMillis(c.NowFn(), false)
	}

	c.processSubnetAttestationLocked(sa)
}

func (c *Store) processSubnetAttestationLocked(sa *types.SignedAttestation) {
	start := time.Now()
	defer func() {
		metrics.AttestationValidationTime.Observe(time.Since(start).Seconds())
	}()

	data := sa.Message
	if data == nil {
		metrics.AttestationsInvalid.WithLabelValues("subnet").Inc()
		return
	}

	if reason := c.validateAttestationData(data); reason != "" {
		log.Debug("subnet attestation rejected", "reason", reason, "slot", data.Slot, "validator", sa.ValidatorID)
		if !isTransientAttestationRejection(reason) {
			metrics.AttestationsInvalid.WithLabelValues("subnet").Inc()
		}
		return
	}

	// Verify signature.
	if c.shouldVerifySignatures() {
		if err := c.verifyAttestationSignature(sa); err != nil {
			metrics.AttestationsInvalid.WithLabelValues("subnet").Inc()
			return
		}
	}

	// Future attestation guard.
	currentSlot := c.time / types.IntervalsPerSlot
	if data.Slot > currentSlot {
		return
	}

	// Store gossip signature for aggregation — only if this node is an aggregator.
	if c.isAggregator {
		c.storeGossipSignatureLocked(sa)
	}
	metrics.AttestationsValid.WithLabelValues("subnet").Inc()
}

func (c *Store) processAttestationLocked(sa *types.SignedAttestation, isFromBlock bool) {
	start := time.Now()
	defer func() {
		metrics.AttestationValidationTime.Observe(time.Since(start).Seconds())
	}()
	sourceLabel := "gossip"
	if isFromBlock {
		sourceLabel = "block"
	}

	data := sa.Message
	validatorID := sa.ValidatorID

	if data == nil {
		metrics.AttestationsInvalid.WithLabelValues(sourceLabel).Inc()
		return
	}

	if reason := c.validateAttestationData(data); reason != "" {
		log.Debug("attestation rejected", "reason", reason, "slot", data.Slot, "validator", validatorID)
		// Unknown/future references are common during gossip races and sync lag.
		// Keep invalid metric for deterministic/protocol-invalid cases.
		if !isTransientAttestationRejection(reason) {
			metrics.AttestationsInvalid.WithLabelValues(sourceLabel).Inc()
		}
		return
	}

	// Verify signature (skip for on-chain attestations; already verified in ProcessBlock).
	if !isFromBlock && c.shouldVerifySignatures() {
		if err := c.verifyAttestationSignature(sa); err != nil {
			metrics.AttestationsInvalid.WithLabelValues(sourceLabel).Inc()
			return
		}
	}

	if isFromBlock {
		// On-chain: update known attestations if this is newer.
		existing, ok := c.latestKnownAttestations[validatorID]
		if !ok || existing == nil || existing.Message == nil || existing.Message.Slot < data.Slot {
			c.latestKnownAttestations[validatorID] = sa
		}
		// Remove from new attestations if superseded.
		newAtt, ok := c.latestNewAttestations[validatorID]
		if ok && newAtt != nil && newAtt.Message != nil && newAtt.Message.Slot <= data.Slot {
			delete(c.latestNewAttestations, validatorID)
		}
	} else {
		// Network gossip attestation processing — used by aggregated attestation path.
		currentSlot := c.time / types.IntervalsPerSlot
		if data.Slot > currentSlot {
			return
		}

		// Update new attestations for forkchoice consideration.
		existing, ok := c.latestNewAttestations[validatorID]
		if !ok || existing == nil || existing.Message == nil || existing.Message.Slot < data.Slot {
			c.latestNewAttestations[validatorID] = sa
		}
		if c.isAggregator {
			c.storeGossipSignatureLocked(sa)
		}
	}

	metrics.AttestationsValid.WithLabelValues(sourceLabel).Inc()
}

func isTransientAttestationRejection(reason string) bool {
	switch reason {
	case "source block unknown", "target block unknown", "head block unknown", "attestation too far in future":
		return true
	default:
		return false
	}
}

// verifyAttestationSignature verifies the XMSS signature on the attestation.
func (c *Store) verifyAttestationSignature(sa *types.SignedAttestation) error {
	headState, ok := c.storage.GetState(c.head)
	if !ok {
		return fmt.Errorf("head state not found")
	}

	return c.verifyAttestationSignatureWithState(headState, sa.ValidatorID, sa.Message, sa.Signature)
}

// validateAttestationData performs attestation validation checks.
// Returns an empty string if valid, or a rejection reason.
func (c *Store) validateAttestationData(data *types.AttestationData) string {
	if data == nil || data.Source == nil || data.Target == nil || data.Head == nil {
		return "incomplete attestation data"
	}

	// Availability check: source, target, and head blocks must exist.
	sourceBlock, ok := c.lookupBlockSummary(data.Source.Root)
	if !ok {
		return "source block unknown"
	}
	targetBlock, ok := c.lookupBlockSummary(data.Target.Root)
	if !ok {
		return "target block unknown"
	}
	if _, ok := c.lookupBlockSummary(data.Head.Root); !ok {
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

// ProcessAggregatedAttestation processes an aggregated attestation from the aggregation gossip topic.
// It verifies the aggregated proof, expands participants into per-validator votes for forkchoice,
// and caches the proof for proposer reuse in block building.
func (c *Store) ProcessAggregatedAttestation(saa *types.SignedAggregatedAttestation) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.NowFn != nil {
		c.advanceTimeLockedMillis(c.NowFn(), false)
	}

	c.processAggregatedAttestationLocked(saa)
}

func (c *Store) processAggregatedAttestationLocked(saa *types.SignedAggregatedAttestation) {
	start := time.Now()
	defer func() {
		metrics.AttestationValidationTime.Observe(time.Since(start).Seconds())
	}()

	if saa == nil || saa.Data == nil || saa.Proof == nil {
		metrics.AttestationsInvalid.WithLabelValues("aggregation").Inc()
		return
	}

	data := saa.Data
	proof := saa.Proof

	// Validate attestation data references.
	if reason := c.validateAttestationData(data); reason != "" {
		log.Debug("aggregated attestation rejected", "reason", reason, "slot", data.Slot)
		if !isTransientAttestationRejection(reason) {
			metrics.AttestationsInvalid.WithLabelValues("aggregation").Inc()
		}
		return
	}

	// Extract validators from participants bitlist.
	validatorIDs := bitlistToValidatorIDs(proof.Participants)
	if len(validatorIDs) == 0 {
		metrics.AttestationsInvalid.WithLabelValues("aggregation").Inc()
		return
	}

	// Verify aggregated proof signature.
	if c.shouldVerifySignatures() {
		headState, ok := c.storage.GetState(c.head)
		if !ok {
			log.Warn("head state not found for aggregated proof verification")
			metrics.AttestationsInvalid.WithLabelValues("aggregation").Inc()
			return
		}

		pubkeys := make([][]byte, 0, len(validatorIDs))
		for _, vid := range validatorIDs {
			if vid >= uint64(len(headState.Validators)) {
				metrics.AttestationsInvalid.WithLabelValues("aggregation").Inc()
				return
			}
			pubkey := headState.Validators[vid].Pubkey
			pubkeys = append(pubkeys, pubkey[:])
		}

		messageRoot, err := data.HashTreeRoot()
		if err != nil {
			metrics.AttestationsInvalid.WithLabelValues("aggregation").Inc()
			return
		}

		leanmultisig.SetupVerifier()
		verifyStart := time.Now()
		if err := leanmultisig.VerifyAggregated(pubkeys, messageRoot, proof.ProofData, uint32(data.Slot)); err != nil {
			metrics.PQSigAggregatedVerificationTime.Observe(time.Since(verifyStart).Seconds())
			metrics.PQSigAggregatedInvalidTotal.Inc()
			log.Warn("aggregated attestation proof invalid", "slot", data.Slot, "participants", len(validatorIDs), "err", err)
			metrics.AttestationsInvalid.WithLabelValues("aggregation").Inc()
			return
		}
		metrics.PQSigAggregatedVerificationTime.Observe(time.Since(verifyStart).Seconds())
		metrics.PQSigAggregatedValidTotal.Inc()
		log.Info("aggregated attestation proof verified", "slot", data.Slot, "participants", len(validatorIDs))
	}

	// Store into the aggregated payloads buffer.
	// Attestations are expanded into per-validator votes during acceptNewAttestationsLocked.
	addAggregatedPayload(c.latestNewAggregatedPayloads, data, proof)
	metrics.LatestNewAggregatedPayloads.Set(float64(len(c.latestNewAggregatedPayloads)))

	// Also cache per-validator proof in aggregatedPayloads for proposer reuse.
	for _, vid := range validatorIDs {
		c.storeAggregatedPayloadLocked(vid, data, proof)
	}

	metrics.AttestationsValid.WithLabelValues("aggregation").Inc()
	log.Debug("processed aggregated attestation", "slot", data.Slot, "participants", len(validatorIDs))
}
