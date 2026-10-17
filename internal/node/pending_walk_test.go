package node

import (
	"testing"

	"github.com/geanlabs/gean/internal/p2p"
	"github.com/geanlabs/gean/internal/storage"
	"github.com/geanlabs/gean/internal/types"
)

// headerReadCounter counts reads of the block-header table.
type headerReadCounter struct {
	storage.Backend
	reads int
}

func (b *headerReadCounter) BeginRead() (storage.ReadView, error) {
	rv, err := b.Backend.BeginRead()
	if err != nil {
		return nil, err
	}
	return &headerReadCounterView{ReadView: rv, owner: b}, nil
}

type headerReadCounterView struct {
	storage.ReadView
	owner *headerReadCounter
}

func (v *headerReadCounterView) Get(table storage.Table, key []byte) ([]byte, error) {
	if table == storage.TableBlockHeaders {
		v.owner.reads++
	}
	return v.ReadView.Get(table, key)
}

func walkRoot(slot uint64, tag byte) [32]byte {
	return [32]byte{tag, byte(slot), byte(slot >> 8), byte(slot >> 16)}
}

// canonicalChainWithPrunedStates stores headers for slots 1..top on one chain
// off genesis, with states only from finalizedSlot up, the shape finalization
// pruning leaves behind, and sets the finalized checkpoint.
func canonicalChainWithPrunedStates(e *Engine, top, finalizedSlot uint64) {
	parent := e.Store.Head()
	for slot := uint64(1); slot <= top; slot++ {
		root := walkRoot(slot, 0xC0)
		e.Store.InsertBlockHeader(root, &types.BlockHeader{Slot: slot, ParentRoot: parent})
		if slot >= finalizedSlot {
			fin := &types.Checkpoint{Root: walkRoot(finalizedSlot, 0xC0), Slot: finalizedSlot}
			e.Store.InsertState(root, stateFinalizing(slot, fin))
		}
		parent = root
	}
	e.Store.SetLatestFinalized(&types.Checkpoint{Root: walkRoot(finalizedSlot, 0xC0), Slot: finalizedSlot})
}

// A block whose branch leaves the chain below the finalized slot can never be
// canonical. Below finalized, canonical headers stay stored while their states
// are pruned, so walking that branch for a stored ancestor with a state used to
// run header by header to genesis on the dispatch loop, link every step into the
// pending buffer, and finally ask peers for the zero root.
func TestStaleBranchBelowFinalizedIsDroppedWithoutWalking(t *testing.T) {
	e := makeTestEngine()
	const top, finalizedSlot = 3000, 2900
	canonicalChainWithPrunedStates(e, top, finalizedSlot)

	counter := &headerReadCounter{Backend: e.Store.Backend}
	e.Store.Backend = counter

	// A block from a node stuck on an old branch: above finalized, but built on a
	// canonical block from far below it.
	stale := &types.SignedBlock{Block: &types.Block{Slot: 2950, ParentRoot: walkRoot(40, 0xC0), Body: &types.BlockBody{}}}
	var queue []*types.SignedBlock
	e.processOneBlock(stale, &queue)

	if n := e.Pending.Count(); n != 0 {
		t.Fatalf("stale branch left %d entries in the pending buffer", n)
	}
	if len(queue) != 0 {
		t.Fatalf("stale branch queued %d blocks for import", len(queue))
	}
	if counter.reads > 4 {
		t.Fatalf("handling the stale block read %d headers; it must not walk the old chain", counter.reads)
	}
}

// A branch above finalized but deeper than MaxBlockFetchDepth stored blocks with
// no state is abandoned at the limit, not followed to its end.
func TestStoredAncestorWalkStopsAtFetchDepth(t *testing.T) {
	e := makeTestEngine()
	parent := e.Store.Head()
	const length = MaxBlockFetchDepth * 2
	for slot := uint64(1); slot <= length; slot++ {
		root := walkRoot(slot, 0xD0)
		// The first block's parent is unknown, so no stored ancestor has a state.
		if slot == 1 {
			parent = walkRoot(0, 0xEE)
		}
		e.Store.InsertBlockHeader(root, &types.BlockHeader{Slot: slot, ParentRoot: parent})
		parent = root
	}

	counter := &headerReadCounter{Backend: e.Store.Backend}
	e.Store.Backend = counter

	tip := &types.SignedBlock{Block: &types.Block{Slot: length + 1, ParentRoot: walkRoot(length, 0xD0), Body: &types.BlockBody{}}}
	var queue []*types.SignedBlock
	e.processOneBlock(tip, &queue)

	if counter.reads > MaxBlockFetchDepth+4 {
		t.Fatalf("walk read %d headers, want it bounded by %d", counter.reads, MaxBlockFetchDepth)
	}
	if n := e.Pending.Count(); n != 0 {
		t.Fatalf("abandoned branch left %d entries in the pending buffer", n)
	}
}

// Every entry the stored-ancestor walk links into the buffer carries its slot,
// so eviction and finalization sweeps can see it.
func TestStoredAncestorWalkRecordsSlots(t *testing.T) {
	e := makeTestEngine()
	// Three stored blocks with no state above an unknown root.
	parent := walkRoot(0, 0xEE)
	for slot := uint64(1); slot <= 3; slot++ {
		root := walkRoot(slot, 0xD0)
		e.Store.InsertBlockHeader(root, &types.BlockHeader{Slot: slot, ParentRoot: parent})
		parent = root
	}
	tip := &types.SignedBlock{Block: &types.Block{Slot: 4, ParentRoot: walkRoot(3, 0xD0), Body: &types.BlockBody{}}}
	var queue []*types.SignedBlock
	e.processOneBlock(tip, &queue)

	if n := e.Pending.Count(); n != 4 {
		t.Fatalf("buffer holds %d entries, want the block and its three stored ancestors", n)
	}
	if dropped := e.Pending.DiscardAtOrBelow(10); dropped != 4 {
		t.Fatalf("slot sweep dropped %d entries, want 4: some linked entries carry no slot", dropped)
	}
	if n := e.Pending.Count(); n != 0 {
		t.Fatalf("buffer still holds %d entries after the sweep", n)
	}
}

// A short gap through stored blocks still connects: the walk queues the first
// stored block whose parent has a state.
func TestStoredAncestorWalkQueuesConnectableBlock(t *testing.T) {
	e := makeTestEngine()
	genesis := e.Store.Head()
	first := walkRoot(1, 0xD0)
	parent := genesis
	for slot := uint64(1); slot <= 3; slot++ {
		root := walkRoot(slot, 0xD0)
		blk := &types.SignedBlock{Block: &types.Block{Slot: slot, ParentRoot: parent, Body: &types.BlockBody{}}, Proof: &types.MultiMessageAggregate{}}
		if err := e.Store.StorePendingBlock(root, blk); err != nil {
			t.Fatal(err)
		}
		parent = root
	}
	tip := &types.SignedBlock{Block: &types.Block{Slot: 4, ParentRoot: walkRoot(3, 0xD0), Body: &types.BlockBody{}}}
	var queue []*types.SignedBlock
	e.processOneBlock(tip, &queue)

	if len(queue) != 1 || queue[0].Block.Slot != 1 {
		t.Fatalf("queue = %d blocks, want block 1 (root 0x%x) ready to import", len(queue), first)
	}
}

// Genesis's parent is the zero root; there is no block to ask peers for.
func TestZeroRootIsNeverFetched(t *testing.T) {
	e := makeTestEngine()
	e.P2P = &p2p.Host{}

	e.queueMissingBlockFetch(types.ZeroRoot)
	if n := len(e.FetchRootCh); n != 0 {
		t.Fatalf("zero root was queued for fetch (%d queued)", n)
	}
	e.queueMissingBlockFetch(walkRoot(7, 0xD0))
	if n := len(e.FetchRootCh); n != 1 {
		t.Fatalf("a real missing root was not queued (%d queued)", n)
	}
}

// When finalization advances, pending entries at or below the new finalized
// slot are dropped with whatever waits on them; higher entries stay.
func TestFinalizationDropsPendingAtOrBelowFinalized(t *testing.T) {
	e := makeTestEngine()
	missing := walkRoot(0, 0xEE)
	old, waiting, fresh := walkRoot(50, 0xA0), walkRoot(70, 0xA0), walkRoot(120, 0xA0)
	e.Pending.SetSlot(old, 50)
	e.Pending.SetParent(old, missing)
	e.Pending.AddChild(missing, old)
	e.Pending.SetSlot(waiting, 70)
	e.Pending.SetParent(waiting, old)
	e.Pending.AddChild(old, waiting)
	e.Pending.SetSlot(fresh, 120)
	e.Pending.SetParent(fresh, missing)
	e.Pending.AddChild(missing, fresh)

	e.discardFinalizedPending(60)

	if n := e.Pending.Count(); n != 1 || e.Pending.ChildCount(missing) != 1 {
		t.Fatalf("buffer holds %d entries, want only the slot-120 block", n)
	}
	if got := e.Pending.ResolveAncestor(fresh); got != missing {
		t.Fatal("entry above the finalized slot lost its link")
	}
}

// Rejecting a block at or below the finalized slot also drops the blocks that
// were waiting on it as their parent: they can't connect either.
func TestRejectedPreFinalizedBlockDropsItsWaitingChildren(t *testing.T) {
	e := makeTestEngine()
	e.Store.SetLatestFinalized(&types.Checkpoint{Root: walkRoot(100, 0xC0), Slot: 100})

	// The awaited parent arrives, and it sits below the finalized slot.
	arrived := &types.SignedBlock{Block: &types.Block{Slot: 90, ParentRoot: walkRoot(89, 0xB0), Body: &types.BlockBody{}}}
	arrivedRoot, err := arrived.Block.HashTreeRoot()
	if err != nil {
		t.Fatal(err)
	}
	child := walkRoot(130, 0xB1)
	e.Pending.SetSlot(child, 130)
	e.Pending.SetParent(child, arrivedRoot)
	e.Pending.AddChild(arrivedRoot, child)

	var queue []*types.SignedBlock
	e.processOneBlock(arrived, &queue)

	if n := e.Pending.Count(); n != 0 {
		t.Fatalf("children of a rejected pre-finalized block stayed pending (%d)", n)
	}
}

// The stale branch is reached only after walking several stored blocks above
// the finalized slot: everything the walk linked goes with it.
func TestStaleBranchDropsEntriesLinkedAboveFinalized(t *testing.T) {
	e := makeTestEngine()
	const top, finalizedSlot = 300, 200
	canonicalChainWithPrunedStates(e, top, finalizedSlot)

	// Stored blocks 201..210 with no state, branching off canonical slot 50.
	parent := walkRoot(50, 0xC0)
	for slot := uint64(201); slot <= 210; slot++ {
		root := walkRoot(slot, 0xD0)
		e.Store.InsertBlockHeader(root, &types.BlockHeader{Slot: slot, ParentRoot: parent})
		parent = root
	}
	var queue []*types.SignedBlock
	first := &types.SignedBlock{Block: &types.Block{Slot: 220, ParentRoot: walkRoot(210, 0xD0), Body: &types.BlockBody{}}}
	e.processOneBlock(first, &queue)
	firstRoot, _ := first.Block.HashTreeRoot()
	second := &types.SignedBlock{Block: &types.Block{Slot: 221, ParentRoot: firstRoot, Body: &types.BlockBody{}}}
	e.processOneBlock(second, &queue)

	if n, m := e.Pending.Count(), e.Pending.Entries(); n != 0 || m != 0 {
		t.Fatalf("stale branch left count=%d entries=%d in the pending buffer", n, m)
	}
	if len(queue) != 0 {
		t.Fatalf("stale branch queued %d blocks", len(queue))
	}
}

// After a restart the buffer is empty but pending blocks are still on disk: a
// new tip on top of them walks to the first one whose parent has a state.
func TestStoredAncestorWalkConnectsAfterRestart(t *testing.T) {
	e := makeTestEngine()
	const top, finalizedSlot = 300, 300
	canonicalChainWithPrunedStates(e, top, finalizedSlot)

	parent := walkRoot(300, 0xC0)
	for slot := uint64(301); slot <= 305; slot++ {
		root := walkRoot(slot, 0xD0)
		blk := &types.SignedBlock{Block: &types.Block{Slot: slot, ParentRoot: parent, Body: &types.BlockBody{}}, Proof: &types.MultiMessageAggregate{}}
		if err := e.Store.StorePendingBlock(root, blk); err != nil {
			t.Fatal(err)
		}
		parent = root
	}
	tip := &types.SignedBlock{Block: &types.Block{Slot: 306, ParentRoot: walkRoot(305, 0xD0), Body: &types.BlockBody{}}}
	var queue []*types.SignedBlock
	e.processOneBlock(tip, &queue)

	if len(queue) != 1 || queue[0].Block.Slot != 301 {
		t.Fatalf("queue = %d blocks, want block 301 ready to import", len(queue))
	}
	if n := e.Pending.Count(); n != 5 {
		t.Fatalf("buffer holds %d entries, want the tip and the four stored blocks above 301", n)
	}
}
