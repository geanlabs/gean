package node

import (
	"errors"
	"time"

	"github.com/geanlabs/gean/internal/attestation"
	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/metrics"
	"github.com/geanlabs/gean/internal/p2p"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

func (e *Engine) onGossipAttestation(att *types.SignedAttestation) {
	if e.AggCtl == nil || !e.AggCtl.Get() || att == nil {
		return
	}

	start := time.Now()
	success := false
	defer func() {
		if success {
			metrics.ObserveAttestationValidationTime(time.Since(start).Seconds())
		}
	}()

	if err := attestation.ValidateAttestationData(e.Store, att.Data); err != nil {
		if se, ok := err.(*store.StoreError); ok && se.Kind == store.ErrUnknownHeadBlock && att.Data.Head != nil {
			added, dropped := e.PendingAttestations.Add(att.Data.Head.Root, att)
			if added {
				select {
				case e.FetchRootCh <- att.Data.Head.Root:
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
	if e.Store.AttestationSignatures.Has(dataRoot, att.ValidatorID) {
		return
	}

	metrics.IncPqSigAttestationSigsTotal()
	verifyStart := time.Now()
	err = attestation.VerifyGossipAttestation(e.Store, att.ValidatorID, att.Data, dataRoot, att.Signature[:])
	e.Shadow.SleepVerify()
	metrics.ObservePqSigVerificationTime(time.Since(verifyStart).Seconds())
	if err != nil {
		metrics.IncPqSigAttestationSigsInvalid()
		metrics.IncAttestationsInvalid()
		return
	}
	metrics.IncPqSigAttestationSigsValid()
	metrics.IncAttestationsValid(1)

	logger.Info(logger.Gossip, "attestation verified: validator=%d slot=%d dataRoot=%x", att.ValidatorID, att.Data.Slot, dataRoot)
	e.Store.AttestationSignatures.Insert(dataRoot, att.Data, att.ValidatorID, att.Signature)
	success = true

	// Nudge the dispatch loop to consider aggregating early now that another vote
	// is in. The signal is best-effort and coalescing; the loop owns the actual
	// timing and quorum decision.
	select {
	case e.EarlyAggregateCh <- struct{}{}:
	default:
	}
}

func (e *Engine) onGossipAggregatedAttestation(agg *types.SignedAggregatedAttestation) {
	if agg == nil {
		return
	}

	if err := attestation.ValidateAttestationData(e.Store, agg.Data); err != nil {
		return
	}

	if agg.Proof == nil || len(agg.Proof.Proof) == 0 {
		return
	}
	verifyStart := time.Now()
	err := attestation.VerifyAggregatedGossipAttestation(e.Store, agg.Data, agg.Proof.Participants, agg.Proof.Proof)
	e.Shadow.SleepVerifyAggregated(int(types.BitlistCount(agg.Proof.Participants)))
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
	e.Store.NewPayloads.Push(dataRoot, agg.Data, agg.Proof)
}

// ValidateGossipBlock decides whether a received block is relayed. Only checks
// that need no state are made here: the proof and the state transition still
// run on import, after relay, so a hop does not add verification latency. A
// block whose parent is unknown is relayed; the parent is usually in flight.
func (e *Engine) ValidateGossipBlock(block *types.SignedBlock) p2p.Verdict {
	slot := block.Block.Slot
	// The import horizon, on the wall clock rather than the store clock: the
	// store clock lags whenever the loop runs late, and judging by it would drop
	// current blocks. Beyond the horizon the block may still be valid to a peer
	// whose clock is ahead, so it is ignored, not rejected.
	if slot > e.currentSlot(uint64(time.Now().UnixMilli()))+1 {
		return p2p.Ignore
	}
	if finalized := e.Store.LatestFinalized(); finalized != nil && slot <= finalized.Slot {
		return p2p.Ignore
	}
	return p2p.Accept
}

// ValidateGossipAttestation decides whether a received vote is relayed. The
// signature is verified after relay, and only by aggregators, as before.
func (e *Engine) ValidateGossipAttestation(att *types.SignedAttestation) p2p.Verdict {
	return attestationDataVerdict(attestation.ValidateAttestationData(e.Store, att.Data))
}

// ValidateGossipAggregatedAttestation decides whether a received aggregate is
// relayed. Its proof is verified after relay.
func (e *Engine) ValidateGossipAggregatedAttestation(agg *types.SignedAggregatedAttestation) p2p.Verdict {
	if len(agg.Proof.Proof) == 0 || types.BitlistCount(agg.Proof.Participants) == 0 {
		return p2p.Reject
	}
	return attestationDataVerdict(attestation.ValidateAttestationData(e.Store, agg.Data))
}

// attestationDataVerdict maps ValidateAttestationData's result to a relay
// decision. Reject only covers data that contradicts itself, or a checkpoint
// slot that disagrees with the block its root names: invalid on every node.
// Votes that may become valid are accepted, as before, and judged again on
// delivery: unknown blocks, so the vote reaches the pending buffer, and the
// future bound, which is measured on the store clock that lags when the loop
// runs late. The rest depends on this node's view and is ignored; the node
// drops those votes on delivery anyway.
func attestationDataVerdict(err error) p2p.Verdict {
	if err == nil {
		return p2p.Accept
	}
	var se *store.StoreError
	if !errors.As(err, &se) {
		return p2p.Reject // malformed data
	}
	switch se.Kind {
	case store.ErrUnknownSourceBlock, store.ErrUnknownTargetBlock, store.ErrUnknownHeadBlock,
		store.ErrAttestationTooFarInFuture:
		return p2p.Accept
	case store.ErrSourceExceedsTarget, store.ErrHeadOlderThanTarget, store.ErrAttestationSlotBeforeHead,
		store.ErrSourceSlotMismatch, store.ErrTargetSlotMismatch, store.ErrHeadSlotMismatch:
		return p2p.Reject
	default:
		return p2p.Ignore
	}
}
