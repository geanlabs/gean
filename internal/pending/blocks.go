package pending

// BlockBuffer holds blocks whose parent state is not yet known, indexed so the
// engine can cascade-process descendants once a missing ancestor arrives and
// can discard orphaned subtrees when a fetch fails or finalization advances.
//
// Not safe for concurrent use: every access happens on the single-threaded
// engine loop, so (unlike AttestationBuffer) it carries no mutex.
type BlockBuffer struct {
	children map[[32]byte]map[[32]byte]bool // parent_root -> {child_roots}
	parents  map[[32]byte][32]byte          // block_root -> resolved missing ancestor
	depths   map[[32]byte]int               // block_root -> fetch depth
}

// NewBlockBuffer constructs an empty buffer.
func NewBlockBuffer() *BlockBuffer {
	return &BlockBuffer{
		children: make(map[[32]byte]map[[32]byte]bool),
		parents:  make(map[[32]byte][32]byte),
		depths:   make(map[[32]byte]int),
	}
}

// Count returns the total number of pending blocks across all parent buckets.
func (b *BlockBuffer) Count() int {
	n := 0
	for _, set := range b.children {
		n += len(set)
	}
	return n
}

// ParentBuckets returns the number of distinct missing-parent buckets.
func (b *BlockBuffer) ParentBuckets() int { return len(b.children) }

// Entries returns the number of tracked block→ancestor relationships.
func (b *BlockBuffer) Entries() int { return len(b.parents) }

// ChildCount returns how many children are buffered under parent.
func (b *BlockBuffer) ChildCount(parent [32]byte) int { return len(b.children[parent]) }

// Depth returns the recorded fetch depth for root.
func (b *BlockBuffer) Depth(root [32]byte) (int, bool) {
	d, ok := b.depths[root]
	return d, ok
}

// SetDepth records the fetch depth for root.
func (b *BlockBuffer) SetDepth(root [32]byte, depth int) { b.depths[root] = depth }

// ClearDepth drops depth tracking for root (called once it is processed).
func (b *BlockBuffer) ClearDepth(root [32]byte) { delete(b.depths, root) }

// ResolveAncestor walks the parent chain from start to the deepest recorded
// missing ancestor (returns start itself if none is recorded).
func (b *BlockBuffer) ResolveAncestor(start [32]byte) [32]byte {
	root := start
	for {
		anc, ok := b.parents[root]
		if !ok {
			return root
		}
		root = anc
	}
}

// SetParent records the missing ancestor that root is waiting on.
func (b *BlockBuffer) SetParent(root, ancestor [32]byte) { b.parents[root] = ancestor }

// AddChild records child as pending under the given missing parent.
func (b *BlockBuffer) AddChild(parent, child [32]byte) {
	set, ok := b.children[parent]
	if !ok {
		set = make(map[[32]byte]bool)
		b.children[parent] = set
	}
	set[child] = true
}

// RemoveBucket removes and returns the child set buffered under parent.
func (b *BlockBuffer) RemoveBucket(parent [32]byte) (map[[32]byte]bool, bool) {
	set, ok := b.children[parent]
	if !ok {
		return nil, false
	}
	delete(b.children, parent)
	return set, true
}

// RemoveChild removes child from parent's bucket, dropping the bucket if empty.
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

// ClearEntry drops the ancestor + depth tracking for root (without touching any
// parent bucket it may belong to).
func (b *BlockBuffer) ClearEntry(root [32]byte) {
	delete(b.parents, root)
	delete(b.depths, root)
}

// Pairs returns a snapshot of every (parent, child) relationship, safe to
// iterate while mutating the buffer.
func (b *BlockBuffer) Pairs() [][2][32]byte {
	var out [][2][32]byte
	for parent, set := range b.children {
		for child := range set {
			out = append(out, [2][32]byte{parent, child})
		}
	}
	return out
}

// DiscardSubtree recursively removes root and all of its pending descendants
// (their ancestor + depth entries and child buckets). It does NOT remove root
// from any parent bucket it belongs to — use RemoveChild for that.
func (b *BlockBuffer) DiscardSubtree(root [32]byte) {
	delete(b.parents, root)
	delete(b.depths, root)
	set, ok := b.children[root]
	if !ok {
		return
	}
	delete(b.children, root)
	for child := range set {
		b.DiscardSubtree(child)
	}
}
