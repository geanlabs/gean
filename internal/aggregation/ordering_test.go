package aggregation

import (
	"testing"

	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

func rootByte(b byte) [32]byte {
	var r [32]byte
	r[0] = b
	return r
}

// snapshotWithTargets builds a snapshot whose only content is one attestation
// group per entry, each voting for the given target slot.
func snapshotWithTargets(targets map[byte]uint64) *Snapshot {
	attSigs := make(map[[32]byte]*store.AttestationDataEntry, len(targets))
	for id, targetSlot := range targets {
		attSigs[rootByte(id)] = &store.AttestationDataEntry{
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
	ordered := orderedGroups(snap)

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

	ordered := orderedGroups(snap)

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

// TestOrderedGroupsRanksJustifiedTargetsLast: a vote for a justified target can
// not advance finality but still carries head weight, so its group is kept
// behind every target that can, including an older backlog one.
func TestOrderedGroupsRanksJustifiedTargetsLast(t *testing.T) {
	const finalized = uint64(100)
	const justifiedTarget = uint64(105)
	const backlogTarget = uint64(103)
	const openTarget = uint64(106)

	justifiedSlots := types.BitlistExtend(nil, 10)
	types.BitlistSet(justifiedSlots, justifiedTarget-finalized-1) // mark slot 105 justified
	snap := &Snapshot{
		slot: justifiedTarget,
		headState: &types.State{
			LatestFinalized: &types.Checkpoint{Slot: finalized},
			JustifiedSlots:  justifiedSlots,
		},
		attSigs: map[[32]byte]*store.AttestationDataEntry{
			rootByte(1): {Data: &types.AttestationData{Slot: justifiedTarget, Target: &types.Checkpoint{Slot: justifiedTarget}}},
			rootByte(2): {Data: &types.AttestationData{Slot: openTarget, Target: &types.Checkpoint{Slot: openTarget}}},
			rootByte(3): {Data: &types.AttestationData{Slot: backlogTarget, Target: &types.Checkpoint{Slot: backlogTarget}}},
		},
	}

	ordered := orderedGroups(snap)
	if len(ordered) != 3 {
		t.Fatalf("groups=%d, want 3 (justified target kept)", len(ordered))
	}
	if ordered[2].targetSlot != justifiedTarget || !ordered[2].settled {
		t.Fatalf("last group target=%d settled=%v, want the justified target %d", ordered[2].targetSlot, ordered[2].settled, justifiedTarget)
	}
}

// TestOrderedGroupsDeterministicTiebreak: equal target slots break ties by data
// root, so the order is stable across snapshots regardless of map iteration.
func TestOrderedGroupsDeterministicTiebreak(t *testing.T) {
	snap := snapshotWithTargets(map[byte]uint64{9: 50, 3: 50, 7: 50})
	first := orderedGroups(snap)
	for range 5 {
		again := orderedGroups(snap)
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
