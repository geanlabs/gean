package node

import (
	"context"

	"github.com/geanlabs/gean/types"
)

// verifiedAttestation is a gossip attestation whose signature checked out,
// ready to join the aggregation pool.
type verifiedAttestation struct {
	dataRoot    [32]byte
	data        *types.AttestationData
	validatorID uint64
	signature   [types.SignatureSize]byte
}

// newPayload is an aggregated proof ready to join the new-payload pool.
type newPayload struct {
	dataRoot [32]byte
	data     *types.AttestationData
	proof    *types.SingleMessageAggregate
}

// submit hands a worker's result to the dispatch loop, or gives up when the
// engine stops.
func submit[T any](ctx context.Context, ch chan<- T, v T) {
	select {
	case ch <- v:
	case <-ctx.Done():
	}
}

func (e *Engine) addVerifiedAttestation(att verifiedAttestation) {
	e.store.AttestationSignatures().Insert(att.dataRoot, att.data, att.validatorID, att.signature)
	// Nudge the dispatch loop to consider aggregating early now that another
	// vote is in. The signal is coalescing; the loop owns the timing and the
	// quorum decision.
	select {
	case e.earlyAggregateCh <- struct{}{}:
	default:
	}
}

func (e *Engine) addNewPayload(p newPayload) {
	e.store.NewPayloads().Push(p.dataRoot, p.data, p.proof)
}

// applyPendingResults applies every worker result already handed over. The
// tick calls it before advancing the store clock, so a result finished before
// an interval boundary lands in the pool that boundary promotes or aggregates.
func (e *Engine) applyPendingResults() {
	for {
		if att, ok := poll(e.verifiedAttestationCh); ok {
			e.addVerifiedAttestation(att)
			continue
		}
		if p, ok := poll(e.newPayloadCh); ok {
			e.addNewPayload(p)
			continue
		}
		if r, ok := poll(e.aggregationResultCh); ok {
			r.Apply(e.store)
			continue
		}
		return
	}
}
