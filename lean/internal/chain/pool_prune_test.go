package chain

import (
	"testing"

	"github.com/geanlabs/gean/lean/consensus/types"
	"github.com/geanlabs/gean/lean/internal/pools"
)

// Finalization pruning is the only other path that clears these pools, and it
// runs on finalization alone — so a finality stall is exactly when nothing
// empties them. The head-relative sweep covers that case.
func TestPruneStaleAttestationPools(t *testing.T) {
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
		p := pools.New()
		head := uint64(5000)
		stale, fresh := root(1), root(2)
		p.AttestationSignatures.Insert(stale, data(100), 0, [types.SignatureSize]byte{})
		p.AttestationSignatures.Insert(fresh, data(4900), 0, [types.SignatureSize]byte{})

		pruneStaleAttestationPools(p, head, 50)

		snap := p.AttestationSignatures.Snapshot()
		if _, ok := snap[stale]; ok {
			t.Fatal("stale root survived the sweep")
		}
		if _, ok := snap[fresh]; !ok {
			t.Fatal("root inside the retention window was swept")
		}
	})

	t.Run("takes a root's payload with its signatures", func(t *testing.T) {
		p := pools.New()
		stale := root(3)
		p.AttestationSignatures.Insert(stale, data(100), 0, [types.SignatureSize]byte{})
		participants := types.NewBitlistSSZ(1)
		types.BitlistSet(participants, 0)
		p.NewPayloads.Push(stale, data(100), &types.SingleMessageAggregate{
			Participants: participants,
			Proof:        []byte{1},
		})

		pruneStaleAttestationPools(p, 5000, 50)

		// Signature entry and payload entry share one AttestationData, so they
		// share a target slot and go stale together. Neither can outlive the
		// other, which is why the sweep needs no exemption for payload-bearing
		// roots.
		if _, ok := p.AttestationSignatures.Snapshot()[stale]; ok {
			t.Fatal("stale signatures survived")
		}
		if p.NewPayloads.Len() != 0 {
			t.Fatal("stale payload survived alongside its signatures")
		}
	})

	t.Run("no-op below the finalized slot", func(t *testing.T) {
		p := pools.New()
		old := root(4)
		p.AttestationSignatures.Insert(old, data(100), 0, [types.SignatureSize]byte{})

		// Finalization is healthy and past the cutoff, so finalization pruning
		// owns this range and repeating it here would be wasted work.
		pruneStaleAttestationPools(p, 5000, 4900)

		if _, ok := p.AttestationSignatures.Snapshot()[old]; !ok {
			t.Fatal("swept below the finalized slot")
		}
	})
}
