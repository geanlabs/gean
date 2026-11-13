package aggregation

import (
	"testing"

	"github.com/geanlabs/gean/lean/consensus/types"
	"github.com/geanlabs/gean/lean/internal/pools"
)

func rootByte(b byte) [32]byte {
	var r [32]byte
	r[0] = b
	return r
}

// snapshotWithTargets builds a snapshot whose only content is one attestation
// group per entry, each voting for the given target slot.
func snapshotWithTargets(targets map[byte]uint64) *Snapshot {
	attSigs := make(map[[32]byte]*pools.AttestationDataEntry, len(targets))
	for id, targetSlot := range targets {
		attSigs[rootByte(id)] = &pools.AttestationDataEntry{
			Data: &types.AttestationData{
				Slot:   targetSlot,
				Target: &types.Checkpoint{Slot: targetSlot},
			},
		}
	}
	return &Snapshot{attSigs: attSigs}
}

// TestOrderedGroupsFrontierFirst: groups come out by ascending target slot, so
// the votes nearest the finalized frontier are proven before the head-most ones.
func TestOrderedGroupsFrontierFirst(t *testing.T) {
	snap := snapshotWithTargets(map[byte]uint64{1: 104, 2: 100, 3: 102, 4: 101, 5: 103})
	ordered := orderedGroups(snap, groupSkips{})

	want := []uint64{100, 101, 102, 103, 104}
	if len(ordered) != len(want) {
		t.Fatalf("group count=%d, want %d", len(ordered), len(want))
	}
	for i, g := range ordered {
		if g.targetSlot != want[i] {
			t.Fatalf("position %d targetSlot=%d, want %d", i, g.targetSlot, want[i])
		}
	}
}

// TestTruncatingAggregatorKeepsFrontier: with a session budget that covers only
// the first few groups, frontier-first keeps the finalization-frontier votes and
// yields the head-most ones. The former newest-first order did the inverse —
// spending the budget on the head while the frontier starved, which advances the
// head but stalls finalization (the observed stall shape).
func TestTruncatingAggregatorKeepsFrontier(t *testing.T) {
	const frontier = uint64(100)
	const headMost = uint64(104)
	snap := snapshotWithTargets(map[byte]uint64{1: frontier, 2: 101, 3: 102, 4: 103, 5: headMost})

	ordered := orderedGroups(snap, groupSkips{})

	// A budget that fits only the first two proofs.
	const budget = 2
	proven := make(map[uint64]bool)
	for _, g := range ordered[:budget] {
		proven[g.targetSlot] = true
	}

	if !proven[frontier] {
		t.Fatalf("frontier target %d dropped under truncation", frontier)
	}
	if proven[headMost] {
		t.Fatal("head-most target proven before the frontier under truncation")
	}
}

// TestOrderedGroupsRanksJustifiedTargets: a vote for a justified target cannot
// advance finality but still carries head weight, so its group is kept. It follows
// every open target, even backlog, so it cannot crowd one out of a capped session.
func TestOrderedGroupsRanksJustifiedTargets(t *testing.T) {
	const finalized = uint64(100)
	const currentSlot = uint64(110)
	justifiedSlots := types.BitlistExtend(nil, 10)
	for _, slot := range []uint64{102, 104, 105} {
		types.BitlistSet(justifiedSlots, slot-finalized-1)
	}
	group := func(slot, target uint64) *pools.AttestationDataEntry {
		return &pools.AttestationDataEntry{Data: &types.AttestationData{Slot: slot, Target: &types.Checkpoint{Slot: target}}}
	}
	snap := &Snapshot{
		slot: currentSlot,
		headState: &types.State{
			LatestFinalized: &types.Checkpoint{Slot: finalized},
			JustifiedSlots:  justifiedSlots,
		},
		attSigs: map[[32]byte]*pools.AttestationDataEntry{
			rootByte(1): group(currentSlot, 105), // current, settled
			rootByte(2): group(currentSlot, 106), // current, open
			rootByte(3): group(107, 103),         // backlog, open
			rootByte(4): group(107, 102),         // backlog, settled
			rootByte(5): group(107, 104),         // backlog, settled, newer target
		},
	}

	ordered := orderedGroups(snap, groupSkips{})
	want := []byte{2, 3, 1, 5, 4}
	if len(ordered) != len(want) {
		t.Fatalf("groups=%d, want %d (justified targets kept)", len(ordered), len(want))
	}
	for i, root := range want {
		if ordered[i].dataRoot != rootByte(root) {
			t.Fatalf("position %d is group %d, want %d", i, ordered[i].dataRoot[0], root)
		}
	}
}

// TestOrderedGroupsDeterministicTiebreak: equal target slots break ties by data
// root, so the order is stable across snapshots regardless of map iteration.
func TestOrderedGroupsDeterministicTiebreak(t *testing.T) {
	snap := snapshotWithTargets(map[byte]uint64{9: 50, 3: 50, 7: 50})
	first := orderedGroups(snap, groupSkips{})
	for range 5 {
		again := orderedGroups(snap, groupSkips{})
		for i := range first {
			if first[i].dataRoot != again[i].dataRoot {
				t.Fatalf("non-deterministic order at %d", i)
			}
		}
	}
	// All equal target; must be ascending by data root.
	for i := 1; i < len(first); i++ {
		if first[i-1].dataRoot[0] > first[i].dataRoot[0] {
			t.Fatalf("tiebreak not ascending by data root: %d before %d", first[i-1].dataRoot[0], first[i].dataRoot[0])
		}
	}
}
