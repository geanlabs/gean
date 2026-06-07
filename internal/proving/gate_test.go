package proving

import (
	"context"
	"testing"
)

func TestGateRejectsBackgroundWhileProposalPending(t *testing.T) {
	gate := NewGate()
	if !gate.Acquire(context.Background(), false) {
		t.Fatal("first background acquire failed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if gate.Acquire(ctx, true) {
		t.Fatal("canceled proposal acquired gate")
	}
	gate.proposalPending.Store(true)
	if gate.Acquire(context.Background(), false) {
		t.Fatal("background acquired while proposal pending")
	}
	gate.proposalPending.Store(false)
	gate.Release(false)
}
