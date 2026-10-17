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
