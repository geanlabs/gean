package node

import (
	"testing"

	"github.com/geanlabs/gean/internal/types"
)

// A block is late once it first reaches the node at or after interval 3 of its
// slot, a later sighting does not move its arrival, and arrivals older than the
// window are dropped.
func TestLateHeads(t *testing.T) {
	e := makeTestEngine()
	slotStart := e.Store.Config().GenesisTime*1000 + 10*types.MillisecondsPerSlot
	onTime, atBoundary, afterSlot := [32]byte{1}, [32]byte{2}, [32]byte{3}

	e.recordBlockArrival(onTime, 10, slotStart+lateHeadMs-1)
	e.recordBlockArrival(atBoundary, 10, slotStart+lateHeadMs)
	e.recordBlockArrival(afterSlot, 10, slotStart+types.MillisecondsPerSlot+500)
	// A re-gossip long after must not turn an on-time block late.
	e.recordBlockArrival(onTime, 10, slotStart+types.MillisecondsPerSlot*3)

	late := e.lateHeads(11)
	if late[onTime] {
		t.Errorf("block first seen %dms into its slot counted as late", lateHeadMs-1)
	}
	if !late[atBoundary] || !late[afterSlot] {
		t.Errorf("late=%v, want both the interval-3 and the after-slot block late", late)
	}

	e.lateHeads(10 + arrivalWindowSlots + 1)
	if len(e.blockArrivals) != 0 {
		t.Errorf("arrivals=%d after the window passed, want 0", len(e.blockArrivals))
	}
}
