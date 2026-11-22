package forkchoice

func (fc *ForkChoice) GetCanonicalAnalysis(anchorRoot [32]byte) (canonical, nonCanonical [][32]byte) {
	anchorIdx, ok := fc.array.Index(anchorRoot)
	if !ok {
		return nil, nil
	}

	ancestorSet := make(map[[32]byte]bool)
	for idx := anchorIdx; idx >= 0; idx = fc.array.At(idx).Parent {
		root := fc.array.At(idx).Root
		ancestorSet[root] = true
		canonical = append(canonical, root)
		if fc.array.At(idx).Parent < 0 {
			break
		}
	}

	// Parents sit at lower indices, so one pass from the anchor finds its subtree.
	descendant := make([]bool, fc.array.Len())
	descendant[anchorIdx] = true
	for i := anchorIdx + 1; i < fc.array.Len(); i++ {
		if parent := fc.array.At(i).Parent; parent >= 0 && descendant[parent] {
			descendant[i] = true
		}
	}

	for i := range fc.array.Len() {
		root := fc.array.At(i).Root
		if ancestorSet[root] || descendant[i] {
			continue
		}
		nonCanonical = append(nonCanonical, root)
	}

	return canonical, nonCanonical
}

func (fc *ForkChoice) ReorgDepth(oldHead, newHead [32]byte) uint64 {
	if oldHead == newHead {
		return 0
	}
	newIdx, ok := fc.array.Index(newHead)
	if !ok {
		return 0
	}
	oldIdx, ok := fc.array.Index(oldHead)
	if !ok {
		return 0
	}

	newAncestors := make(map[[32]byte]struct{})
	for cur := newIdx; cur >= 0; cur = fc.array.At(cur).Parent {
		newAncestors[fc.array.At(cur).Root] = struct{}{}
		if fc.array.At(cur).Parent < 0 {
			break
		}
	}

	depth := uint64(0)
	for cur := oldIdx; cur >= 0; cur = fc.array.At(cur).Parent {
		if _, hit := newAncestors[fc.array.At(cur).Root]; hit {
			return depth
		}
		depth++
		if fc.array.At(cur).Parent < 0 {
			break
		}
	}
	return depth
}

func (fc *ForkChoice) AncestorAtDepth(startRoot [32]byte, depth int) (root [32]byte, slot uint64, ok bool) {
	idx, ok := fc.array.Index(startRoot)
	if !ok {
		return [32]byte{}, 0, false
	}
	if depth < 0 {
		depth = 0
	}

	remaining := depth
	if idx < remaining {
		idx = 0
		remaining = 0
	}

	for remaining > 0 && idx > 0 {
		parentIdx := fc.array.At(idx).Parent
		if parentIdx < 0 {
			break
		}
		idx = parentIdx
		remaining--
	}

	node := fc.array.At(idx)
	return node.Root, node.Slot, true
}

// WalkAncestors calls fn for root and then each ancestor the array holds,
// highest slot first, with the node's parent slot (its own slot at the array's
// anchor), until fn returns false.
func (fc *ForkChoice) WalkAncestors(root [32]byte, fn func(root [32]byte, slot, parentSlot uint64) bool) {
	idx, ok := fc.array.Index(root)
	for ok && idx >= 0 {
		node := fc.array.At(idx)
		parentSlot := node.Slot
		if node.Parent >= 0 {
			parentSlot = fc.array.At(node.Parent).Slot
		}
		if !fn(node.Root, node.Slot, parentSlot) {
			return
		}
		idx = node.Parent
	}
}
