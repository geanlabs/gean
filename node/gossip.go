package node

import (
	"context"
	"time"

	"github.com/geanlabs/gean/consensus/attestation"
	"github.com/geanlabs/gean/logger"
	"github.com/geanlabs/gean/storage/store"
	"github.com/geanlabs/gean/types"
)

func (e *Engine) onGossipAttestation(ctx context.Context, att *types.SignedAttestation) {
	if e.aggregator == nil || !e.aggregator.Get() || att == nil {
		return
	}

	start := time.Now()
	success := false
	defer func() {
		if success {
			e.metrics.ObserveAttestationValidationTime(time.Since(start).Seconds())
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
				e.metrics.IncAttestationsBufferEvicted(dropped)
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

	e.metrics.IncPqSigAttestationSigsTotal()
	verifyStart := time.Now()
	err = attestation.VerifyGossipAttestation(e.store, e.scheme, att.ValidatorID, att.Data, dataRoot, att.Signature[:])
	e.shadowRates.SleepVerify()
	e.metrics.ObservePqSigVerificationTime(time.Since(verifyStart).Seconds())
	if err != nil {
		e.metrics.IncPqSigAttestationSigsInvalid()
		e.metrics.IncAttestationsInvalid()
		return
	}
	e.metrics.IncPqSigAttestationSigsValid()
	e.metrics.IncAttestationsValid(1)

	logger.Info(logger.Gossip, "attestation verified: validator=%d slot=%d dataRoot=%x", att.ValidatorID, att.Data.Slot, dataRoot)
	success = true
	submit(ctx, e.verifiedAttestationCh, verifiedAttestation{
		dataRoot:    dataRoot,
		data:        att.Data,
		validatorID: att.ValidatorID,
		signature:   att.Signature,
	})
}

func (e *Engine) onGossipAggregatedAttestation(ctx context.Context, agg *types.SignedAggregatedAttestation) {
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
	e.metrics.ObservePqSigAggVerificationTime(time.Since(verifyStart).Seconds())
	if err != nil {
		e.metrics.IncPqSigAggregatedInvalid()
		logger.Error(logger.Signature, "aggregated attestation verification failed: %v", err)
		return
	}
	e.metrics.IncPqSigAggregatedValid()

	dataRoot, err := agg.Data.HashTreeRoot()
	if err != nil {
		logger.Error(logger.Signature, "aggregated attestation root failed: %v", err)
		return
	}
	submit(ctx, e.newPayloadCh, newPayload{dataRoot: dataRoot, data: agg.Data, proof: agg.Proof})
}
