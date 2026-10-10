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
