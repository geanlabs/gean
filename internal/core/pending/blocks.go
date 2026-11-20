package pending

type BlockBuffer struct {
	children map[[32]byte]map[[32]byte]bool
	parents  map[[32]byte][32]byte
	depths   map[[32]byte]int
	slots    map[[32]byte]uint64
	// stored holds the roots whose block data the buffer wrote to disk and that
	// have not been imported since: the only data a discard may delete.
	stored map[[32]byte]bool
}

func NewBlockBuffer() *BlockBuffer {
	return &BlockBuffer{
		children: make(map[[32]byte]map[[32]byte]bool),
		parents:  make(map[[32]byte][32]byte),
		depths:   make(map[[32]byte]int),
		slots:    make(map[[32]byte]uint64),
		stored:   make(map[[32]byte]bool),
	}
}

func (b *BlockBuffer) Count() int {
	n := 0
	for _, set := range b.children {
		n += len(set)
	}
	return n
}

func (b *BlockBuffer) ParentBuckets() int {
	return len(b.children)
}

func (b *BlockBuffer) Entries() int {
	return len(b.parents)
}

func (b *BlockBuffer) ChildCount(parent [32]byte) int {
	return len(b.children[parent])
}

func (b *BlockBuffer) Depth(root [32]byte) (int, bool) {
	d, ok := b.depths[root]
	return d, ok
}

func (b *BlockBuffer) SetDepth(root [32]byte, depth int) {
	b.depths[root] = depth
}

// MarkStored records that root's block data was written to disk for the buffer.
func (b *BlockBuffer) MarkStored(root [32]byte) {
	b.stored[root] = true
}

// MarkImported clears root's depth, and its block data now belongs to the chain.
func (b *BlockBuffer) MarkImported(root [32]byte) {
	delete(b.depths, root)
	delete(b.stored, root)
}

func (b *BlockBuffer) SetSlot(root [32]byte, slot uint64) {
	b.slots[root] = slot
}

// HighestSlotEntry returns the tracked entry farthest from the known chain —
// the natural eviction victim when the buffer is full, since entries closer
// to a known state are the ones that can actually connect.
func (b *BlockBuffer) HighestSlotEntry() ([32]byte, uint64, bool) {
	if len(b.slots) == 0 {
		return [32]byte{}, 0, false
	}
	var maxRoot [32]byte
	var maxSlot uint64
	found := false
	for root, slot := range b.slots {
		if !found || slot > maxSlot {
			maxRoot, maxSlot, found = root, slot, true
		}
	}
	return maxRoot, maxSlot, found
}

// DiscardAtOrBelow drops every entry whose slot is at or below slot, with the
// subtree waiting on it. It returns how many slot-carrying entries it dropped
// and the dropped roots whose block data the buffer stored. It reads only the
// recorded slots, so callers must record a slot for every entry they link in
// (the node does).
func (b *BlockBuffer) DiscardAtOrBelow(slot uint64) (int, [][32]byte) {
	var doomed [][32]byte
	for root, s := range b.slots {
		if s <= slot {
			doomed = append(doomed, root)
		}
	}
	dropped := 0
	var stored [][32]byte
	for _, root := range doomed {
		if _, still := b.slots[root]; !still {
			continue
		}
		before := len(b.slots)
		stored = append(stored, b.DiscardSubtree(root)...)
		dropped += before - len(b.slots)
	}
	return dropped, stored
}

func (b *BlockBuffer) ResolveAncestor(start [32]byte) [32]byte {
	root := start
	seen := make(map[[32]byte]bool)
	for {
		if seen[root] {
			return root
		}
		seen[root] = true
		anc, ok := b.parents[root]
		if !ok {
			return root
		}
		root = anc
	}
}

func (b *BlockBuffer) SetParent(root, ancestor [32]byte) {
	b.parents[root] = ancestor
}

func (b *BlockBuffer) AddChild(parent, child [32]byte) {
	set, ok := b.children[parent]
	if !ok {
		set = make(map[[32]byte]bool)
		b.children[parent] = set
	}
	set[child] = true
}

func (b *BlockBuffer) RemoveBucket(parent [32]byte) (map[[32]byte]bool, bool) {
	set, ok := b.children[parent]
	if !ok {
		return nil, false
	}
	delete(b.children, parent)
	return set, true
}

func (b *BlockBuffer) RemoveChild(parent, child [32]byte) {
	set, ok := b.children[parent]
	if !ok {
		return
	}
	delete(set, child)
	if len(set) == 0 {
		delete(b.children, parent)
	}
}

func (b *BlockBuffer) ClearEntry(root [32]byte) {
	delete(b.parents, root)
	delete(b.depths, root)
	delete(b.slots, root)
	delete(b.stored, root)
}

// DiscardSubtree drops root and every pending descendant, returning the dropped
// roots whose block data the buffer stored.
func (b *BlockBuffer) DiscardSubtree(root [32]byte) [][32]byte {
	if parent, ok := b.parents[root]; ok {
		b.RemoveChild(parent, root)
	} else {
		for parent, set := range b.children {
			if set[root] {
				b.RemoveChild(parent, root)
				break
			}
		}
	}
	return b.discardSubtree(root, nil)
}

func (b *BlockBuffer) discardSubtree(root [32]byte, dropped [][32]byte) [][32]byte {
	delete(b.parents, root)
	delete(b.depths, root)
	delete(b.slots, root)
	if b.stored[root] {
		delete(b.stored, root)
		dropped = append(dropped, root)
	}
	set, ok := b.children[root]
	if !ok {
		return dropped
	}
	delete(b.children, root)
	for child := range set {
		dropped = b.discardSubtree(child, dropped)
	}
	return dropped
}
