package node

import (
	"testing"

	"github.com/geanlabs/gean/types"
)

// mockProof creates a test AggregatedSignatureProof covering the given validator IDs.
func mockProof(ids []uint64) *types.AggregatedSignatureProof {
	return &types.AggregatedSignatureProof{
		Participants: AggregationBitsFromIndices(ids),
		ProofData:    []byte{0xDE, 0xAD}, // dummy proof bytes
	}
}

func TestSelectGreedyProofs_SingleProof(t *testing.T) {
	entry := &PayloadEntry{
		Data: &types.AttestationData{
			Slot:   10,
			Head:   &types.Checkpoint{Slot: 10},
			Target: &types.Checkpoint{Slot: 8},
			Source: &types.Checkpoint{Slot: 4},
		},
		Proofs: []*types.AggregatedSignatureProof{
			mockProof([]uint64{0, 1, 2}),
		},
	}

	var attestations []*types.AggregatedAttestation
	var signatures []*types.AggregatedSignatureProof

	// No state/pkCache needed for single proof (no merge).
	selectGreedyProofs(entry, nil, nil, &attestations, &signatures)

	if len(attestations) != 1 {
		t.Fatalf("expected 1 attestation, got %d", len(attestations))
	}
	if len(signatures) != 1 {
		t.Fatalf("expected 1 signature, got %d", len(signatures))
	}

	// Verify all 3 validators covered.
	for _, vid := range []uint64{0, 1, 2} {
		if !types.BitlistGet(signatures[0].Participants, vid) {
			t.Errorf("validator %d not in participants", vid)
		}
	}
}

func TestSelectGreedyProofs_GreedyOrder(t *testing.T) {
	// Two proofs with overlapping coverage:
	// Proof A covers validators {0, 1, 2} (3 validators)
	// Proof B covers validators {2, 3, 4, 5} (4 validators)
	// Greedy should pick B first (more coverage), then A adds {0, 1}.
	entry := &PayloadEntry{
		Data: &types.AttestationData{
			Slot:   10,
			Head:   &types.Checkpoint{Slot: 10},
			Target: &types.Checkpoint{Slot: 8},
			Source: &types.Checkpoint{Slot: 4},
		},
		Proofs: []*types.AggregatedSignatureProof{
			mockProof([]uint64{0, 1, 2}),    // Proof A: 3 validators
			mockProof([]uint64{2, 3, 4, 5}), // Proof B: 4 validators
		},
	}

	var attestations []*types.AggregatedAttestation
	var signatures []*types.AggregatedSignatureProof

	// With nil state/pkCache, mergeProofs will return nil (can't look up pubkeys).
	// selectGreedyProofs should skip the AttestationData when merge fails.
	selectGreedyProofs(entry, nil, nil, &attestations, &signatures)

	// Merge fails (no state) → attestation skipped per spec behavior.
	if len(attestations) != 0 {
		t.Fatalf("expected 0 attestations (merge fails without state), got %d", len(attestations))
	}
}

func TestSelectGreedyProofs_NoOverlap(t *testing.T) {
	// Two non-overlapping proofs:
	// Proof A covers {0, 1}
	// Proof B covers {2, 3}
	// Both should be selected by greedy, then merged.
	entry := &PayloadEntry{
		Data: &types.AttestationData{
			Slot:   10,
			Head:   &types.Checkpoint{Slot: 10},
			Target: &types.Checkpoint{Slot: 8},
			Source: &types.Checkpoint{Slot: 4},
		},
		Proofs: []*types.AggregatedSignatureProof{
			mockProof([]uint64{0, 1}),
			mockProof([]uint64{2, 3}),
		},
	}

	var attestations []*types.AggregatedAttestation
	var signatures []*types.AggregatedSignatureProof

	// Without state, merge fails → skipped.
	selectGreedyProofs(entry, nil, nil, &attestations, &signatures)

	if len(attestations) != 0 {
		t.Fatalf("expected 0 attestations (merge fails without state), got %d", len(attestations))
	}
}

func TestSelectGreedyProofs_EmptyProofs(t *testing.T) {
	entry := &PayloadEntry{
		Data: &types.AttestationData{
			Slot:   10,
			Head:   &types.Checkpoint{Slot: 10},
			Target: &types.Checkpoint{Slot: 8},
			Source: &types.Checkpoint{Slot: 4},
		},
		Proofs: []*types.AggregatedSignatureProof{},
	}

	var attestations []*types.AggregatedAttestation
	var signatures []*types.AggregatedSignatureProof

	selectGreedyProofs(entry, nil, nil, &attestations, &signatures)

	if len(attestations) != 0 {
		t.Fatalf("expected 0 attestations for empty proofs, got %d", len(attestations))
	}
}

func TestSelectGreedyProofs_SubsetProofSkipped(t *testing.T) {
	// Proof A covers {0, 1, 2, 3, 4}
	// Proof B covers {1, 2} (strict subset of A)
	// Greedy picks A first (5 validators). B adds 0 new → not selected.
	// Single proof selected → used directly.
	entry := &PayloadEntry{
		Data: &types.AttestationData{
			Slot:   10,
			Head:   &types.Checkpoint{Slot: 10},
			Target: &types.Checkpoint{Slot: 8},
			Source: &types.Checkpoint{Slot: 4},
		},
		Proofs: []*types.AggregatedSignatureProof{
			mockProof([]uint64{0, 1, 2, 3, 4}),
			mockProof([]uint64{1, 2}),
		},
	}

	var attestations []*types.AggregatedAttestation
	var signatures []*types.AggregatedSignatureProof

	selectGreedyProofs(entry, nil, nil, &attestations, &signatures)

	// Only 1 proof selected (B is a subset, adds nothing new) → used directly.
	if len(attestations) != 1 {
		t.Fatalf("expected 1 attestation (subset skipped), got %d", len(attestations))
	}

	// Should cover all 5 validators from proof A.
	for _, vid := range []uint64{0, 1, 2, 3, 4} {
		if !types.BitlistGet(signatures[0].Participants, vid) {
			t.Errorf("validator %d not in participants", vid)
		}
	}
}

func TestCountParticipants(t *testing.T) {
	bits := AggregationBitsFromIndices([]uint64{0, 3, 7})
	got := countParticipants(bits)
	if got != 3 {
		t.Errorf("countParticipants: got %d, want 3", got)
	}

	empty := AggregationBitsFromIndices([]uint64{})
	if countParticipants(empty) != 0 {
		t.Errorf("countParticipants(empty): got %d, want 0", countParticipants(empty))
	}
}
