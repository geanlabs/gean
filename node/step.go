package node

import (
	"context"

	"github.com/geanlabs/gean/types"
)

// Tick runs the slot-interval tick at the clock's current time. With
// ProcessPending it drives the engine deterministically in place of Run.
func (e *Engine) Tick() {
	e.onTick()
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
	if block, ok := poll(e.BlockCh); ok {
		e.onBlock(block)
		return true
	}
	if root, ok := poll(e.FailedRootCh); ok {
		e.onFailedRoot(root)
		return true
	}
	if att, ok := poll(e.AttestationCh); ok {
		e.onGossipAttestation(att)
		return true
	}
	if agg, ok := poll(e.AggregationCh); ok {
		e.onGossipAggregatedAttestation(agg)
		return true
	}
	if _, ok := poll(e.EarlyAggregateCh); ok {
		e.maybeEarlyAggregate(e.nowMs())
		return true
	}
	if duty, ok := poll(e.ProposalCh); ok {
		e.acceptProposal(ctx, e.proveProposal(ctx, duty))
		return true
	}
	if result, ok := poll(e.ProposalResultCh); ok {
		e.acceptProposal(ctx, result)
		return true
	}
	if dispatch, ok := poll(e.AggregationDispatchCh); ok {
		e.aggregator.Session(ctx, dispatch)
		return true
	}
	if block, ok := poll(e.RecoveryCh); ok {
		e.recoverBlockProofs(ctx, block)
		return true
	}
	if root, ok := poll(e.FetchRootCh); ok {
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
		root, ok := poll(e.FetchRootCh)
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
