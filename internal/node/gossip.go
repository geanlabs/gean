package node

import (
	"time"

	"github.com/geanlabs/gean/internal/attestation"
	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/metrics"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
	"github.com/geanlabs/gean/xmss"
)

// onGossipAttestation validates and stores an individual attestation.
// Only aggregator nodes store gossip signatures.
// Non-aggregators validate and drop — they receive aggregated proofs via the
// aggregation gossip topic instead.
func (e *Engine) onGossipAttestation(att *types.SignedAttestation) {
	if !e.AggCtl.Get() {
		return
	}

	// Match leanMetrics observe_on_attestation: emit on clean success only.
	start := time.Now()
	success := false
	defer func() {
		if success {
			metrics.ObserveAttestationValidationTime(time.Since(start).Seconds())
		}
	}()

	// Validate attestation data. If the attestation references a head block
	// we don't yet know about, buffer it under that head root and fire a
	// targeted BlocksByRoot fetch — when the block arrives, the block import
	// path drains the bucket and replays each attestation through this same
	// function. Source/target unknowns stay strict-drop for now: head is the
	// dominant ordering race in practice (proposer's own block delayed in
	// gossip dispersion); source/target races are rarer and can be handled
	// the same way later if metrics show it matters.
	if err := attestation.ValidateAttestationData(e.Store, att.Data); err != nil {
		if se, ok := err.(*store.StoreError); ok && se.Kind == store.ErrUnknownHeadBlock && att.Data.Head != nil {
			added, dropped := e.PendingAttestations.Add(att.Data.Head.Root, att)
			if added {
				select {
				case e.FetchRootCh <- att.Data.Head.Root:
				default:
					// Batcher channel full — that's fine. Multiple attestations
					// for the same root would otherwise produce one extra fetch
					// each anyway, and the batcher dedups within its grace window.
				}
			}
			if dropped > 0 {
				metrics.IncAttestationsBufferEvicted(dropped)
			}
		}
		return
	}

	dataRoot, _ := att.Data.HashTreeRoot()

	metrics.IncPqSigAttestationSigsTotal()
	verifyStart := time.Now()
	err := attestation.VerifyGossipAttestation(e.Store, att.ValidatorID, att.Data, dataRoot, att.Signature[:])
	metrics.ObservePqSigVerificationTime(time.Since(verifyStart).Seconds())
	if err != nil {
		metrics.IncPqSigAttestationSigsInvalid()
		metrics.IncAttestationsInvalid()
		return
	}
	metrics.IncPqSigAttestationSigsValid()
	metrics.IncAttestationsValid(1)

	// Parse signature to an opaque C handle for aggregation.
	sigHandle, parseErr := xmss.ParseSignature(att.Signature[:])

	// Store for aggregation.
	logger.Info(logger.Gossip, "attestation verified: validator=%d slot=%d dataRoot=%x", att.ValidatorID, att.Data.Slot, dataRoot)
	e.Store.AttestationSignatures.InsertWithHandle(dataRoot, att.Data, att.ValidatorID, att.Signature, sigHandle, parseErr)
	success = true
}

// onGossipAggregatedAttestation validates and stores an aggregated attestation.
func (e *Engine) onGossipAggregatedAttestation(agg *types.SignedAggregatedAttestation) {
	// Validate attestation data.
	if err := attestation.ValidateAttestationData(e.Store, agg.Data); err != nil {
		return
	}

	// Verify aggregated proof.
	if agg.Proof != nil && len(agg.Proof.ProofData) > 0 {
		if err := attestation.VerifyAggregatedGossipAttestation(e.Store, agg.Data, agg.Proof.Participants, agg.Proof.ProofData); err != nil {
			logger.Error(logger.Signature, "aggregated attestation verification failed: %v", err)
			return
		}
	}

	// Store in new payloads.
	dataRoot, _ := agg.Data.HashTreeRoot()
	e.Store.NewPayloads.Push(dataRoot, agg.Data, agg.Proof)
}
