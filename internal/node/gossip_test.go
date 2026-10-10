package node

import (
	"testing"

	"github.com/geanlabs/gean/internal/role"
	"github.com/geanlabs/gean/internal/types"
)

func TestOnGossipAggregatedAttestationRejectsMissingProof(t *testing.T) {
	e := makeTestEngine()
	head := e.Store.Head()
	attData := &types.AttestationData{
		Slot:   0,
		Source: &types.Checkpoint{Root: head, Slot: 0},
		Target: &types.Checkpoint{Root: head, Slot: 0},
		Head:   &types.Checkpoint{Root: head, Slot: 0},
	}

	e.onGossipAggregatedAttestation(&types.SignedAggregatedAttestation{
		Data:  attData,
		Proof: nil,
	})
	if e.Store.NewPayloads.Len() != 0 {
		t.Fatalf("payloads=%d, want 0 after nil proof", e.Store.NewPayloads.Len())
	}

	e.onGossipAggregatedAttestation(&types.SignedAggregatedAttestation{
		Data: attData,
		Proof: &types.SingleMessageAggregate{
			Participants: types.NewBitlistSSZ(0),
			Proof:        nil,
		},
	})
	if e.Store.NewPayloads.Len() != 0 {
		t.Fatalf("payloads=%d, want 0 after empty proof", e.Store.NewPayloads.Len())
	}
}

// An attestation whose head is unknown waits in the pending buffer only if it can
// pass once the head arrives: in the time horizon, above finality, and signed.
func TestOnGossipAttestationBuffersOnlyPlausibleUnknownHead(t *testing.T) {
	genesisCheckpoint := func(e *Engine) *types.Checkpoint { return &types.Checkpoint{Root: e.Store.Head()} }
	unknownHead := &types.Checkpoint{Root: [32]byte{0xEE}, Slot: 1}
	tests := []struct {
		name string
		slot uint64
	}{
		// The zero signature does not verify against the genesis registry.
		{"unsigned", 1},
		{"beyond time horizon", 1 << 40},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := makeTestEngine()
			e.AggCtl = role.New(true)
			e.Store.SetTime(types.IntervalsPerSlot) // slot 1
			e.onGossipAttestation(&types.SignedAttestation{
				ValidatorID: 0,
				Data:        &types.AttestationData{Slot: tt.slot, Head: unknownHead, Target: genesisCheckpoint(e), Source: genesisCheckpoint(e)},
			})
			if got := e.PendingAttestations.Total(); got != 0 {
				t.Fatalf("pending attestations=%d, want 0", got)
			}
		})
	}
}
