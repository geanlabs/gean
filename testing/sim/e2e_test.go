//go:build fakecrypto

package sim

import (
	"strconv"
	"testing"
	"time"
)

// slotTimeout bounds each wait below. A lean slot is 4s; finality takes a few
// slots, so a wait this long means it has stalled.
const slotTimeout = 120 * time.Second

// missedSlots is how many slots the restarted node misses.
const missedSlots = 6

// Three nodes finalize, keep finalizing with one of them down, and the node
// that was down catches up after a restart. Validators are dealt to nodes by
// index modulo the node count, so node2 holds one of four and the other two
// nodes keep the two-thirds majority without it.
func TestE2EFinalizationAndRestart(t *testing.T) {
	s := Start(t, Config{Nodes: 3, Validators: 4, GenesisDelay: 3 * time.Second, Aggregators: []int{0}})

	finalized := waitFinalizedAbove(t, s.Nodes, 0)
	t.Logf("all nodes finalized slot %d", finalized)

	down := s.Nodes[2]
	stoppedAt := headSlot(t, down)
	if err := down.Stop(); err != nil {
		t.Fatalf("stop %s: %v", down.ID, err)
	}
	finalized = waitFinalizedAbove(t, s.Nodes[:2], finalized)
	// Stay down for more slots than the sync driver's threshold, so catching
	// up takes a sync rather than the next gossip block.
	Eventually(t, slotTimeout, "the network to move past the stopped node", func() bool {
		return headSlot(t, s.Nodes[0]) >= stoppedAt+missedSlots
	})
	t.Logf("with %s down from slot %d, finalized slot %d", down.ID, stoppedAt, finalized)

	target := headSlot(t, s.Nodes[0])
	down.Start()
	Eventually(t, slotTimeout, down.ID+" to catch up to slot "+itoa(target), func() bool {
		fc, err := down.ForkChoice()
		if err != nil {
			return false
		}
		head, ok := fc.HeadSlot()
		return ok && head >= target && fc.Finalized.Slot >= finalized
	})
	waitFinalizedAbove(t, s.Nodes, finalized)
}

// waitFinalizedAbove waits until every node has finalized a slot above slot,
// and returns the lowest slot they have finalized.
func waitFinalizedAbove(t *testing.T, nodes []*Node, slot uint64) uint64 {
	t.Helper()
	lowest := uint64(0)
	Eventually(t, slotTimeout, "finality past slot "+itoa(slot), func() bool {
		lowest = ^uint64(0)
		for _, n := range nodes {
			fc, err := n.ForkChoice()
			if err != nil || fc.Finalized.Slot <= slot {
				return false
			}
			lowest = min(lowest, fc.Finalized.Slot)
		}
		return true
	})
	return lowest
}

func headSlot(t *testing.T, n *Node) uint64 {
	t.Helper()
	fc, err := n.ForkChoice()
	if err != nil {
		t.Fatalf("%s fork choice: %v", n.ID, err)
	}
	slot, ok := fc.HeadSlot()
	if !ok {
		t.Fatalf("%s fork choice does not hold its head", n.ID)
	}
	return slot
}

func itoa(n uint64) string { return strconv.FormatUint(n, 10) }
