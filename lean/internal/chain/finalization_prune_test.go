package chain

import (
	"bytes"
	"errors"
	"testing"

	"github.com/geanlabs/gean/internal/storage"
	"github.com/geanlabs/gean/lean/consensus/types"
)

// addFinalizationBlock stores a block whose post-state carries finalizedIn.
func addFinalizationBlock(e *Engine, genesis, root, parent [32]byte, slot uint64, finalizedIn *types.Checkpoint) {
	e.Store.InsertBlockHeader(root, &types.BlockHeader{Slot: slot, ParentRoot: parent})
	e.Store.InsertState(root, &types.State{
		Slot:                     slot,
		LatestBlockHeader:        &types.BlockHeader{Slot: slot, ParentRoot: parent},
		LatestJustified:          &types.Checkpoint{Root: genesis, Slot: 0},
		LatestFinalized:          finalizedIn,
		JustifiedSlots:           types.NewBitlistSSZ(0),
		JustificationsValidators: types.NewBitlistSSZ(0),
	})
	e.FC.OnBlock(slot, root, parent)
}

// Finalization pruning is the only thing that shrinks TableStates and
// TableBlockHeaders, and it reads its delete list from fork choice. For a long
// time it deleted nothing at all: updateFinalizedFromHead pruned the ProtoArray
// before calling PruneOnFinalization, so GetCanonicalAnalysis ran against an
// array that no longer held the ancestors or the losing branches and returned
// two empty lists. Nothing failed, nothing logged, and the two tables grew for
// the life of the chain — which is what made the per-slot full-table scans on
// the dispatch loop expensive enough to stall the slot clock.
//
// The shape here is: genesis -> a(1) -> b(2) -> c(3), with a losing fork x(2)
// off a. Finalizing b must drop x entirely and drop a's state, while keeping
// the finalized block's own state and everything above it.
func TestFinalizationPrunesAncestorAndLosingBranch(t *testing.T) {
	e := makeTestEngine()

	genesis := [32]byte{0x01}
	rootA := [32]byte{0x0a}
	rootB := [32]byte{0x0b}
	rootC := [32]byte{0x0c}
	rootX := [32]byte{0x0f}

	add := func(root, parent [32]byte, slot uint64, finalizedIn *types.Checkpoint) {
		addFinalizationBlock(e, genesis, root, parent, slot, finalizedIn)
	}

	genesisCP := &types.Checkpoint{Root: genesis, Slot: 0}
	add(rootA, genesis, 1, genesisCP)
	add(rootX, rootA, 2, genesisCP)
	add(rootB, rootA, 2, genesisCP)
	// c's post-state is what drives finalization: it finalizes b at slot 2.
	add(rootC, rootB, 3, &types.Checkpoint{Root: rootB, Slot: 2})

	// All live pools prune by target slot after the persisted chain is pruned.
	for _, slot := range []uint64{2, 3} {
		data := &types.AttestationData{Slot: 4, Target: &types.Checkpoint{Slot: slot}}
		root := [32]byte{byte(slot)}
		proof := &types.SingleMessageAggregate{
			Participants: types.BitlistFromIndices([]uint64{0}),
			Proof:        []byte{1},
		}
		e.Pools.AttestationSignatures.Insert(root, data, 0, [types.SignatureSize]byte{})
		e.Pools.KnownPayloads.Push(root, data, proof)
		e.Pools.NewPayloads.Push(root, data, proof)
	}

	e.Store.SetHead(rootC)
	e.updateFinalizedFromHead(rootC)

	if got := e.Store.LatestFinalized(); got == nil || got.Slot != 2 || got.Root != rootB {
		t.Fatalf("finalized checkpoint = %+v, want slot 2 root %x", got, rootB)
	}

	// The losing branch goes entirely: state and block header both.
	if e.Store.HasState(rootX) {
		t.Error("losing branch state survived finalization pruning")
	}
	if e.Store.GetBlockHeader(rootX) != nil {
		t.Error("losing branch block header survived finalization pruning")
	}

	// Ancestors below the finalized root keep their headers but lose their
	// states — canonical[1:] in PruneOnFinalization.
	if e.Store.HasState(rootA) {
		t.Error("ancestor state below the finalized root survived finalization pruning")
	}

	// The finalized block and the head above it must be untouched: without
	// their states the node cannot run a transition from the finalized anchor.
	if !e.Store.HasState(rootB) {
		t.Error("finalized block state was pruned")
	}
	if !e.Store.HasState(rootC) {
		t.Error("head state was pruned")
	}
	if e.Pools.AttestationSignatures.Len() != 1 || e.Pools.KnownPayloads.Len() != 1 || e.Pools.NewPayloads.Len() != 1 {
		t.Fatal("live pools did not retain only the vote above finalization")
	}
	if e.Pools.AttestationSignatures.Has([32]byte{2}, 0) || !e.Pools.AttestationSignatures.Has([32]byte{3}, 0) {
		t.Fatal("signature pruning did not use the finalized target boundary")
	}
	if e.Pools.KnownPayloads.Entries()[[32]byte{3}] == nil || e.Pools.NewPayloads.Entries()[[32]byte{3}] == nil {
		t.Fatal("payload pruning removed the fresh target")
	}
}

type headWriteFailingBackend struct {
	storage.Backend
}

func (b headWriteFailingBackend) BeginWrite() (storage.WriteBatch, error) {
	wb, err := b.Backend.BeginWrite()
	return headWriteFailingBatch{wb}, err
}

type headWriteFailingBatch struct {
	storage.WriteBatch
}

func (b headWriteFailingBatch) PutBatch(table storage.Table, entries []storage.KV) error {
	for _, kv := range entries {
		if table == storage.TableMetadata && bytes.Equal(kv.Key, storage.KeyHead) {
			return errors.New("head write failed")
		}
	}
	return b.WriteBatch.PutBatch(table, entries)
}

// A reorg that advances finalization prunes the old head's branch. The new head
// must be stored first, and if it cannot be, nothing may be pruned: metadata
// would name a deleted head and startup could not restore the chain.
//
// genesis -> a(1) -> x(2) is the old head; a -> b(2) -> c(3) wins and c finalizes b.
func TestReorgPersistsHeadBeforeFinalizationPrune(t *testing.T) {
	genesis := [32]byte{0x01}
	rootA := [32]byte{0x0a}
	rootB := [32]byte{0x0b}
	rootC := [32]byte{0x0c}
	rootX := [32]byte{0x09} // below rootB, so the unweighted tiebreak picks b's branch

	for _, tc := range []struct {
		name       string
		failHead   bool
		wantHead   [32]byte
		wantPruned bool
	}{
		{name: "head stored then old branch pruned", wantHead: rootC, wantPruned: true},
		{name: "head write fails and nothing is pruned", failHead: true, wantHead: rootX},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := makeTestEngine()
			genesisCP := &types.Checkpoint{Root: genesis, Slot: 0}
			addFinalizationBlock(e, genesis, rootA, genesis, 1, genesisCP)
			addFinalizationBlock(e, genesis, rootX, rootA, 2, genesisCP)
			addFinalizationBlock(e, genesis, rootB, rootA, 2, genesisCP)
			addFinalizationBlock(e, genesis, rootC, rootB, 3, &types.Checkpoint{Root: rootB, Slot: 2})
			e.Store.SetHead(rootX)
			if tc.failHead {
				e.Store.Backend = headWriteFailingBackend{e.Store.Backend}
			}

			e.updateHead()

			if got := e.Store.Head(); got != tc.wantHead {
				t.Fatalf("head=%x, want %x", got, tc.wantHead)
			}
			if pruned := e.Store.GetBlockHeader(rootX) == nil; pruned != tc.wantPruned {
				t.Fatalf("old head pruned=%v, want %v", pruned, tc.wantPruned)
			}
		})
	}
}
