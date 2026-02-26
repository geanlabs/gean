package statetransition

import (
	"testing"

	"github.com/geanlabs/gean/types"
)

func TestProcessAttestationsAggregatedSupermajority(t *testing.T) {
	sourceRoot := rootWithByte(0x11)
	targetRoot := rootWithByte(0x22)

	state := &types.State{
		Config:            &types.Config{GenesisTime: 0},
		Slot:              1,
		LatestBlockHeader: &types.BlockHeader{},
		LatestJustified:   &types.Checkpoint{Root: sourceRoot, Slot: 0},
		LatestFinalized:   &types.Checkpoint{Root: sourceRoot, Slot: 0},
		HistoricalBlockHashes: [][32]byte{
			sourceRoot,
			targetRoot,
		},
		JustifiedSlots:           bitlistFromBools(true, false),
		Validators:               makeValidators(3),
		JustificationsRoots:      [][32]byte{},
		JustificationsValidators: []byte{0x01},
	}

	bits := MakeBitlist(2)
	bits = SetBit(bits, 0, true)
	bits = SetBit(bits, 1, true)

	out := ProcessAttestations(state, []*types.AggregatedAttestation{
		{
			AggregationBits: bits,
			Data: &types.AttestationData{
				Slot: 1,
				Head: &types.Checkpoint{Root: targetRoot, Slot: 1},
				Source: &types.Checkpoint{
					Root: sourceRoot,
					Slot: 0,
				},
				Target: &types.Checkpoint{
					Root: targetRoot,
					Slot: 1,
				},
			},
		},
	})

	if out.LatestJustified.Slot != 1 || out.LatestJustified.Root != targetRoot {
		t.Fatalf("latest justified mismatch: got slot=%d root=%x", out.LatestJustified.Slot, out.LatestJustified.Root)
	}
	if !GetBit(out.JustifiedSlots, 1) {
		t.Fatalf("target slot not marked justified: %08b", out.JustifiedSlots)
	}
}

func TestProcessAttestationsDeduplicatesValidatorVotes(t *testing.T) {
	sourceRoot := rootWithByte(0x31)
	targetRoot := rootWithByte(0x32)

	state := &types.State{
		Config:            &types.Config{GenesisTime: 0},
		Slot:              1,
		LatestBlockHeader: &types.BlockHeader{},
		LatestJustified:   &types.Checkpoint{Root: sourceRoot, Slot: 0},
		LatestFinalized:   &types.Checkpoint{Root: sourceRoot, Slot: 0},
		HistoricalBlockHashes: [][32]byte{
			sourceRoot,
			targetRoot,
		},
		JustifiedSlots:           bitlistFromBools(true, false),
		Validators:               makeValidators(2),
		JustificationsRoots:      [][32]byte{},
		JustificationsValidators: []byte{0x01},
	}

	singleValidatorBitlist := func(validatorID uint64) []byte {
		bits := MakeBitlist(validatorID + 1)
		return SetBit(bits, validatorID, true)
	}

	data := &types.AttestationData{
		Slot:   1,
		Head:   &types.Checkpoint{Root: targetRoot, Slot: 1},
		Source: &types.Checkpoint{Root: sourceRoot, Slot: 0},
		Target: &types.Checkpoint{Root: targetRoot, Slot: 1},
	}

	out := ProcessAttestations(state, []*types.AggregatedAttestation{
		{AggregationBits: singleValidatorBitlist(0), Data: data},
		{AggregationBits: singleValidatorBitlist(0), Data: data}, // duplicate voter
	})

	if out.LatestJustified.Slot != 0 || out.LatestJustified.Root != sourceRoot {
		t.Fatalf("duplicate vote should not justify target: got slot=%d root=%x", out.LatestJustified.Slot, out.LatestJustified.Root)
	}
	if GetBit(out.JustifiedSlots, 1) {
		t.Fatalf("target slot should remain unjustified after duplicate vote: %08b", out.JustifiedSlots)
	}
}

func makeValidators(n int) []*types.Validator {
	validators := make([]*types.Validator, n)
	for i := 0; i < n; i++ {
		validators[i] = &types.Validator{Index: uint64(i)}
	}
	return validators
}

func bitlistFromBools(bits ...bool) []byte {
	out := []byte{0x01}
	for _, bit := range bits {
		out = AppendBit(out, bit)
	}
	return out
}

func rootWithByte(b byte) [32]byte {
	var out [32]byte
	out[0] = b
	return out
}
