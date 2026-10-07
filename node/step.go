package node

import (
	"context"

	"github.com/geanlabs/gean/types"
)

// Tick runs the slot-interval tick at the clock's current time. With
// ProcessPending it drives the engine deterministically in place of Run.
func (e *Engine) Tick(ctx context.Context) {
	e.onTick(ctx)
}

// ProcessPending handles every queued event and worker job on the calling
// goroutine, one at a time in a fixed order, until none remain. Each job runs
// the same handler Run's workers use; only the scheduling differs. It is for
// an engine that is not running: a caller that owns scheduling, such as a
// deterministic simulation, calls Tick and ProcessPending instead of Run.
//
// It reports whether it handled anything. Sync delivery and failed-root
// notification block when their channels are full, so the caller must not
// queue more than they hold between calls.
func (e *Engine) ProcessPending(ctx context.Context) bool {
	handled := false
	for e.processOne(ctx) {
		handled = true
	}
	return handled
}

func (e *Engine) processOne(ctx context.Context) bool {
	if block, ok := poll(e.blockCh); ok {
		e.onBlock(block)
		return true
	}
	if root, ok := poll(e.failedRootCh); ok {
		e.onFailedRoot(root)
		return true
	}
	if att, ok := poll(e.attestationCh); ok {
		e.onGossipAttestation(ctx, att)
		return true
	}
	if agg, ok := poll(e.aggregationCh); ok {
		e.onGossipAggregatedAttestation(ctx, agg)
		return true
	}
	if att, ok := poll(e.verifiedAttestationCh); ok {
		e.addVerifiedAttestation(att)
		return true
	}
	if p, ok := poll(e.newPayloadCh); ok {
		e.addNewPayload(p)
		return true
	}
	if r, ok := poll(e.aggregationResultCh); ok {
		r.Apply(e.store)
		return true
	}
	if _, ok := poll(e.earlyAggregateCh); ok {
		e.maybeEarlyAggregate(e.nowMs())
		return true
	}
	if duty, ok := poll(e.proposalCh); ok {
		e.acceptProposal(ctx, e.proveProposal(ctx, duty))
		return true
	}
	if result, ok := poll(e.proposalResultCh); ok {
		e.acceptProposal(ctx, result)
		return true
	}
	if dispatch, ok := poll(e.aggregationDispatchCh); ok {
		e.aggregationWorker.Session(ctx, dispatch).Apply(e.store)
		return true
	}
	if block, ok := poll(e.recoveryCh); ok {
		e.recoverBlockProofs(ctx, block)
		return true
	}
	if root, ok := poll(e.fetchRootCh); ok {
		e.fireBatchFetch(ctx, e.collectFetchBatch(root))
		return true
	}
	return false
}

// collectFetchBatch gathers the roots already queued behind first, up to one
// batch, as the fetch batcher would within its grace period.
func (e *Engine) collectFetchBatch(first [32]byte) [][32]byte {
	batch := [][32]byte{first}
	seen := map[[32]byte]bool{first: true}
	for len(batch) < types.MaxBlocksPerRootFetch {
		root, ok := poll(e.fetchRootCh)
		if !ok {
			break
		}
		if !seen[root] {
			batch = append(batch, root)
			seen[root] = true
		}
	}
	return batch
}

func poll[T any](ch chan T) (T, bool) {
	select {
	case v := <-ch:
		return v, true
	default:
		var zero T
		return zero, false
	}
}
