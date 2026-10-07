package node

import (
	"context"
)

func (e *Engine) startWorkers(ctx context.Context) {
	e.workers.Go(func() { e.runFetchBatcher(ctx) })
	e.workers.Go(func() { e.aggregationWorker.Run(ctx, e.aggregationDispatchCh) })
	e.workers.Go(func() { e.runProposalWorker(ctx) })
	e.workers.Go(func() { e.runRecoveryWorker(ctx) })
	e.workers.Go(func() { e.runAttestationWorker(ctx) })
	e.workers.Go(func() { e.runAggregationWorker(ctx) })
	e.workers.Go(func() { e.runGossipMeshGauge(ctx) })
	e.workers.Go(func() { e.runTickAgeGauge(ctx) })
	e.workers.Go(func() { e.runStorageSizeGauge(ctx) })
}

func (e *Engine) runAttestationWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case att := <-e.attestationCh:
			e.workers.Go(func() { e.onGossipAttestation(att) })
		}
	}
}

// runAggregationWorker verifies gossiped aggregates off the dispatch loop.
// Verification is a recursive XMSS proof check — the most expensive verify gean
// does — and on the loop it delayed store.OnTick, stalling the store clock.
// Unlike single attestations these are not fanned out per message: the check is
// CPU-bound, so serialising it here bounds the cost instead of thrashing. The
// buffers it writes are mutex-protected, so a worker goroutine is safe.
func (e *Engine) runAggregationWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case agg := <-e.aggregationCh:
			e.onGossipAggregatedAttestation(agg)
		}
	}
}
