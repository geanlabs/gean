package forkchoice

// ForkChoice wraps a ProtoArray and VoteStore for LMD GHOST head selection.
type ForkChoice struct {
	Array *ProtoArray
	Votes *VoteStore
}

// New creates a ForkChoice initialized with an anchor block.
func New(anchorSlot uint64, anchorRoot [32]byte) *ForkChoice {
	return &ForkChoice{
		Array: NewProtoArray(anchorSlot, anchorRoot),
		Votes: NewVoteStore(),
	}
}

// OnBlock registers a new block.
func (fc *ForkChoice) OnBlock(slot uint64, root, parentRoot [32]byte) {
	fc.Array.OnBlock(slot, root, parentRoot)
}

// UpdateHead computes the LMD GHOST head using known attestations.
// Returns the head root.
func (fc *ForkChoice) UpdateHead(justifiedRoot [32]byte) [32]byte {
	deltas := ComputeDeltas(fc.Array.Len(), fc.Votes, true)
	fc.Array.ApplyScoreChanges(deltas, 0)
	return fc.Array.FindHead(justifiedRoot)
}

// UpdateSafeTarget computes the head using a 2/3 supermajority threshold.
// Uses all attestations (both known and new merged) — fromKnown=false reads LatestNew
// which at call time should contain the merged pool.
// Matches ethlambda store.rs L104: extract_latest_all_attestations()
func (fc *ForkChoice) UpdateSafeTarget(justifiedRoot [32]byte, numValidators uint64) [32]byte {
	minScore := int64((2*numValidators + 2) / 3) // ceil(2n/3)
	deltas := ComputeDeltas(fc.Array.Len(), fc.Votes, false)
	fc.Array.ApplyScoreChanges(deltas, minScore)
	return fc.Array.FindHead(justifiedRoot)
}

// Prune removes nodes below the finalized root.
func (fc *ForkChoice) Prune(finalizedRoot [32]byte) {
	fc.Array.Prune(finalizedRoot)
}

// NodeIndex returns the proto-array index for a root, or -1 if not found.
func (fc *ForkChoice) NodeIndex(root [32]byte) int {
	if idx, ok := fc.Array.indices[root]; ok {
		return idx
	}
	return -1
}
