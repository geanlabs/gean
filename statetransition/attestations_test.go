package statetransition

import (
	"testing"

	"github.com/geanlabs/gean/types"
)

// TestLatestJustifiedDoesNotRegressWithinBlock ports ethlambda's
// latest_justified_does_not_regress_within_block regression and covers
// leanSpec PR #781.
//
// A single block carries three supermajority attestations targeting slots
// 4, 9, 6 in body order — all justifiable from finalized genesis (Δ=4 ≤ 5,
// Δ=9 = 3², Δ=6 = 2·3). With 4 validators, three votes is supermajority,
// so each attestation crosses the threshold.
//
// The per-slot justified_slots bitfield still records all three targets;
// only the LatestJustified pointer is gated.
func TestLatestJustifiedDoesNotRegressWithinBlock(t *testing.T) {
	const numValidators = 4

	var r3, r4, r6, r9 [types.RootSize]byte
	r3[0] = 3
	r4[0] = 4
	r6[0] = 6
	r9[0] = 9

	// historical_block_hashes indexed by slot. Only 3/4/6/9 populated;
	// other entries are zero (not referenced by any attestation here).
	hashes := make([][]byte, 10)
	for i := range hashes {
		hashes[i] = make([]byte, types.RootSize)
	}
	copy(hashes[3], r3[:])
	copy(hashes[4], r4[:])
	copy(hashes[6], r6[:])
	copy(hashes[9], r9[:])

	state := makeGenesisState(numValidators)
	state.Slot = 10
	state.LatestBlockHeader.Slot = 9
	state.LatestJustified = &types.Checkpoint{Slot: 3, Root: r3}
	state.LatestFinalized = &types.Checkpoint{}
	state.HistoricalBlockHashes = hashes
	// Mark slot 3 as justified so source=(3, r3) passes isValidVote.
	setSlotJustified(state, 0, 3)

	// Aggregation bits: 4-bit bitlist with validators 0/1/2 voted.
	// Layout: data bits [1,1,1,0] + delimiter at position 4 → 0b00010111 = 0x17.
	bits := func() []byte { return []byte{0x17} }

	mkAtt := func(targetSlot uint64, targetRoot [types.RootSize]byte) *types.AggregatedAttestation {
		return &types.AggregatedAttestation{
			AggregationBits: bits(),
			Data: &types.AttestationData{
				Slot:   targetSlot,
				Head:   &types.Checkpoint{Slot: targetSlot, Root: targetRoot},
				Target: &types.Checkpoint{Slot: targetSlot, Root: targetRoot},
				Source: &types.Checkpoint{Slot: 3, Root: r3},
			},
		}
	}

	// Body order: 4 → 9 → 6. Slot-6 attestation is processed last;
	// without the monotonic guard it would overwrite the slot-9 checkpoint.
	atts := []*types.AggregatedAttestation{
		mkAtt(4, r4),
		mkAtt(9, r9),
		mkAtt(6, r6),
	}

	if err := ProcessAttestations(state, atts); err != nil {
		t.Fatalf("ProcessAttestations: %v", err)
	}

	if state.LatestJustified.Slot != 9 {
		t.Fatalf("LatestJustified.Slot = %d, want 9 (monotonic guard violated — slot 6 dragged checkpoint back)",
			state.LatestJustified.Slot)
	}
	if state.LatestJustified.Root != r9 {
		t.Fatalf("LatestJustified.Root = %x, want %x", state.LatestJustified.Root, r9)
	}
}
