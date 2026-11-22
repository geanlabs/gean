// Package protoarray is the proto-array: the block tree fork choice runs on.
// It holds each block's subtree weight and best descendant, so finding the head
// from a justified root is a walk down best-child links. The weights come from
// the chain's own vote accounting; this package knows nothing of a chain's rules.
package protoarray

import "bytes"

type Node struct {
	Slot           uint64
	Root           [32]byte
	ParentRoot     [32]byte
	Parent         int
	Weight         int64
	BestChild      int
	BestDescendant int
}

// Array holds a block only once it holds the block's parent, so a node's parent
// always sits at a lower index. Scoring walks the nodes last to first and pruning
// first to last on that basis.
type Array struct {
	nodes   []Node
	indices map[[32]byte]int
}

func New(anchorSlot uint64, anchorRoot, anchorParentRoot [32]byte) *Array {
	pa := &Array{
		indices: make(map[[32]byte]int),
	}
	pa.nodes = append(pa.nodes, Node{
		Slot:           anchorSlot,
		Root:           anchorRoot,
		ParentRoot:     anchorParentRoot,
		Parent:         -1,
		Weight:         0,
		BestChild:      -1,
		BestDescendant: -1,
	})
	pa.indices[anchorRoot] = 0
	return pa
}

func (pa *Array) Nodes() []Node {
	if pa == nil {
		return nil
	}
	out := make([]Node, len(pa.nodes))
	copy(out, pa.nodes)
	return out
}

func (pa *Array) OnBlock(slot uint64, root, parentRoot [32]byte) {
	if pa == nil {
		return
	}
	if _, exists := pa.indices[root]; exists {
		return
	}
	parentIdx, ok := pa.indices[parentRoot]
	if !ok || slot <= pa.nodes[parentIdx].Slot {
		return
	}
	nodeIndex := len(pa.nodes)

	pa.nodes = append(pa.nodes, Node{
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

func (pa *Array) ApplyScoreChanges(deltas []int64, cutoffWeight int64) {
	if pa == nil {
		return
	}
	if len(deltas) != len(pa.nodes) {
		return
	}

	for i := len(pa.nodes) - 1; i >= 0; i-- {
		pa.nodes[i].Weight += deltas[i]
		if pa.nodes[i].Parent >= 0 {
			deltas[pa.nodes[i].Parent] += deltas[i]
		}
	}

	for i := range pa.nodes {
		pa.nodes[i].BestChild = -1
		pa.nodes[i].BestDescendant = -1
	}

	for i := len(pa.nodes) - 1; i >= 0; i-- {
		parentIdx := pa.nodes[i].Parent
		if parentIdx < 0 {
			continue
		}

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
			if parent.BestDescendant != nodeBestDesc {
				shouldUpdate = true
			}
		} else if parent.BestChild >= 0 {
			bestChild := &pa.nodes[parent.BestChild]
			if bestChild.Weight < pa.nodes[i].Weight {
				shouldUpdate = true
			} else if bestChild.Weight == pa.nodes[i].Weight {
				if bytes.Compare(bestChild.Root[:], pa.nodes[i].Root[:]) < 0 {
					shouldUpdate = true
				}
			}
		} else {
			shouldUpdate = true
		}

		if shouldUpdate {
			parent.BestChild = i
			parent.BestDescendant = nodeBestDesc
		}
	}
}

func (pa *Array) FindHead(justifiedRoot [32]byte) [32]byte {
	if pa == nil {
		return justifiedRoot
	}
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

func (pa *Array) Prune(finalizedRoot [32]byte) map[int]int {
	if pa == nil {
		return nil
	}
	finalizedIdx, ok := pa.indices[finalizedRoot]
	if !ok || finalizedIdx == 0 {
		return nil
	}

	// One pass keeps exactly the finalized subtree, still parents first.
	keep := make([]bool, len(pa.nodes))
	kept := make([]int, 0, len(pa.nodes)-finalizedIdx)
	for i := range pa.nodes {
		parent := pa.nodes[i].Parent
		if i == finalizedIdx || (parent >= 0 && keep[parent]) {
			keep[i] = true
			kept = append(kept, i)
		}
	}

	indexMap := make(map[int]int, len(kept))
	newNodes := make([]Node, 0, len(kept))
	for _, oldIdx := range kept {
		indexMap[oldIdx] = len(newNodes)
		newNodes = append(newNodes, pa.nodes[oldIdx])
	}

	newIndices := make(map[[32]byte]int, len(newNodes))
	for i := range newNodes {
		newIndices[newNodes[i].Root] = i
		newNodes[i].Parent = remapProtoIndex(newNodes[i].Parent, indexMap)
		newNodes[i].BestChild = remapProtoIndex(newNodes[i].BestChild, indexMap)
		newNodes[i].BestDescendant = remapProtoIndex(newNodes[i].BestDescendant, indexMap)
	}
	pa.nodes = newNodes
	pa.indices = newIndices
	return indexMap
}

func remapProtoIndex(oldIdx int, indexMap map[int]int) int {
	if oldIdx < 0 {
		return -1
	}
	if newIdx, ok := indexMap[oldIdx]; ok {
		return newIdx
	}
	return -1
}

func (pa *Array) Len() int {
	if pa == nil {
		return 0
	}
	return len(pa.nodes)
}

// Index returns root's index in the array, and whether the array holds root.
func (pa *Array) Index(root [32]byte) (int, bool) {
	if pa == nil {
		return 0, false
	}
	idx, ok := pa.indices[root]
	return idx, ok
}

// At returns the node at index i, which must be in range.
func (pa *Array) At(i int) Node {
	return pa.nodes[i]
}
