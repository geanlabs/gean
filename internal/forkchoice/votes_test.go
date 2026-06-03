package forkchoice

import "testing"

func TestRemapIndicesAfterPrune(t *testing.T) {
	// Setup: 5 nodes [A(0), B(1), C(2), D(3), E(4)]
	fc := New(0, [32]byte{0xAA}, [32]byte{})
	fc.OnBlock(1, [32]byte{0xBB}, [32]byte{0xAA})
	fc.OnBlock(2, [32]byte{0xCC}, [32]byte{0xBB})
	fc.OnBlock(3, [32]byte{0xDD}, [32]byte{0xCC})
	fc.OnBlock(4, [32]byte{0xEE}, [32]byte{0xDD})

	if fc.array.Len() != 5 {
		t.Fatalf("expected 5 nodes, got %d", fc.array.Len())
	}

	// Validator 0 votes for D (index 3).
	fc.votes.SetKnown(0, 3, 3, nil)

	// Validator 1 votes for E (index 4).
	fc.votes.SetKnown(1, 4, 4, nil)

	// Apply votes so AppliedIndex gets set.
	deltas := ComputeDeltas(fc.array.Len(), fc.votes, true)
	fc.array.ApplyScoreChanges(deltas, 0)

	// Verify AppliedIndex before prune.
	if fc.votes.Votes[0].AppliedIndex != 3 {
		t.Fatalf("pre-prune: validator 0 AppliedIndex = %d, want 3", fc.votes.Votes[0].AppliedIndex)
	}
	if fc.votes.Votes[1].AppliedIndex != 4 {
		t.Fatalf("pre-prune: validator 1 AppliedIndex = %d, want 4", fc.votes.Votes[1].AppliedIndex)
	}

	// Prune at C (index 2). Removes A(0), B(1). C becomes index 0.
	// D: 3 -> 1, E: 4 -> 2
	fc.Prune([32]byte{0xCC})

	if fc.array.Len() != 3 {
		t.Fatalf("post-prune: expected 3 nodes, got %d", fc.array.Len())
	}

	// Verify AppliedIndex was remapped.
	v0 := fc.votes.Votes[0]
	if v0.AppliedIndex != 1 {
		t.Errorf("post-prune: validator 0 AppliedIndex = %d, want 1 (D shifted from 3)", v0.AppliedIndex)
	}
	v1 := fc.votes.Votes[1]
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
	deltas2 := ComputeDeltas(fc.array.Len(), fc.votes, true)
	fc.array.ApplyScoreChanges(deltas2, 0)

	// D (now index 1) has weight 2 (own vote + E's subtree).
	// E (now index 2) has weight 1 (own vote only).
	// These should be unchanged from pre-prune values.
	if fc.array.nodes[1].Weight != 2 {
		t.Errorf("D weight = %d, want 2 (own + E subtree)", fc.array.nodes[1].Weight)
	}
	if fc.array.nodes[2].Weight != 1 {
		t.Errorf("E weight = %d, want 1", fc.array.nodes[2].Weight)
	}
}

func TestRemapIndicesPrunedVotesInvalidated(t *testing.T) {
	fc := New(0, [32]byte{0xAA}, [32]byte{})
	fc.OnBlock(1, [32]byte{0xBB}, [32]byte{0xAA})
	fc.OnBlock(2, [32]byte{0xCC}, [32]byte{0xBB})

	// Vote for A (index 0), which will be pruned.
	fc.votes.SetKnown(0, 0, 0, nil)
	deltas := ComputeDeltas(fc.array.Len(), fc.votes, true)
	fc.array.ApplyScoreChanges(deltas, 0)

	// Prune at B (index 1). A removed.
	fc.Prune([32]byte{0xBB})

	// Vote for pruned node should be invalidated.
	v := fc.votes.Votes[0]
	if v.AppliedIndex != -1 {
		t.Errorf("pruned vote AppliedIndex = %d, want -1", v.AppliedIndex)
	}
	if v.LatestKnown != nil {
		t.Errorf("pruned vote LatestKnown should be nil, got index %d", v.LatestKnown.Index)
	}
}

func TestVoteStoreIgnoresInvalidIndices(t *testing.T) {
	vs := NewVoteStore()
	vs.SetKnown(0, -1, 1, nil)
	vs.SetNew(1, -1, 1, nil)

	if len(vs.Votes) != 0 {
		t.Fatalf("votes=%d, want 0", len(vs.Votes))
	}

	tracker := &VoteTracker{AppliedIndex: -1, LatestKnown: &VoteTarget{Index: -1, Slot: 1}}
	vs.Votes[2] = tracker
	deltas := ComputeDeltas(2, vs, true)
	if len(deltas) != 2 || deltas[0] != 0 || deltas[1] != 0 {
		t.Fatalf("deltas=%v, want zero deltas", deltas)
	}
	if tracker.AppliedIndex != -1 {
		t.Fatalf("applied index=%d, want -1", tracker.AppliedIndex)
	}
}

func TestPromoteNewToKnown(t *testing.T) {
	vs := NewVoteStore()
	vs.SetKnown(0, 1, 1, nil)
	vs.SetNew(0, 2, 2, nil)
	vs.SetNew(1, 3, 3, nil)

	vs.PromoteNewToKnown()

	v0 := vs.Votes[0]
	if v0.LatestKnown == nil || v0.LatestKnown.Index != 2 || v0.LatestNew != nil {
		t.Fatalf("validator 0 after promote = %+v, want known index 2 and no new", v0)
	}
	v1 := vs.Votes[1]
	if v1.LatestKnown == nil || v1.LatestKnown.Index != 3 || v1.LatestNew != nil {
		t.Fatalf("validator 1 after promote = %+v, want known index 3 and no new", v1)
	}
}

// The latest vote per validator is last-wins, matching the spec: a fresh vote
// always replaces the prior one, whether it is older, newer, or at the same
// slot. A validator only ever holds one latest vote, so a same-slot
// re-vote (equivocation) is counted once by construction.
func TestVoteStoreLastWins(t *testing.T) {
	vs := NewVoteStore()

	// Same-slot re-vote with a different target overwrites.
	vs.SetKnown(0, 1, 10, nil)
	vs.SetKnown(0, 2, 10, nil)
	if got := vs.Votes[0].LatestKnown; got == nil || got.Index != 2 || got.Slot != 10 {
		t.Fatalf("same-slot known re-vote=%+v, want index 2 slot 10", got)
	}

	// An older-slot vote still replaces the newer one (last-wins).
	vs.SetNew(0, 3, 12, nil)
	vs.SetNew(0, 4, 9, nil)
	if got := vs.Votes[0].LatestNew; got == nil || got.Index != 4 || got.Slot != 9 {
		t.Fatalf("older new re-vote=%+v, want index 4 slot 9", got)
	}
}
