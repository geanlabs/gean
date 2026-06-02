package pending

import "testing"

func TestBlockBuffer_Count(t *testing.T) {
	b := NewBlockBuffer()

	if b.Count() != 0 {
		t.Fatal("expected 0 pending blocks initially")
	}

	parent1 := [32]byte{0x01}
	parent2 := [32]byte{0x02}
	child1 := [32]byte{0x10}
	child2 := [32]byte{0x20}
	child3 := [32]byte{0x30}

	b.AddChild(parent1, child1)
	b.AddChild(parent1, child2)
	b.AddChild(parent2, child3)

	if b.Count() != 3 {
		t.Fatalf("expected 3 pending blocks, got %d", b.Count())
	}
	if b.ParentBuckets() != 2 {
		t.Fatalf("expected 2 parent buckets, got %d", b.ParentBuckets())
	}
	if b.ChildCount(parent1) != 2 {
		t.Fatalf("expected 2 children under parent1, got %d", b.ChildCount(parent1))
	}
}

func TestBlockBuffer_DepthTracking(t *testing.T) {
	b := NewBlockBuffer()

	root1 := [32]byte{0x01}
	root2 := [32]byte{0x02}
	root3 := [32]byte{0x03}

	b.SetDepth(root1, 1)
	b.SetDepth(root2, 2)
	b.SetDepth(root3, 3)

	if d, ok := b.Depth(root3); !ok || d != 3 {
		t.Fatalf("expected depth 3, got %d (ok=%v)", d, ok)
	}

	// Depth inherited from parent.
	parentDepth, _ := b.Depth(root2)
	if parentDepth+1 != 3 {
		t.Fatalf("expected inherited depth 3, got %d", parentDepth+1)
	}

	b.ClearDepth(root3)
	if _, ok := b.Depth(root3); ok {
		t.Fatal("depth should be cleared")
	}
}

func TestBlockBuffer_ResolveAncestor(t *testing.T) {
	b := NewBlockBuffer()
	root := [32]byte{0x01}
	child := [32]byte{0x10}
	grandchild := [32]byte{0xA0}

	// grandchild -> child -> root (root has no further parent).
	b.SetParent(grandchild, child)
	b.SetParent(child, root)

	if got := b.ResolveAncestor(grandchild); got != root {
		t.Fatalf("expected ancestor=root, got 0x%x", got)
	}
	if got := b.ResolveAncestor(root); got != root {
		t.Fatalf("unrecorded root should resolve to itself, got 0x%x", got)
	}
}

func TestBlockBuffer_DiscardSubtree(t *testing.T) {
	b := NewBlockBuffer()

	// Tree: root -> child1, child1 -> grandchild1, grandchild2
	root := [32]byte{0x01}
	child1 := [32]byte{0x10}
	grandchild1 := [32]byte{0xA0}
	grandchild2 := [32]byte{0xB0}

	b.AddChild(root, child1)
	b.AddChild(child1, grandchild1)
	b.AddChild(child1, grandchild2)
	b.SetParent(child1, root)
	b.SetParent(grandchild1, child1)
	b.SetParent(grandchild2, child1)
	b.SetDepth(child1, 1)
	b.SetDepth(grandchild1, 2)
	b.SetDepth(grandchild2, 2)

	b.DiscardSubtree(child1)

	// child1 and its descendants are gone from parents + depths.
	for _, r := range [][32]byte{child1, grandchild1, grandchild2} {
		if _, ok := b.parents[r]; ok {
			t.Fatalf("0x%x should be removed from parents", r)
		}
		if _, ok := b.depths[r]; ok {
			t.Fatalf("0x%x should be removed from depths", r)
		}
	}
	// child1's own child bucket is gone.
	if _, ok := b.children[child1]; ok {
		t.Fatal("child1's child bucket should be removed")
	}
	// Root's bucket still exists (DiscardSubtree does not touch the parent link).
	if _, ok := b.children[root]; !ok {
		t.Fatal("root's child bucket should still exist")
	}
}
