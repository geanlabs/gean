package node

import (
	"testing"

	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

// makeChainStore builds a store with a chain whose blocks fall on the given
// slots. Slot 0 is always genesis. Returns the store and a slot→root map for
// assertions.
func makeChainStore(t *testing.T, slots []uint64) (*store.ConsensusStore, map[uint64][32]byte) {
	t.Helper()
	s := makeTestStore()

	roots := make(map[uint64][32]byte)
	var prevRoot [32]byte
	for _, slot := range slots {
		var root [32]byte
		root[0] = byte(slot + 1) // distinct, non-zero
		signed := &types.SignedBlock{
			Block: &types.Block{
				Slot:       slot,
				ParentRoot: prevRoot,
				Body:       &types.BlockBody{},
			},
		}
		s.StorePendingBlock(root, signed)
		roots[slot] = root
		prevRoot = root
	}
	if len(slots) > 0 {
		s.SetHead(roots[slots[len(slots)-1]])
	}
	return s, roots
}

// TestGetCanonicalBlocksInRange_FullChain — range covers the entire chain.
func TestGetCanonicalBlocksInRange_FullChain(t *testing.T) {
	s, roots := makeChainStore(t, []uint64{0, 2, 5, 7})

	got := s.GetCanonicalBlocksInRange(0, 8)
	if len(got) != 4 {
		t.Fatalf("expected 4 blocks, got %d", len(got))
	}
	wantSlots := []uint64{0, 2, 5, 7}
	for i, b := range got {
		if b.Block.Slot != wantSlots[i] {
			t.Errorf("blocks[%d].Slot = %d, want %d", i, b.Block.Slot, wantSlots[i])
		}
	}
	_ = roots
}

// TestGetCanonicalBlocksInRange_ZeroCount — count=0 returns nil.
func TestGetCanonicalBlocksInRange_ZeroCount(t *testing.T) {
	s, _ := makeChainStore(t, []uint64{0, 1, 2})
	if got := s.GetCanonicalBlocksInRange(0, 0); got != nil {
		t.Fatalf("expected nil for count=0, got %d blocks", len(got))
	}
}

// TestGetCanonicalBlocksInRange_SkipsEmptySlots — range that includes empty
// slots (no block produced) returns only the slots that actually have blocks.
func TestGetCanonicalBlocksInRange_SkipsEmptySlots(t *testing.T) {
	// Chain at slots 0, 2, 5, 7. Slots 1, 3, 4, 6 are empty.
	s, _ := makeChainStore(t, []uint64{0, 2, 5, 7})

	// [3, 7) covers slots 3, 4, 5, 6. Only slot 5 has a block.
	got := s.GetCanonicalBlocksInRange(3, 4)
	if len(got) != 1 || got[0].Block.Slot != 5 {
		t.Fatalf("expected single block at slot 5, got %d blocks", len(got))
	}

	// [2, 6) covers slots 2, 3, 4, 5. Blocks at 2 and 5.
	got = s.GetCanonicalBlocksInRange(2, 4)
	if len(got) != 2 || got[0].Block.Slot != 2 || got[1].Block.Slot != 5 {
		t.Fatalf("expected blocks at [2, 5], got %d blocks", len(got))
	}
}

// TestGetCanonicalBlocksInRange_AboveHead — range above head returns nothing.
func TestGetCanonicalBlocksInRange_AboveHead(t *testing.T) {
	s, _ := makeChainStore(t, []uint64{0, 2, 5, 7})

	got := s.GetCanonicalBlocksInRange(10, 5)
	if got != nil {
		t.Fatalf("expected nil for range above head, got %d blocks", len(got))
	}
}

// TestGetCanonicalBlocksInRange_AscendingOrder — output must be ascending by slot.
func TestGetCanonicalBlocksInRange_AscendingOrder(t *testing.T) {
	s, _ := makeChainStore(t, []uint64{0, 1, 2, 3, 4, 5})

	got := s.GetCanonicalBlocksInRange(1, 4)
	if len(got) != 4 {
		t.Fatalf("expected 4 blocks, got %d", len(got))
	}
	for i := 1; i < len(got); i++ {
		if got[i].Block.Slot <= got[i-1].Block.Slot {
			t.Fatalf("non-ascending slots at index %d: %d <= %d",
				i, got[i].Block.Slot, got[i-1].Block.Slot)
		}
	}
}

// TestGetCanonicalBlocksInRange_PartialFromGenesis — typical bulk-sync request.
func TestGetCanonicalBlocksInRange_PartialFromGenesis(t *testing.T) {
	s, _ := makeChainStore(t, []uint64{0, 1, 2, 3, 4, 5, 6, 7})

	// Request first 3 blocks.
	got := s.GetCanonicalBlocksInRange(0, 3)
	if len(got) != 3 {
		t.Fatalf("expected 3 blocks, got %d", len(got))
	}
	for i, b := range got {
		if b.Block.Slot != uint64(i) {
			t.Errorf("blocks[%d].Slot = %d, want %d", i, b.Block.Slot, i)
		}
	}
}
