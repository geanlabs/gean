package aggregation

import (
	"github.com/geanlabs/gean/internal/metrics"
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

// A group whose target is already justified is kept only when its head arrived
// late: those votes decide the fork that a late block leaves behind. It ranks
// after every open target, even a backlog one, so a capped session spends its
// budget on what can still be justified. With an on-time head it is skipped,
// since its votes repeat a head the network already agrees on.
func TestOrderedGroupsKeepsSettledTargetsOnlyForLateHeads(t *testing.T) {
	const finalized = uint64(100)
	const settledTarget = uint64(105)
	const openTarget = uint64(103)
	lateHead := [32]byte{0xaa}
	onTimeHead := [32]byte{0xbb}

	justifiedSlots := types.BitlistExtend(nil, 10)
	types.BitlistSet(justifiedSlots, settledTarget-finalized-1)
	settled := func(head [32]byte) *store.AttestationDataEntry {
		return &store.AttestationDataEntry{Data: &types.AttestationData{
			Slot:   settledTarget,
			Head:   &types.Checkpoint{Root: head, Slot: settledTarget},
			Target: &types.Checkpoint{Slot: settledTarget},
		}}
	}
	snap := &Snapshot{
		slot: settledTarget,
		headState: &types.State{
			LatestFinalized: &types.Checkpoint{Slot: finalized},
			JustifiedSlots:  justifiedSlots,
		},
		lateHeads: map[[32]byte]bool{lateHead: true},
		attSigs: map[[32]byte]*store.AttestationDataEntry{
			rootByte(1): settled(lateHead),
			rootByte(2): {Data: &types.AttestationData{Slot: openTarget, Target: &types.Checkpoint{Slot: openTarget}}},
			rootByte(3): settled(onTimeHead),
		},
	}

	skips := groupSkips{}
	ordered := orderedGroups(snap, skips)

	if len(ordered) != 2 {
		t.Fatalf("groups=%d, want 2 (open target, settled target with a late head)", len(ordered))
	}
	if ordered[0].dataRoot != rootByte(2) || ordered[0].settled {
		t.Errorf("first group=%x settled=%v, want the open target first", ordered[0].dataRoot[:1], ordered[0].settled)
	}
	if ordered[1].dataRoot != rootByte(1) || !ordered[1].settled {
		t.Errorf("last group=%x settled=%v, want the late-head settled target last", ordered[1].dataRoot[:1], ordered[1].settled)
	}
	if got := skips[metrics.AggGroupSkipTargetJustified]; got != 1 {
		t.Errorf("target_justified skips=%d, want 1 (the on-time head)", got)
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
