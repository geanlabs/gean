package pending

import (
	"sync"
	"testing"

	"github.com/geanlabs/gean/internal/types"
)

func makeAtt(slot uint64) *types.SignedAttestation {
	return &types.SignedAttestation{
		Data: &types.AttestationData{Slot: slot},
	}
}

func TestAttestationBuffer_AddAndDrain(t *testing.T) {
	buf := NewAttestationBuffer(8, 64)

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

func TestAttestationBuffer_DrainMissing(t *testing.T) {
	buf := NewAttestationBuffer(8, 64)
	var head [32]byte
	if got := buf.Drain(head); got != nil {
		t.Fatalf("drain on empty key: got %v, want nil", got)
	}
}

func TestAttestationBuffer_PerRootFIFOEviction(t *testing.T) {
	buf := NewAttestationBuffer(2, 64)

	var head [32]byte
	head[0] = 0x01

	a, b, c := makeAtt(10), makeAtt(11), makeAtt(12)

	if added, dropped := buf.Add(head, a); !added || dropped != 0 {
		t.Fatalf("add a: added=%v dropped=%d", added, dropped)
	}
	if added, dropped := buf.Add(head, b); !added || dropped != 0 {
		t.Fatalf("add b: added=%v dropped=%d", added, dropped)
	}

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
	if drained[0] != b || drained[1] != c {
		t.Fatalf("FIFO order broken: got [%p %p], want [%p %p]", drained[0], drained[1], b, c)
	}

	if evicted, rejected := buf.Stats(); evicted != 1 || rejected != 0 {
		t.Fatalf("stats: evicted=%d rejected=%d, want 1,0", evicted, rejected)
	}
}

func TestAttestationBuffer_TotalCapRejection(t *testing.T) {
	buf := NewAttestationBuffer(8, 2)

	var h1, h2, h3 [32]byte
	h1[0], h2[0], h3[0] = 0x01, 0x02, 0x03

	if added, _ := buf.Add(h1, makeAtt(1)); !added {
		t.Fatalf("first add should succeed")
	}
	if added, _ := buf.Add(h2, makeAtt(2)); !added {
		t.Fatalf("second add should succeed")
	}

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

	if added, dropped := buf.Add(h1, makeAtt(99)); added || dropped != 0 {
		t.Fatalf("intra-bucket add at total cap: added=%v dropped=%d, want false,0", added, dropped)
	}
}

func TestAttestationBuffer_FIFOEvictsEvenAtTotalCap(t *testing.T) {
	buf := NewAttestationBuffer(2, 2)

	var head [32]byte
	head[0] = 0x01

	a, b, c := makeAtt(10), makeAtt(11), makeAtt(12)
	buf.Add(head, a)
	buf.Add(head, b)
	if buf.Total() != 2 {
		t.Fatalf("setup: total=%d, want 2", buf.Total())
	}

	added, dropped := buf.Add(head, c)
	if !added || dropped != 1 {
		t.Fatalf("FIFO at total cap: added=%v dropped=%d, want true,1", added, dropped)
	}
	if buf.Total() != 2 {
		t.Fatalf("total after FIFO at cap: %d, want 2", buf.Total())
	}
}

func TestAttestationBuffer_PruneBelow(t *testing.T) {
	buf := NewAttestationBuffer(8, 64)

	var h1, h2 [32]byte
	h1[0], h2[0] = 0x01, 0x02

	for _, slot := range []uint64{5, 10, 15} {
		buf.Add(h1, makeAtt(slot))
	}
	for _, slot := range []uint64{3, 20} {
		buf.Add(h2, makeAtt(slot))
	}
	if buf.Total() != 5 || buf.Len() != 2 {
		t.Fatalf("setup: total=%d len=%d, want 5,2", buf.Total(), buf.Len())
	}

	removed := buf.PruneBelow(10)
	if removed != 3 {
		t.Fatalf("prune removed=%d, want 3", removed)
	}
	if buf.Total() != 2 {
		t.Fatalf("after prune: total=%d, want 2", buf.Total())
	}
	if buf.Len() != 2 {
		t.Fatalf("after prune: len=%d, want 2 (both buckets non-empty)", buf.Len())
	}

	removed = buf.PruneBelow(100)
	if removed != 2 {
		t.Fatalf("final prune removed=%d, want 2", removed)
	}
	if buf.Total() != 0 || buf.Len() != 0 {
		t.Fatalf("after final prune: total=%d len=%d, want 0,0", buf.Total(), buf.Len())
	}
}

func TestAttestationBuffer_RejectsMalformedAndInvalidCaps(t *testing.T) {
	var head [32]byte

	buf := NewAttestationBuffer(0, 64)
	if added, dropped := buf.Add(head, makeAtt(1)); added || dropped != 0 {
		t.Fatalf("invalid per-root cap: added=%v dropped=%d, want false,0", added, dropped)
	}

	buf = NewAttestationBuffer(8, 0)
	if added, dropped := buf.Add(head, makeAtt(1)); added || dropped != 0 {
		t.Fatalf("invalid total cap: added=%v dropped=%d, want false,0", added, dropped)
	}

	buf = NewAttestationBuffer(8, 64)
	if added, dropped := buf.Add(head, nil); added || dropped != 0 {
		t.Fatalf("nil attestation: added=%v dropped=%d, want false,0", added, dropped)
	}
	if added, dropped := buf.Add(head, &types.SignedAttestation{}); added || dropped != 0 {
		t.Fatalf("nil data: added=%v dropped=%d, want false,0", added, dropped)
	}
	if evicted, rejected := buf.Stats(); evicted != 0 || rejected != 2 {
		t.Fatalf("stats: evicted=%d rejected=%d, want 0,2", evicted, rejected)
	}
}

func TestAttestationBuffer_NilGuards(t *testing.T) {
	var buf *AttestationBuffer
	var head [32]byte

	if added, dropped := buf.Add(head, makeAtt(1)); added || dropped != 0 {
		t.Fatalf("nil add: added=%v dropped=%d, want false,0", added, dropped)
	}
	if drained := buf.Drain(head); drained != nil {
		t.Fatalf("nil drain=%v, want nil", drained)
	}
	if removed := buf.PruneBelow(10); removed != 0 {
		t.Fatalf("nil prune removed=%d, want 0", removed)
	}
	if total := buf.Total(); total != 0 {
		t.Fatalf("nil total=%d, want 0", total)
	}
	if length := buf.Len(); length != 0 {
		t.Fatalf("nil len=%d, want 0", length)
	}
	if evicted, rejected := buf.Stats(); evicted != 0 || rejected != 0 {
		t.Fatalf("nil stats=%d/%d, want 0/0", evicted, rejected)
	}
}

func TestAttestationBuffer_PruneBelowDropsMalformedEntries(t *testing.T) {
	buf := NewAttestationBuffer(8, 64)
	var head [32]byte

	buf.byHead[head] = []*types.SignedAttestation{
		nil,
		{},
		makeAtt(5),
		makeAtt(20),
	}
	buf.total = 4

	if removed := buf.PruneBelow(10); removed != 3 {
		t.Fatalf("removed=%d, want 3", removed)
	}
	drained := buf.Drain(head)
	if len(drained) != 1 || drained[0].Data.Slot != 20 {
		t.Fatalf("drained=%v, want only slot 20", drained)
	}
}

func TestAttestationBuffer_Concurrent(t *testing.T) {
	buf := NewAttestationBuffer(16, 1024)

	const writers = 8
	const drainers = 4
	const pruners = 2
	const opsPerGoroutine = 500

	var wg sync.WaitGroup

	wg.Add(writers)
	for i := range writers {
		i := i
		go func() {
			defer wg.Done()
			for j := range opsPerGoroutine {
				var root [32]byte
				root[0] = byte(j % 4)
				buf.Add(root, makeAtt(uint64(i*1000+j)))
			}
		}()
	}

	wg.Add(drainers)
	for range drainers {
		go func() {
			defer wg.Done()
			for j := range opsPerGoroutine {
				var root [32]byte
				root[0] = byte(j % 4)
				_ = buf.Drain(root)
			}
		}()
	}

	wg.Add(pruners)
	for range pruners {
		go func() {
			defer wg.Done()
			for j := range opsPerGoroutine {
				_ = buf.PruneBelow(uint64(j))
			}
		}()
	}

	wg.Wait()

	for r := range 4 {
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
