package node

import (
	"time"

	"github.com/geanlabs/gean/attestation"
	"github.com/geanlabs/gean/logger"
	"github.com/geanlabs/gean/metrics"
	"github.com/geanlabs/gean/store"
	"github.com/geanlabs/gean/types"
)

func (e *Engine) onGossipAttestation(att *types.SignedAttestation) {
	if e.aggregator == nil || !e.aggregator.Get() || att == nil {
		return
	}

	start := time.Now()
	success := false
	defer func() {
		if success {
			metrics.ObserveAttestationValidationTime(time.Since(start).Seconds())
		}
	}()

	if err := attestation.ValidateAttestationData(e.store, att.Data); err != nil {
		if se, ok := err.(*store.StoreError); ok && se.Kind == store.ErrUnknownHeadBlock && att.Data.Head != nil {
			added, dropped := e.pendingAttestations.Add(att.Data.Head.Root, att)
			if added {
				select {
				case e.fetchRootCh <- att.Data.Head.Root:
				default:
				}
			}
			if dropped > 0 {
				metrics.IncAttestationsBufferEvicted(dropped)
			}
		}
		return
	}

	dataRoot, err := att.Data.HashTreeRoot()
	if err != nil {
		logger.Error(logger.Gossip, "attestation root failed validator=%d: %v", att.ValidatorID, err)
		return
	}

	// A duplicate arrival cannot change the answer, and verification is the
	// expensive step. The store's own record of what it holds serves as the
	// seen set, so it is pruned along with the signatures themselves.
	if e.store.AttestationSignatures().Has(dataRoot, att.ValidatorID) {
		return
	}

	metrics.IncPqSigAttestationSigsTotal()
	verifyStart := time.Now()
	err = attestation.VerifyGossipAttestation(e.store, e.scheme, att.ValidatorID, att.Data, dataRoot, att.Signature[:])
	e.shadowRates.SleepVerify()
	metrics.ObservePqSigVerificationTime(time.Since(verifyStart).Seconds())
	if err != nil {
		metrics.IncPqSigAttestationSigsInvalid()
		metrics.IncAttestationsInvalid()
		return
	}
	metrics.IncPqSigAttestationSigsValid()
	metrics.IncAttestationsValid(1)

	logger.Info(logger.Gossip, "attestation verified: validator=%d slot=%d dataRoot=%x", att.ValidatorID, att.Data.Slot, dataRoot)
	e.store.AttestationSignatures().Insert(dataRoot, att.Data, att.ValidatorID, att.Signature)
	success = true

	// Nudge the dispatch loop to consider aggregating early now that another vote
	// is in. The signal is best-effort and coalescing; the loop owns the actual
	// timing and quorum decision.
	select {
	case e.earlyAggregateCh <- struct{}{}:
	default:
	}
}

func (e *Engine) onGossipAggregatedAttestation(agg *types.SignedAggregatedAttestation) {
	if agg == nil {
		return
	}

	if err := attestation.ValidateAttestationData(e.store, agg.Data); err != nil {
		return
	}

	if agg.Proof == nil || len(agg.Proof.Proof) == 0 {
		return
	}
	verifyStart := time.Now()
	err := attestation.VerifyAggregatedGossipAttestation(e.store, e.scheme, agg.Data, agg.Proof.Participants, agg.Proof.Proof)
	e.shadowRates.SleepVerifyAggregated(int(types.BitlistCount(agg.Proof.Participants)))
	metrics.ObservePqSigAggVerificationTime(time.Since(verifyStart).Seconds())
	if err != nil {
		metrics.IncPqSigAggregatedInvalid()
		logger.Error(logger.Signature, "aggregated attestation verification failed: %v", err)
		return
	}
	metrics.IncPqSigAggregatedValid()

	dataRoot, err := agg.Data.HashTreeRoot()
	if err != nil {
		logger.Error(logger.Signature, "aggregated attestation root failed: %v", err)
		return
	}
	e.store.NewPayloads().Push(dataRoot, agg.Data, agg.Proof)
}
