package sim

import (
	"context"
	"testing"
	"time"

	"github.com/geanlabs/gean/store"
	"github.com/geanlabs/gean/types"
)

// Under real concurrency (worker goroutines, results arriving whenever proofs
// finish, a contended prover gate) the nodes must still agree and finalize,
// and stopping must wait for all of it.
func TestRealTimeClusterFinalizes(t *testing.T) {
	if testing.Short() {
		t.Skip("runs for several real slots")
	}
	ctx := context.Background()
	r, err := StartRealTime(ctx, Config{
		GenesisTime: uint64(time.Now().Unix()),
		Validators:  4,
		Nodes:       [][]uint64{{0, 1}, {2, 3}, {}},
		Aggregators: []int{0},
	})
	if err != nil {
		t.Fatalf("start cluster: %v", err)
	}
	stopped := false
	defer func() {
		if !stopped {
			r.Stop()
		}
	}()

	deadline := time.After(20 * types.SecondsPerSlot * time.Second)
	poll := time.NewTicker(types.MillisecondsPerInterval * time.Millisecond)
	defer poll.Stop()
	for !allFinalized(r.Nodes()) {
		select {
		case <-deadline:
			t.Fatalf("no finality within 20 slots: finalized %v", finalizedSlots(r.Nodes()))
		case <-poll.C:
		}
	}

	r.Stop()
	stopped = true
	// Nodes may stop a block apart, but whatever each finalized must be on the
	// chain node 0 follows.
	reference := r.Nodes()[0].Store
	for i, n := range r.Nodes() {
		finalized := n.Store.LatestFinalized()
		if !onChain(reference, reference.Head(), finalized) {
			t.Fatalf("node %d finalized slot %d root %x, which is not on node 0's chain", i, finalized.Slot, finalized.Root)
		}
	}
}

// onChain reports whether checkpoint is head or one of its ancestors in s.
func onChain(s *store.ConsensusStore, head [32]byte, checkpoint *types.Checkpoint) bool {
	root := head
	for {
		if root == checkpoint.Root {
			return true
		}
		header := s.GetBlockHeader(root)
		if header == nil || header.Slot <= checkpoint.Slot {
			return false
		}
		root = header.ParentRoot
	}
}

func allFinalized(nodes []*Node) bool {
	for _, n := range nodes {
		if n.Store.LatestFinalized().Slot == 0 {
			return false
		}
	}
	return true
}

func finalizedSlots(nodes []*Node) []uint64 {
	slots := make([]uint64, len(nodes))
	for i, n := range nodes {
		slots[i] = n.Store.LatestFinalized().Slot
	}
	return slots
}
