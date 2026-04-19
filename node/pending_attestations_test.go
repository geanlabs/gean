package node

import (
	"sync"
	"testing"

	"github.com/geanlabs/gean/types"
)

// makeAtt builds a minimal SignedAttestation with the given slot. Other
// fields are zero-valued; the buffer only inspects Data.Slot for pruning.
func makeAtt(slot uint64) *types.SignedAttestation {
	return &types.SignedAttestation{
		Data: &types.AttestationData{Slot: slot},
	}
}

func TestPendingAttestationBuffer_AddAndDrain(t *testing.T) {
	buf := NewPendingAttestationBuffer(8, 64)

	var head [32]byte
	head[0] = 0x01

	att := makeAtt(10)
	added, dropped := buf.Add(head, att)
	if !added || dropped != 0 {
		t.Fatalf("first add: added=%v dropped=%d, want true,0", added, dropped)
	}
	if buf.Total() != 1 || buf.Len() != 1 {
		t.Fatalf("after add: total=%d len=%d, want 1,1", buf.Total(), buf.Len())
	}

	drained := buf.Drain(head)
	if len(drained) != 1 || drained[0] != att {
		t.Fatalf("drain: got %d entries, want 1 matching pointer", len(drained))
	}
	if buf.Total() != 0 || buf.Len() != 0 {
		t.Fatalf("after drain: total=%d len=%d, want 0,0", buf.Total(), buf.Len())
	}
}

func TestPendingAttestationBuffer_DrainMissing(t *testing.T) {
	buf := NewPendingAttestationBuffer(8, 64)
	var head [32]byte
	if got := buf.Drain(head); got != nil {
		t.Fatalf("drain on empty key: got %v, want nil", got)
	}
}

func TestPendingAttestationBuffer_PerRootFIFOEviction(t *testing.T) {
	buf := NewPendingAttestationBuffer(2, 64)

	var head [32]byte
	head[0] = 0x01

	a, b, c := makeAtt(10), makeAtt(11), makeAtt(12)

	// Fill bucket to per-root cap.
	if added, dropped := buf.Add(head, a); !added || dropped != 0 {
		t.Fatalf("add a: added=%v dropped=%d", added, dropped)
	}
	if added, dropped := buf.Add(head, b); !added || dropped != 0 {
		t.Fatalf("add b: added=%v dropped=%d", added, dropped)
	}

	// Third add overflows: oldest (a) evicted, total stays at 2.
	added, dropped := buf.Add(head, c)
	if !added || dropped != 1 {
		t.Fatalf("add c: added=%v dropped=%d, want true,1", added, dropped)
	}
	if buf.Total() != 2 {
		t.Fatalf("total after FIFO eviction: %d, want 2", buf.Total())
	}

	drained := buf.Drain(head)
	if len(drained) != 2 {
		t.Fatalf("drain: %d entries, want 2", len(drained))
	}
	// FIFO: oldest evicted, so we expect b then c, not a.
	if drained[0] != b || drained[1] != c {
		t.Fatalf("FIFO order broken: got [%p %p], want [%p %p]", drained[0], drained[1], b, c)
	}

	if evicted, rejected := buf.Stats(); evicted != 1 || rejected != 0 {
		t.Fatalf("stats: evicted=%d rejected=%d, want 1,0", evicted, rejected)
	}
}

func TestPendingAttestationBuffer_TotalCapRejection(t *testing.T) {
	buf := NewPendingAttestationBuffer(8, 2)

	var h1, h2, h3 [32]byte
	h1[0], h2[0], h3[0] = 0x01, 0x02, 0x03

	if added, _ := buf.Add(h1, makeAtt(1)); !added {
		t.Fatalf("first add should succeed")
	}
	if added, _ := buf.Add(h2, makeAtt(2)); !added {
		t.Fatalf("second add should succeed")
	}
	// Third distinct-root add hits totalCap=2 → reject.
	added, dropped := buf.Add(h3, makeAtt(3))
	if added || dropped != 0 {
		t.Fatalf("third add (over total cap): added=%v dropped=%d, want false,0", added, dropped)
	}
	if buf.Total() != 2 || buf.Len() != 2 {
		t.Fatalf("after reject: total=%d len=%d, want 2,2", buf.Total(), buf.Len())
	}
	if _, rejected := buf.Stats(); rejected != 1 {
		t.Fatalf("rejected count: got %d, want 1", rejected)
	}

	// Total cap is hard: an intra-bucket add that does NOT trigger FIFO
	// eviction is rejected when total == totalCap. (h1 has 1 entry,
	// perRootCap=8, so we'd otherwise just append — but total is at cap.)
	if added, dropped := buf.Add(h1, makeAtt(99)); added || dropped != 0 {
		t.Fatalf("intra-bucket add at total cap: added=%v dropped=%d, want false,0", added, dropped)
	}
}

// TestPendingAttestationBuffer_FIFOEvictsEvenAtTotalCap verifies that
// per-root FIFO eviction is permitted even when the buffer is at totalCap,
// because the eviction swaps one entry for another and does not grow total.
func TestPendingAttestationBuffer_FIFOEvictsEvenAtTotalCap(t *testing.T) {
	// perRootCap=2, totalCap=2 — a single bucket can saturate both caps.
	buf := NewPendingAttestationBuffer(2, 2)

	var head [32]byte
	head[0] = 0x01

	a, b, c := makeAtt(10), makeAtt(11), makeAtt(12)
	buf.Add(head, a)
	buf.Add(head, b)
	if buf.Total() != 2 {
		t.Fatalf("setup: total=%d, want 2", buf.Total())
	}

	// Bucket is at perRootCap AND total is at totalCap. FIFO eviction must
	// still succeed (one in, one out — total unchanged).
	added, dropped := buf.Add(head, c)
	if !added || dropped != 1 {
		t.Fatalf("FIFO at total cap: added=%v dropped=%d, want true,1", added, dropped)
	}
	if buf.Total() != 2 {
		t.Fatalf("total after FIFO at cap: %d, want 2", buf.Total())
	}
}

func TestPendingAttestationBuffer_PruneBelow(t *testing.T) {
	buf := NewPendingAttestationBuffer(8, 64)

	var h1, h2 [32]byte
	h1[0], h2[0] = 0x01, 0x02

	// h1: slots 5, 10, 15. h2: slots 3, 20.
	for _, slot := range []uint64{5, 10, 15} {
		buf.Add(h1, makeAtt(slot))
	}
	for _, slot := range []uint64{3, 20} {
		buf.Add(h2, makeAtt(slot))
	}
	if buf.Total() != 5 || buf.Len() != 2 {
		t.Fatalf("setup: total=%d len=%d, want 5,2", buf.Total(), buf.Len())
	}

	// Prune at slot 10: drops slot<=10 → 5, 10, 3. Keeps 15, 20.
	removed := buf.PruneBelow(10)
	if removed != 3 {
		t.Fatalf("prune removed=%d, want 3", removed)
	}
	if buf.Total() != 2 {
		t.Fatalf("after prune: total=%d, want 2", buf.Total())
	}
	// h2's only surviving entry stays under h2; h1's only surviving entry stays under h1.
	if buf.Len() != 2 {
		t.Fatalf("after prune: len=%d, want 2 (both buckets non-empty)", buf.Len())
	}

	// Prune everything → empty buckets removed.
	removed = buf.PruneBelow(100)
	if removed != 2 {
		t.Fatalf("final prune removed=%d, want 2", removed)
	}
	if buf.Total() != 0 || buf.Len() != 0 {
		t.Fatalf("after final prune: total=%d len=%d, want 0,0", buf.Total(), buf.Len())
	}
}

// TestPendingAttestationBuffer_Concurrent exercises the buffer under
// concurrent Add/Drain/PruneBelow load. Run with -race to catch any locking
// regression. We don't assert exact counts because Drain races with Add by
// design — we only assert that the buffer's accounting stays consistent
// (total == sum-of-bucket-lengths) at the end.
func TestPendingAttestationBuffer_Concurrent(t *testing.T) {
	buf := NewPendingAttestationBuffer(16, 1024)

	const writers = 8
	const drainers = 4
	const pruners = 2
	const opsPerGoroutine = 500

	var wg sync.WaitGroup

	// Writers each pick from a small set of head roots so buckets fill.
	wg.Add(writers)
	for i := 0; i < writers; i++ {
		i := i
		go func() {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				var root [32]byte
				root[0] = byte(j % 4) // 4 distinct head roots, contention guaranteed
				buf.Add(root, makeAtt(uint64(i*1000+j)))
			}
		}()
	}

	// Drainers race against writers on the same root namespace.
	wg.Add(drainers)
	for i := 0; i < drainers; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				var root [32]byte
				root[0] = byte(j % 4)
				_ = buf.Drain(root)
			}
		}()
	}

	// Pruners cull below an advancing slot.
	wg.Add(pruners)
	for i := 0; i < pruners; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				_ = buf.PruneBelow(uint64(j))
			}
		}()
	}

	wg.Wait()

	// Final consistency: drain every known head root and confirm Total
	// drops to 0. If the internal counter ever drifted from actual bucket
	// contents under -race, this would surface as a non-zero residual.
	for r := 0; r < 4; r++ {
		var root [32]byte
		root[0] = byte(r)
		_ = buf.Drain(root)
	}
	if buf.Total() != 0 {
		t.Fatalf("after universal drain: total=%d, want 0", buf.Total())
	}
	if buf.Len() != 0 {
		t.Fatalf("after universal drain: len=%d, want 0", buf.Len())
	}
}
