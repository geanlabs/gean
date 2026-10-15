package proving

import (
	"context"
	"sync/atomic"
	"time"
)

type Gate struct {
	token           chan struct{}
	proposalPending atomic.Bool
	// heldSinceMs is when the current holder took the token, 0 while it is free;
	// heldByProposal says which kind of work holds it.
	heldSinceMs    atomic.Int64
	heldByProposal atomic.Bool
}

func NewGate() *Gate {
	g := &Gate{token: make(chan struct{}, 1)}
	g.token <- struct{}{}
	return g
}

func (g *Gate) Acquire(ctx context.Context, proposal bool) bool {
	if proposal {
		g.proposalPending.Store(true)
	} else if g.proposalPending.Load() {
		return false
	}
	select {
	case <-ctx.Done():
		if proposal {
			g.proposalPending.Store(false)
		}
		return false
	case <-g.token:
		if !proposal && g.proposalPending.Load() {
			g.token <- struct{}{}
			return false
		}
		g.heldByProposal.Store(proposal)
		g.heldSinceMs.Store(time.Now().UnixMilli())
		return true
	}
}

func (g *Gate) Release(proposal bool) {
	// Clear the priority flag before returning the token: a background waiter
	// woken by the token send re-checks the flag, and the reverse order lets it
	// observe a stale true and spuriously cancel its work.
	if proposal {
		g.proposalPending.Store(false)
	}
	g.heldSinceMs.Store(0)
	g.token <- struct{}{}
}

// Held reports how long the current holder has held the gate, and whether it is
// a proposal; zero while the gate is free. A native proof cannot be cancelled,
// so a holder that never returns is visible only here.
func (g *Gate) Held(now time.Time) (time.Duration, bool) {
	since := g.heldSinceMs.Load()
	if since == 0 {
		return 0, false
	}
	return max(now.Sub(time.UnixMilli(since)), 0), g.heldByProposal.Load()
}

// ProposalPending lets a background owner yield between non-interruptible proofs.
func (g *Gate) ProposalPending() bool {
	return g != nil && g.proposalPending.Load()
}
