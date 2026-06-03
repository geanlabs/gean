package forkchoice

func (fc *ForkChoice) GetCanonicalAnalysis(anchorRoot [32]byte) (canonical, nonCanonical [][32]byte) {
	if fc == nil || fc.array == nil {
		return nil, nil
	}
	anchorIdx, ok := fc.array.indices[anchorRoot]
	if !ok {
		return nil, nil
	}

	ancestorSet := make(map[[32]byte]bool)
	for idx := anchorIdx; idx >= 0; idx = fc.array.nodes[idx].Parent {
		root := fc.array.nodes[idx].Root
		ancestorSet[root] = true
		canonical = append(canonical, root)
		if fc.array.nodes[idx].Parent < 0 {
			break
		}
	}

	descendantSet := map[[32]byte]bool{anchorRoot: true}
	for changed := true; changed; {
		changed = false
		for i := range fc.array.nodes {
			root := fc.array.nodes[i].Root
			if descendantSet[root] {
				continue
			}
			parent := fc.array.nodes[i].Parent
			if parent >= 0 && parent < len(fc.array.nodes) && descendantSet[fc.array.nodes[parent].Root] {
				descendantSet[root] = true
				changed = true
			}
		}
	}

	for i := range fc.array.nodes {
		root := fc.array.nodes[i].Root
		if ancestorSet[root] || descendantSet[root] {
			continue
		}
		nonCanonical = append(nonCanonical, root)
	}

	return canonical, nonCanonical
}

func (fc *ForkChoice) ReorgDepth(oldHead, newHead [32]byte) uint64 {
	if fc == nil || fc.array == nil {
		return 0
	}
	if oldHead == newHead {
		return 0
	}
	newIdx, ok := fc.array.indices[newHead]
	if !ok {
		return 0
	}
	oldIdx, ok := fc.array.indices[oldHead]
	if !ok {
		return 0
	}

	newAncestors := make(map[[32]byte]struct{})
	for cur := newIdx; cur >= 0; cur = fc.array.nodes[cur].Parent {
		newAncestors[fc.array.nodes[cur].Root] = struct{}{}
		if fc.array.nodes[cur].Parent < 0 {
			break
		}
	}

	depth := uint64(0)
	for cur := oldIdx; cur >= 0; cur = fc.array.nodes[cur].Parent {
		if _, hit := newAncestors[fc.array.nodes[cur].Root]; hit {
			return depth
		}
		depth++
		if fc.array.nodes[cur].Parent < 0 {
			break
		}
	}
	return depth
}

func (fc *ForkChoice) AncestorAtDepth(startRoot [32]byte, depth int) (root [32]byte, slot uint64, ok bool) {
	if fc == nil || fc.array == nil {
		return [32]byte{}, 0, false
	}
	idx, ok := fc.array.indices[startRoot]
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
		parentIdx := fc.array.nodes[idx].Parent
		if parentIdx < 0 {
			break
		}
		idx = parentIdx
		remaining--
	}

	node := &fc.array.nodes[idx]
	return node.Root, node.Slot, true
}
