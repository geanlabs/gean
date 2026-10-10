package store_test

import (
	"testing"

	"github.com/geanlabs/gean/internal/storage"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

// climbFinalized is the spec's update_head derivation done literally: climb
// parent links from the head to its ancestor at the head state's finalized slot.
// DeriveFinalizedFromHead must give the same answer without the climb.
func climbFinalized(s *store.ConsensusStore, headRoot [32]byte) *types.Checkpoint {
	headState := s.GetState(headRoot)
	if headState == nil || headState.LatestFinalized == nil {
		return nil
	}
	finalizedSlot := headState.LatestFinalized.Slot
	root := headRoot
	for {
		header := s.GetBlockHeader(root)
		if header == nil || header.Slot <= finalizedSlot {
			break
		}
		if s.GetBlockHeader(header.ParentRoot) == nil {
			break
		}
		root = header.ParentRoot
	}
	if header := s.GetBlockHeader(root); header != nil && header.Slot == finalizedSlot {
		return &types.Checkpoint{Root: root, Slot: finalizedSlot}
	}
	return nil
}

func finalityRoot(b byte) [32]byte { return [32]byte{0xf0, b} }

// postStateFinalizing is a minimal post-state that round-trips through SSZ and
// carries the given finalized checkpoint.
func postStateFinalizing(slot uint64, finalized *types.Checkpoint) *types.State {
	return &types.State{
		Config:                   &types.ChainConfig{GenesisTime: 1000},
		Slot:                     slot,
		LatestBlockHeader:        &types.BlockHeader{},
		LatestJustified:          finalized,
		LatestFinalized:          finalized,
		JustifiedSlots:           types.NewBitlistSSZ(0),
		JustificationsValidators: types.NewBitlistSSZ(0),
	}
}

type finalityBlock struct {
	root, parent [32]byte
	slot         uint64
	finalized    types.Checkpoint
}

func storeFinalityBlocks(s *store.ConsensusStore, blocks []finalityBlock) {
	for _, b := range blocks {
		s.InsertBlockHeader(b.root, &types.BlockHeader{Slot: b.slot, ParentRoot: b.parent})
		fin := b.finalized
		s.InsertState(b.root, postStateFinalizing(b.slot, &fin))
	}
}

func checkpointsEqual(a, b *types.Checkpoint) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Root == b.Root && a.Slot == b.Slot
}

// Every head — canonical, forked, genesis, and a checkpoint-sync anchor whose
// finalized block was never stored — must resolve exactly as the climb does,
// both while the store remembers each state's checkpoint and after a restart
// has emptied that memory.
func TestDeriveFinalizedFromHeadMatchesClimb(t *testing.T) {
	g, a1, a2, a3, a4, a5 := finalityRoot(0), finalityRoot(1), finalityRoot(2), finalityRoot(3), finalityRoot(4), finalityRoot(5)
	b3, b4 := finalityRoot(0x13), finalityRoot(0x14)
	anchor, anchorChild, unstored := finalityRoot(0x20), finalityRoot(0x21), finalityRoot(0x2f)
	cp := func(root [32]byte, slot uint64) types.Checkpoint { return types.Checkpoint{Root: root, Slot: slot} }

	blocks := []finalityBlock{
		// Genesis carries a zero finalized root; the climb answers with genesis.
		{root: g, slot: 0, finalized: cp(types.ZeroRoot, 0)},
		{root: a1, parent: g, slot: 1, finalized: cp(g, 0)},
		{root: a2, parent: a1, slot: 2, finalized: cp(g, 0)},
		{root: a3, parent: a2, slot: 3, finalized: cp(a1, 1)},
		{root: a4, parent: a3, slot: 4, finalized: cp(a2, 2)},
		// A skipped slot between a4 and a5.
		{root: a5, parent: a4, slot: 6, finalized: cp(a3, 3)},
		// A fork off a2 whose states finalize less.
		{root: b3, parent: a2, slot: 3, finalized: cp(g, 0)},
		{root: b4, parent: b3, slot: 5, finalized: cp(a1, 1)},
		// A checkpoint-sync anchor: its parent and its finalized block are absent.
		{root: anchor, parent: finalityRoot(0x1f), slot: 20, finalized: cp(unstored, 16)},
		{root: anchorChild, parent: anchor, slot: 21, finalized: cp(unstored, 16)},
	}

	backend := storage.NewInMemoryBackend()
	live := store.NewConsensusStore(backend)
	storeFinalityBlocks(live, blocks)
	// A fresh store over the same backend has none of the in-memory checkpoints,
	// as after a restart, so it exercises the decode-on-miss path.
	restarted := store.NewConsensusStore(backend)

	for name, s := range map[string]*store.ConsensusStore{"live": live, "restarted": restarted} {
		for _, b := range blocks {
			want := climbFinalized(s, b.root)
			got := store.DeriveFinalizedFromHead(s, b.root)
			if !checkpointsEqual(got, want) {
				t.Errorf("%s head slot %d: got %+v, climb gives %+v", name, b.slot, got, want)
			}
		}
	}

	// Spot-check that the comparison covers both outcomes.
	if got := store.DeriveFinalizedFromHead(live, a5); !checkpointsEqual(got, &types.Checkpoint{Root: a3, Slot: 3}) {
		t.Fatalf("a5: got %+v, want a3 at slot 3", got)
	}
	if got := store.DeriveFinalizedFromHead(live, anchorChild); got != nil {
		t.Fatalf("anchor child: got %+v, want nil", got)
	}
}

// The derivation must not read the chain between head and finalized. Under a
// finality stall that stretch grows by a block a slot, and reading it on every
// head update is what stalled the dispatch loop.
func TestDeriveFinalizedFromHeadReadsIndependentOfGap(t *testing.T) {
	readsAt := func(gap int) int64 {
		backend := &countingBackend{Backend: storage.NewInMemoryBackend()}
		s := store.NewConsensusStore(backend)
		finalized := types.Checkpoint{Root: finalityRoot(1), Slot: 1}
		parent := finalityRoot(0)
		var head [32]byte
		for slot := uint64(1); slot <= uint64(gap)+1; slot++ {
			head = [32]byte{0xf1, byte(slot), byte(slot >> 8)}
			if slot == 1 {
				head = finalityRoot(1)
			}
			storeFinalityBlocks(s, []finalityBlock{{root: head, parent: parent, slot: slot, finalized: finalized}})
			parent = head
		}

		before := backend.reads()
		if got := store.DeriveFinalizedFromHead(s, head); !checkpointsEqual(got, &finalized) {
			t.Fatalf("gap %d: got %+v, want %+v", gap, got, finalized)
		}
		return backend.reads() - before
	}

	short, long := readsAt(10), readsAt(2000)
	if long != short {
		t.Fatalf("reads grow with the unfinalized gap: %d at gap 10, %d at gap 2000", short, long)
	}
	if short > 2 {
		t.Fatalf("derivation read %d entries, want at most the head and finalized headers", short)
	}
}
