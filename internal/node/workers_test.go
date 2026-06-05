package node

import (
	"context"
	"testing"
	"time"

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
