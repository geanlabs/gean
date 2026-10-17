package store_test

import (
	"reflect"
	"testing"

	"github.com/geanlabs/gean/internal/forkchoice"
	"github.com/geanlabs/gean/internal/storage"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

// PruneOnFinalization is the only other path that clears these pools, and it
// runs on finalization alone — so a finality stall is exactly when nothing
// empties them. The head-relative sweep covers that case.
func TestPruneStaleAttestationPools(t *testing.T) {
	newStore := func() *store.ConsensusStore {
		return store.NewConsensusStore(storage.NewInMemoryBackend())
	}
	data := func(target uint64) *types.AttestationData {
		return &types.AttestationData{
			Slot:   target,
			Head:   &types.Checkpoint{},
			Target: &types.Checkpoint{Slot: target},
			Source: &types.Checkpoint{},
		}
	}
	root := func(b byte) [32]byte {
		var r [32]byte
		r[0] = b
		return r
	}

	t.Run("sweeps stale roots while finalization is stalled", func(t *testing.T) {
		s := newStore()
		head := uint64(5000)
		stale, fresh := root(1), root(2)
		s.AttestationSignatures.Insert(stale, data(100), 0, [types.SignatureSize]byte{})
		s.AttestationSignatures.Insert(fresh, data(4900), 0, [types.SignatureSize]byte{})

		store.PruneStaleAttestationPools(s, head, 50)

		snap := s.AttestationSignatures.Snapshot()
		if _, ok := snap[stale]; ok {
			t.Fatal("stale root survived the sweep")
		}
		if _, ok := snap[fresh]; !ok {
			t.Fatal("root inside the retention window was swept")
		}
	})

	t.Run("takes a root's payload with its signatures", func(t *testing.T) {
		s := newStore()
		stale := root(3)
		s.AttestationSignatures.Insert(stale, data(100), 0, [types.SignatureSize]byte{})
		participants := types.NewBitlistSSZ(1)
		types.BitlistSet(participants, 0)
		s.NewPayloads.Push(stale, data(100), &types.SingleMessageAggregate{
			Participants: participants,
			Proof:        []byte{1},
		})

		store.PruneStaleAttestationPools(s, 5000, 50)

		// Signature entry and payload entry share one AttestationData, so they
		// share a target slot and go stale together. Neither can outlive the
		// other, which is why the sweep needs no exemption for payload-bearing
		// roots.
		if _, ok := s.AttestationSignatures.Snapshot()[stale]; ok {
			t.Fatal("stale signatures survived")
		}
		if s.NewPayloads.Len() != 0 {
			t.Fatal("stale payload survived alongside its signatures")
		}
	})

	t.Run("no-op below the finalized slot", func(t *testing.T) {
		s := newStore()
		old := root(4)
		s.AttestationSignatures.Insert(old, data(100), 0, [types.SignatureSize]byte{})

		// Finalization is healthy and past the cutoff, so PruneOnFinalization
		// owns this range and repeating it here would be wasted work.
		store.PruneStaleAttestationPools(s, 5000, 4900)

		if _, ok := s.AttestationSignatures.Snapshot()[old]; !ok {
			t.Fatal("swept below the finalized slot")
		}
	})
}

func countStates(t *testing.T, s *store.ConsensusStore) int {
	t.Helper()
	rv, err := s.Backend.BeginRead()
	if err != nil {
		t.Fatal(err)
	}
	it, err := rv.PrefixIterator(storage.TableStates, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer it.Close()
	n := 0
	for it.Next() {
		n++
	}
	return n
}

// stallChain is a store and fork choice fed the way import feeds them.
type stallChain struct {
	s  *store.ConsensusStore
	fc *forkchoice.ForkChoice
}

func newStallChain(t *testing.T) *stallChain {
	t.Helper()
	// Pebble, as on a node: the in-memory backend re-sorts a table on every
	// iterator, which over thousands of slots would dominate the test.
	backend, err := storage.NewPebbleBackend(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { backend.Close() })
	c := &stallChain{s: store.NewConsensusStore(backend), fc: forkchoice.New(0, slotRoot(0), [32]byte{})}
	c.s.InsertBlockHeader(slotRoot(0), &types.BlockHeader{})
	c.s.InsertState(slotRoot(0), &types.State{})
	return c
}

// importBlock imports a block whose parent state is present, as persistBlock
// does.
func (c *stallChain) importBlock(slot uint64, root, parent [32]byte) {
	c.s.InsertBlockHeader(root, &types.BlockHeader{Slot: slot, ParentRoot: parent})
	c.s.InsertState(root, &types.State{Slot: slot})
	c.s.InsertLiveChainEntry(slot, root, parent)
	c.s.ObserveImportedState(slot)
	c.fc.OnBlock(slot, root, parent)
}

// deliver imports a new block the way the import path does: a missing parent
// state is rebuilt by re-importing stored ancestors from the nearest kept
// state. It returns how many imports that took.
func (c *stallChain) deliver(t *testing.T, slot uint64, root, parent [32]byte) int {
	t.Helper()
	var missing [][32]byte
	for r := parent; !c.s.HasState(r); {
		h := c.s.GetBlockHeader(r)
		if h == nil {
			t.Fatalf("no header for 0x%x", r)
		}
		missing = append(missing, r)
		r = h.ParentRoot
	}
	for i := len(missing) - 1; i >= 0; i-- {
		h := c.s.GetBlockHeader(missing[i])
		c.s.InsertState(missing[i], &types.State{Slot: h.Slot})
		c.s.ObserveImportedState(h.Slot)
	}
	c.importBlock(slot, root, parent)
	return len(missing) + 1
}

func (c *stallChain) sweep(head [32]byte, headSlot uint64, finalized, justified *types.Checkpoint) {
	store.PruneStaleStates(c.s, c.fc, head, headSlot, finalized, justified)
}

func slotRoot(slot uint64) [32]byte {
	var r [32]byte
	r[0], r[1], r[2], r[3] = byte(slot), byte(slot>>8), byte(slot>>16), 0xE1
	return r
}

func forkRoot(slot uint64) [32]byte {
	r := slotRoot(slot)
	r[3] = 0xF0
	return r
}

// The issue's acceptance case: a node in a finality stall keeps its state table
// within a bound however long the stall runs.
func TestPruneStaleStatesBoundsTheStateTableInAStall(t *testing.T) {
	c := newStallChain(t)
	finalized := &types.Checkpoint{Root: slotRoot(0), Slot: 0}
	justified := &types.Checkpoint{Root: slotRoot(40), Slot: 40}
	const stall = 3 * store.StateRetentionSlots
	for slot := uint64(1); slot <= stall; slot++ {
		c.importBlock(slot, slotRoot(slot), slotRoot(slot-1))
		c.sweep(slotRoot(slot), slot, finalized, justified)
	}

	// The head and the StateRetentionSlots blocks below it, the finalized and
	// justified states, and one state per checkpoint bucket below the window.
	cutoff := uint64(stall - store.StateRetentionSlots)
	bound := store.StateRetentionSlots + 1 + 2 + int((cutoff-1)/store.StateCheckpointSlots)
	if got := countStates(t, c.s); got != bound {
		t.Fatalf("states after a %d-slot stall: %d, want %d", stall, got, bound)
	}
	for _, kept := range []uint64{0, 40, store.StateCheckpointSlots, cutoff, stall} {
		if !c.s.HasState(slotRoot(kept)) {
			t.Fatalf("state at slot %d was pruned", kept)
		}
	}
	if c.s.HasState(slotRoot(cutoff - 1)) {
		t.Fatal("state just past the retention window survived")
	}
	if c.s.GetBlockHeader(slotRoot(100)) == nil {
		t.Fatal("pruning a state took the block header with it")
	}
}

// Syncing onto a branch that forks far below the head: each branch block's
// parent state must survive the sweeps, or every sweep deletes what the last
// import built and the next block rebuilds the branch from the bottom.
func TestPruneStaleStatesKeepsABranchBeingSynced(t *testing.T) {
	c := newStallChain(t)
	finalized := &types.Checkpoint{Root: slotRoot(0), Slot: 0}
	justified := finalized
	const head = 3000
	for slot := uint64(1); slot <= head; slot++ {
		c.importBlock(slot, slotRoot(slot), slotRoot(slot-1))
		c.sweep(slotRoot(slot), slot, finalized, justified)
	}

	// The branch forks at slot 100, which is not a checkpoint: the first block
	// rebuilds back to the nearest kept ancestor, the rest import directly.
	const fork, branchEnd = 100, 900
	imports := 0
	parent := slotRoot(fork)
	for slot := uint64(fork + 1); slot <= branchEnd; slot++ {
		imports += c.deliver(t, slot, forkRoot(slot), parent)
		parent = forkRoot(slot)
		c.sweep(slotRoot(head), head, finalized, justified)
	}
	branch := branchEnd - fork
	if limit := branch + store.StateCheckpointSlots; imports > limit {
		t.Fatalf("syncing a %d-block branch took %d imports, want at most %d", branch, imports, limit)
	}
}

// A justified state kept below the window is pruned once justification moves on.
func TestPruneStaleStatesDropsTheOldJustifiedState(t *testing.T) {
	c := newStallChain(t)
	finalized := &types.Checkpoint{Root: slotRoot(0), Slot: 0}
	justified := &types.Checkpoint{Root: slotRoot(40), Slot: 40}
	const head = 3000
	for slot := uint64(1); slot <= head; slot++ {
		c.importBlock(slot, slotRoot(slot), slotRoot(slot-1))
		c.sweep(slotRoot(slot), slot, finalized, justified)
	}
	if !c.s.HasState(slotRoot(40)) {
		t.Fatal("justified state was pruned")
	}

	c.importBlock(head+1, slotRoot(head+1), slotRoot(head))
	c.sweep(slotRoot(head+1), head+1, finalized, &types.Checkpoint{Root: slotRoot(2990), Slot: 2990})
	if c.s.HasState(slotRoot(40)) {
		t.Fatal("old justified state survived after justification moved on")
	}
}

func TestPruneStaleStatesLeavesHealthyFinalityAlone(t *testing.T) {
	c := newStallChain(t)
	for slot := uint64(1); slot <= 3000; slot++ {
		c.importBlock(slot, slotRoot(slot), slotRoot(slot-1))
	}
	// Finalized within the window: PruneOnFinalization owns everything below it.
	finalized := &types.Checkpoint{Root: slotRoot(2990), Slot: 2990}
	if got := store.PruneStaleStates(c.s, c.fc, slotRoot(3000), 3000, finalized, finalized); got != 0 {
		t.Fatalf("pruned %d states with finality inside the window, want 0", got)
	}
}

// Finalized blocks more than BlockProofRetentionSlots below the head lose their
// stored signed block and live-chain entry; headers stay, and nothing at or
// above the finalized slot is touched.
func TestPruneOnFinalizationDropsOldProofs(t *testing.T) {
	run := func(t *testing.T, slots []uint64, finalized, head uint64) map[uint64]bool {
		t.Helper()
		s := makeTestStore()
		roots := map[uint64][32]byte{}
		var parent [32]byte
		for i, slot := range slots {
			var r [32]byte
			r[0], r[1] = byte(i+1), 0xC1
			roots[slot] = r
			s.InsertBlockHeader(r, &types.BlockHeader{Slot: slot, ParentRoot: parent})
			s.InsertLiveChainEntry(slot, r, parent)
			signed := &types.SignedBlock{Block: &types.Block{Slot: slot, Body: &types.BlockBody{}}, Proof: &types.MultiMessageAggregate{}}
			if err := store.WriteBlockData(s, r, signed); err != nil {
				t.Fatal(err)
			}
			parent = r
		}
		s.SetHead(roots[head])
		fc := forkchoice.New(finalized, roots[finalized], [32]byte{})
		fc.OnBlock(head, roots[head], roots[finalized])
		store.PruneOnFinalization(s, fc, 0, finalized, roots[finalized])

		kept := map[uint64]bool{}
		for _, slot := range slots {
			if s.GetBlockHeader(roots[slot]) == nil {
				t.Fatalf("slot %d lost its header", slot)
			}
			if s.GetSignedBlock(roots[slot]) != nil {
				kept[slot] = true
			}
		}
		return kept
	}

	t.Run("healthy finality drops proofs older than the retention window", func(t *testing.T) {
		// Cutoff min(29990, 30000-21600) = 8400.
		kept := run(t, []uint64{0, 100, 7300, 29990, 30000}, 29990, 30000)
		want := map[uint64]bool{29990: true, 30000: true}
		if !reflect.DeepEqual(kept, want) {
			t.Fatalf("slots with proofs %v, want %v", kept, want)
		}
	})

	t.Run("a chain younger than the window keeps every proof", func(t *testing.T) {
		kept := run(t, []uint64{0, 100, 7300, 9000}, 7300, 9000)
		if len(kept) != 4 {
			t.Fatalf("slots with proofs %v, want all four", kept)
		}
	})

	t.Run("a long stall keeps everything from the finalized block up", func(t *testing.T) {
		// Cutoff min(7300, 40000-21600) = 7300.
		kept := run(t, []uint64{0, 100, 7300, 40000}, 7300, 40000)
		want := map[uint64]bool{7300: true, 40000: true}
		if !reflect.DeepEqual(kept, want) {
			t.Fatalf("slots with proofs %v, want %v", kept, want)
		}
	})
}
