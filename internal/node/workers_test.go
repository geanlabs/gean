package node

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/geanlabs/gean/internal/role"
	"github.com/geanlabs/gean/internal/shadow"
	"github.com/geanlabs/gean/internal/types"
)

func TestRunAttestationWorkerStopsOnContextCancel(t *testing.T) {
	e := &Engine{AttestationCh: make(chan *types.SignedAttestation)}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		e.runAttestationWorker(ctx)
		close(done)
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("attestation worker did not stop after context cancellation")
	}
}

// A flood of gossip attestations must not run more verifies at once than there are
// workers: each verify holds an OS thread in cgo.
func TestAttestationWorkerBoundsConcurrentVerifies(t *testing.T) {
	e := makeTestEngine()
	e.AggCtl = role.New(true)
	e.Shadow = shadow.Rates{VerifySignature: 20} // 50ms per verify
	genesis := e.Store.Head()
	checkpoint := &types.Checkpoint{Root: genesis}

	baseline := runtime.NumGoroutine()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go e.runAttestationWorker(ctx)

	const flood = 200
	go func() {
		for vid := range uint64(flood) {
			e.AttestationCh <- &types.SignedAttestation{
				ValidatorID: vid,
				Data:        &types.AttestationData{Head: checkpoint, Target: checkpoint, Source: checkpoint},
			}
		}
	}()

	peak := 0
	for deadline := time.Now().Add(300 * time.Millisecond); time.Now().Before(deadline); {
		peak = max(peak, runtime.NumGoroutine()-baseline)
		time.Sleep(time.Millisecond)
	}
	// Workers, the dispatcher and the feeding goroutine; nothing per attestation.
	if limit := runtime.GOMAXPROCS(0) + 2; peak > limit {
		t.Fatalf("peak extra goroutines=%d during a %d-attestation flood, want <= %d", peak, flood, limit)
	}
}

// Shutdown joins the attestation workers before closing storage, so cancelling
// must not let runAttestationWorker return while a verify is still running.
func TestRunAttestationWorkerWaitsForInFlightVerify(t *testing.T) {
	e := makeTestEngine()
	e.AggCtl = role.New(true)
	e.Shadow = shadow.Rates{VerifySignature: 5} // 200ms per verify
	checkpoint := &types.Checkpoint{Root: e.Store.Head()}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		e.runAttestationWorker(ctx)
		close(done)
	}()

	start := time.Now()
	e.AttestationCh <- &types.SignedAttestation{
		Data: &types.AttestationData{Head: checkpoint, Target: checkpoint, Source: checkpoint},
	}
	time.Sleep(20 * time.Millisecond)
	cancel()
	<-done
	if elapsed := time.Since(start); elapsed < 150*time.Millisecond {
		t.Fatalf("worker returned %v after receiving an attestation, before its 200ms verify finished", elapsed)
	}
}

// A full verify queue drops attestations without logging each one; the
// chain-status line reports the count once and resets it.
func TestOnGossipAttestationCountsDrops(t *testing.T) {
	e := makeTestEngine()
	e.AttestationCh = make(chan *types.SignedAttestation) // no receiver: every send drops

	for range 3 {
		e.OnGossipAttestation(&types.SignedAttestation{})
	}
	if got := e.attestationsDropped.Load(); got != 3 {
		t.Fatalf("dropped=%d, want 3", got)
	}
	e.logChainStatus(0)
	if got := e.attestationsDropped.Load(); got != 0 {
		t.Fatalf("dropped=%d after chain status, want 0", got)
	}
}
