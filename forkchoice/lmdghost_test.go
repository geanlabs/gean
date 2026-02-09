package forkchoice

import (
	"testing"

	"github.com/devylongs/gean/types"
)

// makeRoot creates a deterministic root from a byte value.
func makeRoot(b byte) types.Root {
	var r types.Root
	r[0] = b
	return r
}

// makeBlocks builds a simple block map with parent links.
// Each entry is {root byte, slot, parent root byte}. A parent of 0 means zero root.
func makeBlocks(entries [][3]byte) map[types.Root]*types.Block {
	blocks := make(map[types.Root]*types.Block)
	for _, e := range entries {
		root := makeRoot(e[0])
		parent := types.Root{}
		if e[2] != 0 {
			parent = makeRoot(e[2])
		}
		blocks[root] = &types.Block{
			Slot:       types.Slot(e[1]),
			ParentRoot: parent,
		}
	}
	return blocks
}

func TestGetHead_LinearChain(t *testing.T) {
	// Chain: A(slot=0) -> B(slot=1) -> C(slot=2)
	blocks := makeBlocks([][3]byte{
		{1, 0, 0}, // A at slot 0, no parent
		{2, 1, 1}, // B at slot 1, parent A
		{3, 2, 2}, // C at slot 2, parent B
	})

	// One vote for C
	votes := []types.Checkpoint{
		{Root: makeRoot(3), Slot: 2},
	}

	head := GetHead(blocks, makeRoot(1), votes, 0)
	if head != makeRoot(3) {
		t.Errorf("head = %x, want root 3 (tip of chain)", head[:4])
	}
}

func TestGetHead_Fork_MajorityWins(t *testing.T) {
	// Fork:
	//   A(slot=0) -> B(slot=1)
	//   A(slot=0) -> C(slot=1)
	// Two votes for B, one for C => B wins
	blocks := makeBlocks([][3]byte{
		{1, 0, 0}, // A
		{2, 1, 1}, // B, parent A
		{3, 1, 1}, // C, parent A
	})

	votes := []types.Checkpoint{
		{Root: makeRoot(2), Slot: 1}, // vote for B
		{Root: makeRoot(2), Slot: 1}, // vote for B
		{Root: makeRoot(3), Slot: 1}, // vote for C
	}

	head := GetHead(blocks, makeRoot(1), votes, 0)
	if head != makeRoot(2) {
		t.Errorf("head = %x, want root 2 (majority votes)", head[:4])
	}
}

func TestGetHead_Tiebreak_HigherSlot(t *testing.T) {
	// Fork: A -> B(slot=1), A -> C(slot=2)
	// One vote each => C wins (higher slot)
	blocks := makeBlocks([][3]byte{
		{1, 0, 0},
		{2, 1, 1},
		{3, 2, 1},
	})

	votes := []types.Checkpoint{
		{Root: makeRoot(2), Slot: 1},
		{Root: makeRoot(3), Slot: 2},
	}

	head := GetHead(blocks, makeRoot(1), votes, 0)
	if head != makeRoot(3) {
		t.Errorf("head = %x, want root 3 (higher slot tiebreak)", head[:4])
	}
}

func TestGetHead_Tiebreak_HigherHash(t *testing.T) {
	// Fork: A -> B(slot=1), A -> C(slot=1)
	// One vote each, same slot => higher hash wins
	// Root 3 > Root 2 lexicographically
	blocks := makeBlocks([][3]byte{
		{1, 0, 0},
		{2, 1, 1},
		{3, 1, 1},
	})

	votes := []types.Checkpoint{
		{Root: makeRoot(2), Slot: 1},
		{Root: makeRoot(3), Slot: 1},
	}

	head := GetHead(blocks, makeRoot(1), votes, 0)
	if head != makeRoot(3) {
		t.Errorf("head = %x, want root 3 (higher hash tiebreak)", head[:4])
	}
}

func TestGetHead_MinScore(t *testing.T) {
	// A -> B -> C, only 1 vote for C, minScore = 2
	// B and C have fewer than 2 votes, so they shouldn't be followed
	blocks := makeBlocks([][3]byte{
		{1, 0, 0},
		{2, 1, 1},
		{3, 2, 2},
	})

	votes := []types.Checkpoint{
		{Root: makeRoot(3), Slot: 2},
	}

	head := GetHead(blocks, makeRoot(1), votes, 2)
	if head != makeRoot(1) {
		t.Errorf("head = %x, want root 1 (no children meet minScore)", head[:4])
	}
}

func TestGetHead_NoVotes(t *testing.T) {
	// A -> B -> C, no votes => still walk to leaf via children (minScore=0 includes all)
	blocks := makeBlocks([][3]byte{
		{1, 0, 0},
		{2, 1, 1},
		{3, 2, 2},
	})

	head := GetHead(blocks, makeRoot(1), nil, 0)
	// With minScore=0, all children are included. Should reach C (slot 2).
	if head != makeRoot(3) {
		t.Errorf("head = %x, want root 3 (walk to leaf)", head[:4])
	}
}

func TestGetLatestJustified_FindsHighest(t *testing.T) {
	states := map[types.Root]*types.State{
		makeRoot(1): {LatestJustified: types.Checkpoint{Root: makeRoot(10), Slot: 2}},
		makeRoot(2): {LatestJustified: types.Checkpoint{Root: makeRoot(20), Slot: 5}},
		makeRoot(3): {LatestJustified: types.Checkpoint{Root: makeRoot(30), Slot: 3}},
	}

	latest := GetLatestJustified(states)
	if latest == nil {
		t.Fatal("expected non-nil checkpoint")
	}
	if latest.Slot != 5 {
		t.Errorf("latest justified slot = %d, want 5", latest.Slot)
	}
	if latest.Root != makeRoot(20) {
		t.Errorf("latest justified root = %x, want root 20", latest.Root[:4])
	}
}

func TestGetLatestJustified_EmptyStates(t *testing.T) {
	latest := GetLatestJustified(nil)
	if latest != nil {
		t.Error("expected nil for empty states map")
	}

	latest = GetLatestJustified(map[types.Root]*types.State{})
	if latest != nil {
		t.Error("expected nil for empty states map")
	}
}
