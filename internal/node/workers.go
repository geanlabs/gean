package node

import (
	"context"
	"runtime"
	"sync"

	"github.com/geanlabs/gean/internal/aggregation"
)

func (e *Engine) startWorkers(ctx context.Context) {
	go e.runFetchBatcher(ctx)
	go aggregation.RunWorker(ctx, e.AggregationDispatchCh, e.Store, e.Store.PubKeyCache, e.P2P, e.ProvingGate, e.Shadow)
	go e.runProposalWorker(ctx)
	go e.runRecoveryWorker(ctx)
	go e.runAggregationWorker(ctx)
	go e.runGossipMeshGauge(ctx)
	go e.runStallGauges(ctx)

	// These read the backend, so shutdown has to join them before Close: see
	// Engine.WaitForStorageWorkers.
	e.storageWorkers.Add(2)
	go func() {
		defer e.storageWorkers.Done()
		e.runAttestationWorker(ctx)
	}()
	go func() {
		defer e.storageWorkers.Done()
		e.runStorageSizeGauge(ctx)
	}()
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
