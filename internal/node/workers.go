package node

import (
	"context"
	"runtime"
	"sync"

	"github.com/geanlabs/gean/internal/aggregation"
)

// startWorkers starts every worker on e.tasks, so Engine.Wait joins them.
func (e *Engine) startWorkers(ctx context.Context) {
	for _, run := range []func(){
		func() { e.runFetchBatcher(ctx) },
		func() {
			aggregation.RunWorker(ctx, e.AggregationDispatchCh, e.Store, e.Store.PubKeyCache, e.P2P, e.ProvingGate, e.Shadow)
		},
		func() { e.runProposalWorker(ctx) },
		func() { e.runRecoveryWorker(ctx) },
		func() { e.runAttestationWorker(ctx) },
		func() { e.runAggregationWorker(ctx) },
		func() { e.runGossipMeshGauge(ctx) },
		func() { e.runStallGauges(ctx) },
		func() { e.runStorageSizeGauge(ctx) },
	} {
		e.tasks.Go(run)
	}
}

// runAttestationWorker verifies gossip attestations on one worker per core. Each
// verify holds an OS thread in cgo, so a fixed pool bounds the threads a flood can
// take, and once every worker is busy AttestationCh fills and drops the excess.
// It returns only after every worker has finished its current verify.
func (e *Engine) runAttestationWorker(ctx context.Context) {
	var wg sync.WaitGroup
	for range runtime.GOMAXPROCS(0) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case att := <-e.AttestationCh:
					e.onGossipAttestation(att)
				}
			}
		}()
	}
	wg.Wait()
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
		case agg := <-e.AggregationCh:
			e.onGossipAggregatedAttestation(agg)
		}
	}
}
