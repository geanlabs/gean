package store_test

import (
	"testing"

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

func importBlock(s *store.ConsensusStore, slot uint64, root, parent [32]byte) {
	s.InsertBlockHeader(root, &types.BlockHeader{Slot: slot, ParentRoot: parent})
	s.InsertState(root, &types.State{Slot: slot})
	s.InsertLiveChainEntry(slot, root, parent)
}

func slotRoot(slot uint64) [32]byte {
	var r [32]byte
	r[0], r[1], r[2], r[3] = byte(slot), byte(slot>>8), byte(slot>>16), 0xE1
	return r
}

// The issue's acceptance case: a node in a finality stall keeps its state table
// within a bound however long the stall runs.
func TestPruneStaleStatesBoundsTheStateTableInAStall(t *testing.T) {
	// Pebble, as on a node: the in-memory backend re-sorts a table on every
	// iterator, which over thousands of slots would dominate the test.
	backend, err := storage.NewPebbleBackend(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { backend.Close() })
	s := store.NewConsensusStore(backend)
	finalized := &types.Checkpoint{Root: slotRoot(0), Slot: 0}
	justified := &types.Checkpoint{Root: slotRoot(40), Slot: 40}
	importBlock(s, 0, slotRoot(0), [32]byte{})
	const stall = 3 * store.StateRetentionSlots
	for slot := uint64(1); slot <= stall; slot++ {
		importBlock(s, slot, slotRoot(slot), slotRoot(slot-1))
		store.PruneStaleStates(s, slot, finalized, justified)
	}

	// The last StateRetentionSlots blocks below the head and the head itself,
	// plus the finalized and justified states.
	if got, bound := countStates(t, s), store.StateRetentionSlots+3; got != bound {
		t.Fatalf("states after a %d-slot stall: %d, want %d", stall, got, bound)
	}
	for _, kept := range []uint64{0, 40, stall - store.StateRetentionSlots, stall} {
		if !s.HasState(slotRoot(kept)) {
			t.Fatalf("state at slot %d was pruned", kept)
		}
	}
	if s.HasState(slotRoot(stall - store.StateRetentionSlots - 1)) {
		t.Fatal("state just past the retention window survived")
	}
	if s.GetBlockHeader(slotRoot(100)) == nil {
		t.Fatal("pruning a state took the block header with it")
	}
}

func TestPruneStaleStatesLeavesHealthyFinalityAlone(t *testing.T) {
	s := makeTestStore()
	for slot := uint64(0); slot <= 3000; slot++ {
		importBlock(s, slot, slotRoot(slot), slotRoot(slot-1))
	}
	// Finalized within the window: PruneOnFinalization owns everything below it.
	finalized := &types.Checkpoint{Root: slotRoot(2990), Slot: 2990}
	if got := store.PruneStaleStates(s, 3000, finalized, finalized); got != 0 {
		t.Fatalf("pruned %d states with finality inside the window, want 0", got)
	}
}
