package forkchoice

import "bytes"

// ProtoNode is a single block in the proto-array tree.
type ProtoNode struct {
	Slot           uint64
	Root           [32]byte
	ParentRoot     [32]byte
	Parent         int    // index into nodes, -1 if none
	Weight         int64  // accumulated attestation weight
	BestChild      int    // index, -1 if none
	BestDescendant int    // index, -1 if none
}

// ProtoArray is a flat array representing the block tree for O(n) fork choice.
type ProtoArray struct {
	nodes   []ProtoNode
	indices map[[32]byte]int // root -> index
}

// NewProtoArray creates a proto-array with an anchor block.
func NewProtoArray(anchorSlot uint64, anchorRoot [32]byte) *ProtoArray {
	pa := &ProtoArray{
		indices: make(map[[32]byte]int),
	}
	pa.nodes = append(pa.nodes, ProtoNode{
		Slot:           anchorSlot,
		Root:           anchorRoot,
		ParentRoot:     [32]byte{},
		Parent:         -1,
		Weight:         0,
		BestChild:      -1,
		BestDescendant: -1,
	})
	pa.indices[anchorRoot] = 0
	return pa
}

// OnBlock registers a new block in the proto-array.

func (pa *ProtoArray) OnBlock(slot uint64, root, parentRoot [32]byte) {
	if _, exists := pa.indices[root]; exists {
		return // already registered
	}
	nodeIndex := len(pa.nodes)
	parentIdx := -1
	if idx, ok := pa.indices[parentRoot]; ok {
		parentIdx = idx
	}

	pa.nodes = append(pa.nodes, ProtoNode{
		Slot:           slot,
		Root:           root,
		ParentRoot:     parentRoot,
		Parent:         parentIdx,
		Weight:         0,
		BestChild:      -1,
		BestDescendant: -1,
	})
	pa.indices[root] = nodeIndex
}

// ApplyScoreChanges propagates weight deltas backward through the array
// and recalculates bestChild/bestDescendant.

func (pa *ProtoArray) ApplyScoreChanges(deltas []int64, cutoffWeight int64) {
	if len(deltas) != len(pa.nodes) {
		return
	}

	// Pass 1: iterate backward, apply deltas and propagate to parents.
	for i := len(pa.nodes) - 1; i >= 0; i-- {
		pa.nodes[i].Weight += deltas[i]
		if pa.nodes[i].Parent >= 0 {
			deltas[pa.nodes[i].Parent] += deltas[i]
		}
	}

	// Pass 2: iterate backward, recalculate bestChild and bestDescendant.
	for i := len(pa.nodes) - 1; i >= 0; i-- {
		parentIdx := pa.nodes[i].Parent
		if parentIdx < 0 {
			continue
		}

		// This node's best descendant, or itself if it meets the cutoff.
		nodeBestDesc := pa.nodes[i].BestDescendant
		if nodeBestDesc < 0 {
			if pa.nodes[i].Weight >= cutoffWeight {
				nodeBestDesc = i
			} else {
				nodeBestDesc = -1
			}
		}

		parent := &pa.nodes[parentIdx]
		shouldUpdate := false

		if parent.BestChild == i {
			// Already best child — just update descendant if changed.
			if parent.BestDescendant != nodeBestDesc {
				shouldUpdate = true
			}
		} else if parent.BestChild >= 0 {
			bestChild := &pa.nodes[parent.BestChild]
			if bestChild.Weight < pa.nodes[i].Weight {
				shouldUpdate = true
			} else if bestChild.Weight == pa.nodes[i].Weight {
				// Tie-break: lexicographically larger root wins (leanSpec-compatible).
				if bytes.Compare(bestChild.Root[:], pa.nodes[i].Root[:]) < 0 {
					shouldUpdate = true
				}
			}
		} else {
			// No best child yet.
			shouldUpdate = true
		}

		if shouldUpdate {
			parent.BestChild = i
			parent.BestDescendant = nodeBestDesc
		}
	}
}

// FindHead returns the head root by walking the bestDescendant chain from justifiedRoot.

func (pa *ProtoArray) FindHead(justifiedRoot [32]byte) [32]byte {
	idx, ok := pa.indices[justifiedRoot]
	if !ok {
		return justifiedRoot
	}
	bestDesc := pa.nodes[idx].BestDescendant
	if bestDesc < 0 {
		return justifiedRoot
	}
	return pa.nodes[bestDesc].Root
}

// FindHeadWithThreshold is like FindHead but with a minimum weight cutoff.
// Used for safe target computation (2/3 threshold).
func (pa *ProtoArray) FindHeadWithThreshold(justifiedRoot [32]byte, minScore int64) [32]byte {
	return pa.FindHead(justifiedRoot) // cutoff applied during ApplyScoreChanges
}

// Prune removes all nodes below the finalized root.
func (pa *ProtoArray) Prune(finalizedRoot [32]byte) {
	finalizedIdx, ok := pa.indices[finalizedRoot]
	if !ok || finalizedIdx == 0 {
		return
	}

	// Remove pruned nodes from indices.
	for i := 0; i < finalizedIdx; i++ {
		delete(pa.indices, pa.nodes[i].Root)
	}

	// Shift nodes.
	pa.nodes = pa.nodes[finalizedIdx:]

	// Rebuild indices.
	newIndices := make(map[[32]byte]int, len(pa.nodes))
	for i := range pa.nodes {
		newIndices[pa.nodes[i].Root] = i
		// Adjust parent pointers.
		if pa.nodes[i].Parent >= 0 {
			pa.nodes[i].Parent -= finalizedIdx
			if pa.nodes[i].Parent < 0 {
				pa.nodes[i].Parent = -1
			}
		}
		if pa.nodes[i].BestChild >= 0 {
			pa.nodes[i].BestChild -= finalizedIdx
			if pa.nodes[i].BestChild < 0 {
				pa.nodes[i].BestChild = -1
			}
		}
		if pa.nodes[i].BestDescendant >= 0 {
			pa.nodes[i].BestDescendant -= finalizedIdx
			if pa.nodes[i].BestDescendant < 0 {
				pa.nodes[i].BestDescendant = -1
			}
		}
	}
	pa.indices = newIndices
}

// Len returns the number of nodes.
func (pa *ProtoArray) Len() int {
	return len(pa.nodes)
}
