package importer

import (
	"testing"

	"github.com/geanlabs/gean/internal/lean/types"
)

func TestPendingBlocks(t *testing.T) {
	imp := makeTestImporter()

	var blockRoot, parentRoot [32]byte
	blockRoot[0] = 0x10
	parentRoot[0] = 0x20

	imp.Pending.SetParent(blockRoot, parentRoot)
	imp.Pending.AddChild(parentRoot, blockRoot)

	if imp.Pending.ParentBuckets() != 1 {
		t.Fatalf("expected 1 pending parent, got %d", imp.Pending.ParentBuckets())
	}
	if imp.Pending.Entries() != 1 {
		t.Fatalf("expected 1 pending block, got %d", imp.Pending.Entries())
	}
}

func TestProcessOneBlock_RejectsPreFinalized(t *testing.T) {
	imp := makeTestImporter()

	var finalizedRoot, parentRoot [32]byte
	finalizedRoot[0] = 0x05
	parentRoot[0] = 0xBB
	imp.store.SetLatestFinalized(&types.Checkpoint{Root: finalizedRoot, Slot: 10})

	signedBlock := &types.SignedBlock{
		Block: &types.Block{
			Slot:       5,
			ParentRoot: parentRoot,
			Body:       &types.BlockBody{},
		},
		Proof: &types.MultiMessageAggregate{},
	}

	var queue []*types.SignedBlock
	imp.processOneBlock(signedBlock, &queue, false)

	if imp.Pending.ParentBuckets() != 0 {
		t.Fatalf("pre-finalized block was buffered as pending; ParentBuckets=%d", imp.Pending.ParentBuckets())
	}
	if imp.Pending.Entries() != 0 {
		t.Fatalf("pre-finalized block recorded a missing-parent entry; Entries=%d", imp.Pending.Entries())
	}
	if len(queue) != 0 {
		t.Fatalf("pre-finalized block produced cascade work; queue=%d", len(queue))
	}
}

// A parent-missing block at the finalized slot can't be the finalized block,
// which has a state, so it is dropped as the spec's head sync drops it.
func TestProcessOneBlock_RejectsOtherBlockAtFinalizedSlot(t *testing.T) {
	imp := makeTestImporter()
	admitPendingBlocks(imp, 10)

	var finalizedRoot, parentRoot [32]byte
	finalizedRoot[0] = 0x05
	parentRoot[0] = 0xBB
	imp.store.SetLatestFinalized(&types.Checkpoint{Root: finalizedRoot, Slot: 10})

	signedBlock := &types.SignedBlock{
		Block: &types.Block{
			Slot:       10,
			ParentRoot: parentRoot,
			Body:       &types.BlockBody{},
		},
		Proof: &types.MultiMessageAggregate{},
	}

	var queue []*types.SignedBlock
	imp.processOneBlock(signedBlock, &queue, false)

	if imp.Pending.Entries() != 0 || imp.Pending.Count() != 0 {
		t.Fatalf("block at the finalized slot was buffered: entries=%d count=%d", imp.Pending.Entries(), imp.Pending.Count())
	}
	if len(queue) != 0 {
		t.Fatalf("block at the finalized slot produced cascade work; queue=%d", len(queue))
	}
}

func TestCascadePending(t *testing.T) {
	imp := makeTestImporter()

	var parentRoot, child1, child2 [32]byte
	parentRoot[0] = 0x01
	child1[0] = 0x10
	child2[0] = 0x20

	imp.Pending.SetParent(child1, parentRoot)
	imp.Pending.SetParent(child2, parentRoot)
	imp.Pending.AddChild(parentRoot, child1)
	imp.Pending.AddChild(parentRoot, child2)

	if imp.Pending.ChildCount(parentRoot) != 2 {
		t.Fatalf("expected 2 children pending, got %d", imp.Pending.ChildCount(parentRoot))
	}

	var queue []*types.SignedBlock
	imp.collectPendingChildren(parentRoot, &queue)

	if imp.Pending.ParentBuckets() != 0 {
		t.Fatalf("expected 0 pending after cascade, got %d", imp.Pending.ParentBuckets())
	}
	if imp.Pending.Entries() != 0 {
		t.Fatalf("expected 0 pending parents after cascade, got %d", imp.Pending.Entries())
	}
}

func TestCascadeClearsDepth(t *testing.T) {
	imp := makeTestImporter()

	var parentRoot, child1 [32]byte
	parentRoot[0] = 0x01
	child1[0] = 0x10

	imp.Pending.SetParent(child1, parentRoot)
	imp.Pending.SetDepth(child1, 5)
	imp.Pending.AddChild(parentRoot, child1)

	var queue []*types.SignedBlock
	imp.collectPendingChildren(parentRoot, &queue)

	if _, ok := imp.Pending.Depth(child1); ok {
		t.Fatal("depth should be cleared after collectPendingChildren")
	}
}

func TestBufferMissingParentKeepsImmediateParentLink(t *testing.T) {
	imp := makeTestImporter()
	admitPendingBlocks(imp, 2)

	var missingRoot, parentRoot, blockRoot [32]byte
	missingRoot[0] = 0xAA
	parentRoot[0] = 0xBB
	blockRoot[0] = 0xCC

	imp.Pending.SetParent(parentRoot, missingRoot)
	imp.Pending.SetDepth(parentRoot, 1)
	imp.Pending.AddChild(missingRoot, parentRoot)

	signedBlock := &types.SignedBlock{
		Block: &types.Block{
			Slot:       2,
			ParentRoot: parentRoot,
			Body:       &types.BlockBody{},
		},
		Proof: &types.MultiMessageAggregate{},
	}

	var queue []*types.SignedBlock
	imp.bufferMissingParentBlock(signedBlock, blockRoot, parentRoot, &queue)

	if got := imp.Pending.ResolveAncestor(blockRoot); got != missingRoot {
		t.Fatalf("resolved ancestor=0x%x, want missing root 0x%x", got, missingRoot)
	}
	if imp.Pending.ChildCount(parentRoot) != 1 {
		t.Fatalf("parent child count=%d, want 1", imp.Pending.ChildCount(parentRoot))
	}

	imp.Pending.DiscardSubtree(blockRoot)
	if imp.Pending.ChildCount(parentRoot) != 0 {
		t.Fatalf("stale child entry left under immediate parent; count=%d", imp.Pending.ChildCount(parentRoot))
	}
	if imp.Pending.Count() != 1 {
		t.Fatalf("pending count=%d, want only parentRoot waiting on missingRoot", imp.Pending.Count())
	}
}

// A full pending buffer must admit blocks near the connect frontier by
// evicting the farthest-out entry, and keep rejecting blocks that sit no
// closer than what it already holds.
func TestBufferMissingParentEvictsFarthestWhenFull(t *testing.T) {
	imp := makeTestImporter()
	admitPendingBlocks(imp, 1<<20)
	var queue []*types.SignedBlock

	mkRoot := func(slot uint64, tag byte) [32]byte {
		return [32]byte{byte(slot), byte(slot >> 8), byte(slot >> 16), tag}
	}
	buffer := func(slot uint64) [32]byte {
		blk := &types.SignedBlock{Block: &types.Block{Slot: slot, ParentRoot: mkRoot(slot, 0xBB), Body: &types.BlockBody{}}}
		root := mkRoot(slot, 0xCC)
		imp.bufferMissingParentBlock(blk, root, blk.Block.ParentRoot, &queue)
		return root
	}

	for i := 0; i < MaxPendingBlocks; i++ {
		buffer(uint64(1000 + i))
	}
	if got := imp.Pending.Count(); got != MaxPendingBlocks {
		t.Fatalf("expected buffer filled to %d, got %d", MaxPendingBlocks, got)
	}

	// Nearer than everything held: admitted, farthest entry evicted.
	nearRoot := buffer(10)
	if _, ok := imp.Pending.Depth(nearRoot); !ok {
		t.Fatal("near-frontier block was rejected by a full buffer")
	}
	if got := imp.Pending.Count(); got != MaxPendingBlocks {
		t.Fatalf("expected count to stay at cap after eviction, got %d", got)
	}
	if _, slot, ok := imp.Pending.HighestSlotEntry(); !ok || slot >= uint64(1000+MaxPendingBlocks-1) {
		t.Fatalf("expected farthest entry evicted, highest tracked slot=%d ok=%v", slot, ok)
	}

	// Farther than everything held: still rejected.
	farRoot := buffer(1 << 20)
	if _, ok := imp.Pending.Depth(farRoot); ok {
		t.Fatal("farther-than-all block should have been rejected")
	}
	if got := imp.Pending.Count(); got != MaxPendingBlocks {
		t.Fatalf("expected count unchanged after rejection, got %d", got)
	}
}

// A block with an unknown parent reaches disk before its signature can be checked,
// so the cheap checks import would apply run first.
func TestBufferMissingParentRejectsImplausibleBlocks(t *testing.T) {
	tests := []struct {
		name     string
		slot     uint64
		proposer uint64
		admitted bool
	}{
		{"next slot, slot proposer", 6, 0, true},
		{"beyond future horizon", 7, 0, false},
		{"not the slot proposer", 6, 1, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			imp := makeTestImporter()
			admitPendingBlocks(imp, 5)
			root := [32]byte{0xCC}
			blk := &types.SignedBlock{Block: &types.Block{Slot: tt.slot, ProposerIndex: tt.proposer, ParentRoot: [32]byte{0xBB}, Body: &types.BlockBody{}}}
			var queue []*types.SignedBlock
			imp.bufferMissingParentBlock(blk, root, blk.Block.ParentRoot, &queue)

			_, pending := imp.Pending.Depth(root)
			onDisk := imp.store.GetSignedBlock(root) != nil
			if pending != tt.admitted || onDisk != tt.admitted {
				t.Fatalf("pending=%v on disk=%v, want both %v", pending, onDisk, tt.admitted)
			}
		})
	}
}

// Discarding a pending block also removes the copy written to disk.
func TestDiscardedPendingBlockLeavesDisk(t *testing.T) {
	imp := makeTestImporter()
	admitPendingBlocks(imp, 5)
	parent, root := [32]byte{0xBB}, [32]byte{0xCC}
	blk := &types.SignedBlock{Block: &types.Block{Slot: 5, ParentRoot: parent, Body: &types.BlockBody{}}}
	var queue []*types.SignedBlock
	imp.bufferMissingParentBlock(blk, root, parent, &queue)
	if imp.store.GetSignedBlock(root) == nil {
		t.Fatal("precondition: pending block should be on disk")
	}

	imp.OnFailedRoot(parent)

	if imp.store.GetSignedBlock(root) != nil || imp.store.GetBlockHeader(root) != nil {
		t.Fatal("discarded pending block still on disk")
	}
}

// prunedCanonicalWithCraftedChild stands in for canonical blocks below finality,
// a1 <- a2: block data on disk, state pruned by PruneOnFinalization. z is a1's
// parent with no header. An unsigned block at the current slot from the slot's
// proposer then names a2 as its parent.
func prunedCanonicalWithCraftedChild(t *testing.T, finalizedSlot uint64) (imp *Importer, z, a1, a2, crafted [32]byte) {
	t.Helper()
	imp = makeTestImporter()
	admitPendingBlocks(imp, 10)
	imp.store.SetLatestFinalized(&types.Checkpoint{Root: imp.store.Head(), Slot: finalizedSlot})
	z, a1, a2 = [32]byte{0x0F}, [32]byte{0xA1}, [32]byte{0xA2}
	for _, b := range []struct {
		root, parent [32]byte
		slot         uint64
	}{{a1, z, 1}, {a2, a1, 2}} {
		sb := &types.SignedBlock{Block: &types.Block{Slot: b.slot, ParentRoot: b.parent, Body: &types.BlockBody{}}}
		if err := imp.store.StorePendingBlock(b.root, sb); err != nil {
			t.Fatal(err)
		}
	}
	block := &types.SignedBlock{Block: &types.Block{Slot: 10, ParentRoot: a2, Body: &types.BlockBody{}}}
	crafted, err := block.Block.HashTreeRoot()
	if err != nil {
		t.Fatal(err)
	}
	var queue []*types.SignedBlock
	imp.processOneBlock(block, &queue, false)
	return imp, z, a1, a2, crafted
}

// Discarding a pending subtree deletes only the blocks the buffer stored, never
// imported blocks it walked over.
func TestDiscardPendingKeepsImportedAncestors(t *testing.T) {
	for _, tc := range []struct {
		name    string
		discard func(imp *Importer, z [32]byte)
	}{
		{"failed fetch", func(imp *Importer, z [32]byte) { imp.OnFailedRoot(z) }},
		{"finalized", func(imp *Importer, _ [32]byte) { imp.DiscardFinalized(2) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			imp, z, a1, a2, crafted := prunedCanonicalWithCraftedChild(t, 0)
			if imp.store.GetSignedBlock(crafted) == nil {
				t.Fatal("precondition: crafted block should be pending on disk")
			}
			tc.discard(imp, z)
			if imp.store.GetBlockHeader(a1) == nil || imp.store.GetBlockHeader(a2) == nil {
				t.Fatal("imported block data deleted")
			}
			if imp.store.GetSignedBlock(crafted) != nil {
				t.Fatal("crafted pending block still on disk")
			}
		})
	}
}

// A pending chain that reaches a stateless block at or below finality can never
// import, so it is dropped at once instead of fetching further ancestors.
func TestPendingWalkStopsAtFinality(t *testing.T) {
	imp, _, a1, a2, crafted := prunedCanonicalWithCraftedChild(t, 2)
	if n := imp.Pending.Count(); n != 0 {
		t.Fatalf("pending count=%d, want 0", n)
	}
	if len(imp.fetchInFlight) != 0 {
		t.Fatalf("fetches queued=%d, want 0", len(imp.fetchInFlight))
	}
	if imp.store.GetSignedBlock(crafted) != nil {
		t.Fatal("crafted pending block still on disk")
	}
	if imp.store.GetBlockHeader(a1) == nil || imp.store.GetBlockHeader(a2) == nil {
		t.Fatal("imported block data deleted")
	}
}
