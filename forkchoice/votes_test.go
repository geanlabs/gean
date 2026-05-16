package forkchoice

import "testing"

func TestRemapIndicesAfterPrune(t *testing.T) {
	// Setup: 5 nodes [A(0), B(1), C(2), D(3), E(4)]
	fc := New(0, [32]byte{0xAA})
	fc.OnBlock(1, [32]byte{0xBB}, [32]byte{0xAA})
	fc.OnBlock(2, [32]byte{0xCC}, [32]byte{0xBB})
	fc.OnBlock(3, [32]byte{0xDD}, [32]byte{0xCC})
	fc.OnBlock(4, [32]byte{0xEE}, [32]byte{0xDD})

	if fc.Array.Len() != 5 {
		t.Fatalf("expected 5 nodes, got %d", fc.Array.Len())
	}

	// Validator 0 votes for D (index 3).
	fc.Votes.SetKnown(0, 3, 3, nil)

	// Validator 1 votes for E (index 4).
	fc.Votes.SetKnown(1, 4, 4, nil)

	// Apply votes so AppliedIndex gets set.
	deltas := ComputeDeltas(fc.Array.Len(), fc.Votes, true)
	fc.Array.ApplyScoreChanges(deltas, 0)

	// Verify AppliedIndex before prune.
	if fc.Votes.Votes[0].AppliedIndex != 3 {
		t.Fatalf("pre-prune: validator 0 AppliedIndex = %d, want 3", fc.Votes.Votes[0].AppliedIndex)
	}
	if fc.Votes.Votes[1].AppliedIndex != 4 {
		t.Fatalf("pre-prune: validator 1 AppliedIndex = %d, want 4", fc.Votes.Votes[1].AppliedIndex)
	}

	// Prune at C (index 2). Removes A(0), B(1). C becomes index 0.
	// D: 3 -> 1, E: 4 -> 2
	fc.Prune([32]byte{0xCC})

	if fc.Array.Len() != 3 {
		t.Fatalf("post-prune: expected 3 nodes, got %d", fc.Array.Len())
	}

	// Verify AppliedIndex was remapped.
	v0 := fc.Votes.Votes[0]
	if v0.AppliedIndex != 1 {
		t.Errorf("post-prune: validator 0 AppliedIndex = %d, want 1 (D shifted from 3)", v0.AppliedIndex)
	}
	v1 := fc.Votes.Votes[1]
	if v1.AppliedIndex != 2 {
		t.Errorf("post-prune: validator 1 AppliedIndex = %d, want 2 (E shifted from 4)", v1.AppliedIndex)
	}

	// Verify LatestKnown.Index was remapped.
	if v0.LatestKnown == nil || v0.LatestKnown.Index != 1 {
		t.Errorf("post-prune: validator 0 LatestKnown.Index = %v, want 1", v0.LatestKnown)
	}
	if v1.LatestKnown == nil || v1.LatestKnown.Index != 2 {
		t.Errorf("post-prune: validator 1 LatestKnown.Index = %v, want 2", v1.LatestKnown)
	}

	// Verify weights are correct after re-applying deltas post-prune.
	deltas2 := ComputeDeltas(fc.Array.Len(), fc.Votes, true)
	fc.Array.ApplyScoreChanges(deltas2, 0)

	// D (now index 1) has weight 2 (own vote + E's subtree).
	// E (now index 2) has weight 1 (own vote only).
	// These should be unchanged from pre-prune values — no phantom inflation.
	if fc.Array.nodes[1].Weight != 2 {
		t.Errorf("D weight = %d, want 2 (own + E subtree)", fc.Array.nodes[1].Weight)
	}
	if fc.Array.nodes[2].Weight != 1 {
		t.Errorf("E weight = %d, want 1", fc.Array.nodes[2].Weight)
	}
}

func TestRemapIndicesPrunedVotesInvalidated(t *testing.T) {
	fc := New(0, [32]byte{0xAA})
	fc.OnBlock(1, [32]byte{0xBB}, [32]byte{0xAA})
	fc.OnBlock(2, [32]byte{0xCC}, [32]byte{0xBB})

	// Vote for A (index 0) — will be pruned.
	fc.Votes.SetKnown(0, 0, 0, nil)
	deltas := ComputeDeltas(fc.Array.Len(), fc.Votes, true)
	fc.Array.ApplyScoreChanges(deltas, 0)

	// Prune at B (index 1). A removed.
	fc.Prune([32]byte{0xBB})

	// Vote for pruned node should be invalidated.
	v := fc.Votes.Votes[0]
	if v.AppliedIndex != -1 {
		t.Errorf("pruned vote AppliedIndex = %d, want -1", v.AppliedIndex)
	}
	if v.LatestKnown != nil {
		t.Errorf("pruned vote LatestKnown should be nil, got index %d", v.LatestKnown.Index)
	}
}
