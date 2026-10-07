package node

import (
	"context"
	"testing"
	"time"

	"github.com/geanlabs/gean/crypto/insecure"
	"github.com/geanlabs/gean/types"
)

// blockingNetwork holds every attestation publish until its context ends, the
// way a stalled network implementation would.
type blockingNetwork struct {
	Network
	published chan struct{}
}

func (n *blockingNetwork) PublishAttestation(ctx context.Context, _ *types.SignedAttestation, _ uint64) error {
	close(n.published)
	<-ctx.Done()
	return ctx.Err()
}

// Attestations are published from the dispatch loop, so a publish that never
// returns would keep Run from returning and shutdown from completing. It must
// end with the engine's context.
func TestAttestationPublishEndsWithEngineContext(t *testing.T) {
	e := makeTestEngine()
	e.keys = insecure.NewKeys(0)
	network := &blockingNetwork{published: make(chan struct{})}
	e.network = network

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		e.produceAttestations(ctx, 1)
		close(done)
	}()

	select {
	case <-network.published:
	case <-done:
		t.Fatal("attestation was never published")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("attestation publish outlived the engine's context")
	}
}
