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

// GetCanonicalAnalysis identifies canonical and non-canonical roots relative to an anchor.
// Returns (canonical, nonCanonical) where canonical[0] is the anchor root.
// Walks the proto-array tree to separate canonical from non-canonical blocks.
func (fc *ForkChoice) GetCanonicalAnalysis(anchorRoot [32]byte) (canonical, nonCanonical [][32]byte) {
	anchorIdx, ok := fc.Array.indices[anchorRoot]
	if !ok {
		return nil, nil
	}

	// Phase 1: Build canonical view by walking parent pointers from head to anchor.
	canonicalSet := make(map[[32]byte]bool)

	// Walk backwards from the last node to find canonical chain through anchor.
	// Start from the highest-index node that descends from anchor.
	for i := len(fc.Array.nodes) - 1; i >= anchorIdx; i-- {
		node := &fc.Array.nodes[i]
		// Check if this node is on the canonical path by walking up to anchor.
		if i == anchorIdx {
			canonicalSet[node.Root] = true
			break
		}
	}

	// Walk from anchor forwards: a node is canonical if its parent is canonical.
	canonicalSet[fc.Array.nodes[anchorIdx].Root] = true
	for i := anchorIdx + 1; i < len(fc.Array.nodes); i++ {
		node := &fc.Array.nodes[i]
		if node.Parent >= anchorIdx {
			parentRoot := fc.Array.nodes[node.Parent].Root
			if canonicalSet[parentRoot] {
				canonicalSet[node.Root] = true
			}
		}
	}

	// Phase 2: Segregate into canonical (at/below anchor slot) and non-canonical.
	anchorSlot := fc.Array.nodes[anchorIdx].Slot

	for i := anchorIdx; i < len(fc.Array.nodes); i++ {
		node := &fc.Array.nodes[i]
		if canonicalSet[node.Root] {
			if node.Slot <= anchorSlot {
				canonical = append(canonical, node.Root)
			}
			// Descendants above anchor slot are kept (still live)
		} else {
			nonCanonical = append(nonCanonical, node.Root)
		}
	}

	return canonical, nonCanonical
}

// GetCanonicalAncestorAtDepth returns the canonical block at depth steps back from head.
// Walks parent pointers from head backwards by depth steps.
func (fc *ForkChoice) GetCanonicalAncestorAtDepth(depth int) (root [32]byte, slot uint64, ok bool) {
	if len(fc.Array.nodes) == 0 {
		return [32]byte{}, 0, false
	}

	// Start from the last node (head) and walk back.
	idx := len(fc.Array.nodes) - 1
	remaining := depth
	if idx < remaining {
		idx = 0
		remaining = 0
	}

	for remaining > 0 && idx > 0 {
		parentIdx := fc.Array.nodes[idx].Parent
		if parentIdx < 0 {
			break
		}
		idx = parentIdx
		remaining--
	}

	node := &fc.Array.nodes[idx]
	return node.Root, node.Slot, true
}
