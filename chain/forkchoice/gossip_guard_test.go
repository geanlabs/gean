//go:build skip_sig_verify

package forkchoice

import (
	"testing"

	"github.com/geanlabs/gean/types"
)

// makeGossipAttestationTest creates a store with a valid attestation that will
// pass validateAttestationData. Returns the store and a ready-to-process SignedAttestation.
func makeGossipAttestationTest(isAggregator bool) (*Store, *types.SignedAttestation) {
	fc := makeTestStore(4)
	fc.isAggregator = isAggregator
	// Advance time past slot 0 so the attestation is not in the future.
	fc.time = 2 * types.IntervalsPerSlot

	genesisRoot := fc.head

	// The source must reference a known block with matching slot.
	// Genesis root at slot 0 is our source.
	data := &types.AttestationData{
		Slot:   1,
		Head:   &types.Checkpoint{Root: genesisRoot, Slot: 0},
		Target: &types.Checkpoint{Root: genesisRoot, Slot: 0},
		Source: &types.Checkpoint{Root: genesisRoot, Slot: 0},
	}

	sa := &types.SignedAttestation{
		ValidatorID: 0,
		Message:     data,
	}
	return fc, sa
}

func TestGossipSignaturesNotStoredForNonAggregator(t *testing.T) {
	fc, sa := makeGossipAttestationTest(false)

	fc.processAttestationLocked(sa, false)

	if len(fc.gossipSignatures) != 0 {
		t.Fatalf("non-aggregator should not store gossip signatures, got %d", len(fc.gossipSignatures))
	}
	// The attestation should still be tracked for fork choice.
	if len(fc.latestNewAttestations) != 1 {
		t.Fatalf("expected attestation in latestNewAttestations, got %d", len(fc.latestNewAttestations))
	}
}

func TestGossipSignaturesStoredForAggregator(t *testing.T) {
	fc, sa := makeGossipAttestationTest(true)

	fc.processAttestationLocked(sa, false)

	if len(fc.gossipSignatures) != 1 {
		t.Fatalf("aggregator should store gossip signatures, got %d", len(fc.gossipSignatures))
	}
}
