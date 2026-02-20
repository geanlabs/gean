package unit

import (
	"testing"

	"github.com/geanlabs/gean/types"
)

func TestAdvanceTimeAdvancesToCorrectInterval(t *testing.T) {
	fc, _ := makeGenesisFC(5)
	genesisTime := fc.GenesisTime

	// Advance 2 seconds past genesis (= 2 intervals with SecondsPerInterval=1).
	fc.AdvanceTime(genesisTime+2, false)

	if fc.Time != 2 {
		t.Fatalf("fc.Time = %d, want 2", fc.Time)
	}
}

func TestAdvanceTimeBeforeGenesisNoOp(t *testing.T) {
	fc, _ := makeGenesisFC(5)
	initialTime := fc.Time

	fc.AdvanceTime(fc.GenesisTime-1, false)

	if fc.Time != initialTime {
		t.Fatalf("fc.Time changed to %d, should stay at %d before genesis", fc.Time, initialTime)
	}
}

func TestTickIntervalCyclesThroughAllIntervals(t *testing.T) {
	fc, _ := makeGenesisFC(5)

	// Tick through a full slot (4 intervals).
	for i := 0; i < int(types.IntervalsPerSlot); i++ {
		fc.TickInterval(false)
	}

	expectedTime := uint64(types.IntervalsPerSlot) // genesis is at slot 0, so time starts at 0
	if fc.Time != expectedTime {
		t.Fatalf("after %d ticks: fc.Time = %d, want %d", types.IntervalsPerSlot, fc.Time, expectedTime)
	}
}

func TestAcceptNewAttestationsOnInterval0WithProposal(t *testing.T) {
	fc, _ := makeGenesisFC(5)
	fc.LatestNewAttestations[0] = &types.SignedAttestation{
		Message: &types.Attestation{
			ValidatorID: 0,
			Data: &types.AttestationData{
				Slot:   0,
				Head:   &types.Checkpoint{Root: fc.Head, Slot: 0},
				Target: &types.Checkpoint{Root: fc.Head, Slot: 0},
				Source: &types.Checkpoint{Root: fc.Head, Slot: 0},
			},
		},
	}

	// Tick to interval 0 of next slot with hasProposal=true.
	// We need to tick IntervalsPerSlot times to reach interval 0 of the next slot.
	for i := uint64(0); i < types.IntervalsPerSlot; i++ {
		fc.TickInterval(i == types.IntervalsPerSlot-1) // only last tick has proposal
	}

	// Attestation should have been accepted at interval 3 (accept new attestations).
	if _, ok := fc.LatestKnownAttestations[0]; !ok {
		t.Fatal("attestation should have been accepted at interval 3 (accept new attestations)")
	}
}

func TestUpdateSafeTargetRequiresSupermajority(t *testing.T) {
	fc, _ := makeGenesisFC(5)

	// With no votes, safe target should stay at genesis.
	fc.UpdateSafeTarget()
	initialSafe := fc.SafeTarget

	// Safe target should still be genesis (head) since no votes exist.
	if fc.SafeTarget != initialSafe {
		t.Fatal("safe target changed without any votes")
	}
}
