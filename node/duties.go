package node

import (
	"context"
	"time"

	"github.com/geanlabs/gean/attestation"
	"github.com/geanlabs/gean/logger"
	"github.com/geanlabs/gean/metrics"
	"github.com/geanlabs/gean/types"
)

func (e *Engine) produceAttestations(ctx context.Context, slot uint64) {
	if e.keys == nil {
		return
	}

	if e.dutyGate != nil && !e.dutyGate.Decide("attestation", slot, e.store.HeadSlot(), e.networkSeenSlot()) {
		metrics.IncAttestationsSkippedLag()
		return
	}

	attData := attestation.ProduceAttestationData(e.store, slot)
	if attData == nil {
		return
	}

	for _, vid := range e.keys.ValidatorIDs() {
		prodStart := time.Now()

		sStart := time.Now()
		sig, err := e.keys.SignAttestation(vid, attData)
		metrics.ObservePqSigSigningTime(time.Since(sStart).Seconds())
		if err != nil {
			logger.Error(logger.Validator, "sign attestation failed validator=%d: %v", vid, err)
			continue
		}

		signedAtt := &types.SignedAttestation{
			ValidatorID: vid,
			Data:        attData,
			Signature:   sig,
		}

		logger.Info(logger.Validator, "produced attestation slot=%d validator=%d", slot, vid)

		if e.aggregator != nil && e.aggregator.Get() {
			dataRoot, err := attData.HashTreeRoot()
			if err != nil {
				logger.Error(logger.Validator, "attestation root failed validator=%d: %v", vid, err)
				continue
			}
			e.store.AttestationSignatures().Insert(dataRoot, attData, vid, sig)
		}

		if e.network != nil {
			// Publishing runs on the dispatch loop, so it is bounded like a
			// proposal publish and ends when the engine shuts down.
			publishCtx, cancel := context.WithTimeout(ctx, types.MillisecondsPerInterval*time.Millisecond)
			err := e.network.PublishAttestation(publishCtx, signedAtt, e.committeeCount)
			cancel()
			if err != nil {
				logger.Error(logger.Network, "publish attestation failed validator=%d: %v", vid, err)
			} else {
				logger.Info(logger.Network, "published attestation to network slot=%d validator=%d", slot, vid)
			}
		}

		metrics.ObserveAttestationsProductionTime(time.Since(prodStart).Seconds())
	}
}

// proposingAt reports whether one of this node's validators proposes at slot,
// given an already-resolved validator count. getOurProposer decodes the head
// state to learn that count; callers holding a state should use this instead
// rather than pay a second SSZ decode on the tick loop.
func (e *Engine) proposingAt(slot uint64, numValidators uint64) bool {
	if e.keys == nil || numValidators == 0 {
		return false
	}
	for _, vid := range e.keys.ValidatorIDs() {
		if types.IsProposer(slot, vid, numValidators) {
			return true
		}
	}
	return false
}

func (e *Engine) getOurProposer(slot uint64) (uint64, bool) {
	if e.keys == nil {
		return 0, false
	}
	headState := e.store.GetState(e.store.Head())
	if headState == nil {
		return 0, false
	}
	numValidators := headState.NumValidators()

	for _, vid := range e.keys.ValidatorIDs() {
		if types.IsProposer(slot, vid, numValidators) {
			return vid, true
		}
	}
	return 0, false
}
