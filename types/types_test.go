package types

import (
	"os"
	"testing"

	"github.com/OffchainLabs/go-bitfield"
)

func TestRoot_IsZero(t *testing.T) {
	tests := []struct {
		name string
		root Root
		want bool
	}{
		{"zero root", Root{}, true},
		{"non-zero first byte", Root{1}, false},
		{"non-zero last byte", func() Root { var r Root; r[31] = 1; return r }(), false},
		{"all ones", func() Root { var r Root; for i := range r { r[i] = 0xff }; return r }(), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.root.IsZero(); got != tt.want {
				t.Errorf("Root.IsZero() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSlot_IsJustifiableAfter(t *testing.T) {
	tests := []struct {
		name          string
		slot          Slot
		finalizedSlot Slot
		want          bool
	}{
		// Immediate range (delta <= 5)
		{"delta 0", 10, 10, true},
		{"delta 1", 11, 10, true},
		{"delta 5", 15, 10, true},
		// Perfect squares
		{"delta 9 (3^2)", 19, 10, true},
		{"delta 16 (4^2)", 26, 10, true},
		{"delta 25 (5^2)", 35, 10, true},

		// Pronic numbers (n*(n+1))
		{"delta 6 (2*3)", 16, 10, true},
		{"delta 12 (3*4)", 22, 10, true},
		{"delta 20 (4*5)", 30, 10, true},

		// Non-justifiable
		{"delta 7", 17, 10, false},
		{"delta 8", 18, 10, false},
		{"delta 10", 20, 10, false},

		// Slot before finalized
		{"slot before finalized", 5, 10, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.slot.IsJustifiableAfter(tt.finalizedSlot); got != tt.want {
				t.Errorf("Slot(%d).IsJustifiableAfter(%d) = %v, want %v",
					tt.slot, tt.finalizedSlot, got, tt.want)
			}
		})
	}
}

func TestCheckpoint_HashTreeRoot(t *testing.T) {
	cp := Checkpoint{
		Root: Root{1, 2, 3},
		Slot: 100,
	}

	root, err := cp.HashTreeRoot()
	if err != nil {
		t.Fatalf("HashTreeRoot() error = %v", err)
	}

	// Hash should be non-zero
	if root == (Root{}) {
		t.Error("HashTreeRoot() returned zero root")
	}

	// Same input should produce same hash
	root2, _ := cp.HashTreeRoot()
	if root != root2 {
		t.Error("HashTreeRoot() not deterministic")
	}

	// Different input should produce different hash
	cp2 := Checkpoint{Root: Root{4, 5, 6}, Slot: 200}
	root3, _ := cp2.HashTreeRoot()
	if root == root3 {
		t.Error("Different checkpoints should have different hashes")
	}
}

func TestBlock_HashTreeRoot(t *testing.T) {
	block := Block{
		Slot:          10,
		ProposerIndex: 5,
		ParentRoot:    Root{1, 2, 3},
		StateRoot:     Root{4, 5, 6},
		Body:          BlockBody{},
	}

	root, err := block.HashTreeRoot()
	if err != nil {
		t.Fatalf("HashTreeRoot() error = %v", err)
	}

	if root == (Root{}) {
		t.Error("HashTreeRoot() returned zero root")
	}

	// Deterministic
	root2, _ := block.HashTreeRoot()
	if root != root2 {
		t.Error("HashTreeRoot() not deterministic")
	}
}

func TestState_HashTreeRoot(t *testing.T) {
	// Use bitfield library to create valid SSZ bitlists
	justifiedSlots := bitfield.NewBitlist(0)
	justificationValidators := bitfield.NewBitlist(0)

	state := State{
		Slot: 10,
		Config: Config{
			GenesisTime: 1000000000,
		},
		LatestBlockHeader: BlockHeader{
			Slot: 9,
		},
		LatestJustified:         Checkpoint{Slot: 5},
		LatestFinalized:         Checkpoint{Slot: 3},
		HistoricalBlockHashes:   []Root{{1}, {2}, {3}},
		JustifiedSlots:          justifiedSlots,
		Validators:              []Validator{},
		JustificationValidators: justificationValidators,
	}

	root, err := state.HashTreeRoot()
	if err != nil {
		t.Fatalf("HashTreeRoot() error = %v", err)
	}

	if root == (Root{}) {
		t.Error("HashTreeRoot() returned zero root")
	}
}

func TestConstants(t *testing.T) {
	// Verify constants are set correctly
	if SecondsPerSlot != 4 {
		t.Errorf("SecondsPerSlot = %d, want 4", SecondsPerSlot)
	}
	if IntervalsPerSlot != 4 {
		t.Errorf("IntervalsPerSlot = %d, want 4", IntervalsPerSlot)
	}
	if SecondsPerInterval != 1 {
		t.Errorf("SecondsPerInterval = %d, want 1", SecondsPerInterval)
	}
}

func makeTestSignature(seed byte) Signature {
	var sig Signature
	for i := range sig {
		sig[i] = byte((int(seed) + i) % 251)
	}
	return sig
}

func makeTestRoot(seed byte) Root {
	var r Root
	for i := range r {
		r[i] = byte((int(seed) + i) % 251)
	}
	return r
}

func TestPhase1_SSZRoundTrip_AttestationData(t *testing.T) {
	orig := AttestationData{
		Slot:   12,
		Head:   Checkpoint{Root: makeTestRoot(1), Slot: 11},
		Target: Checkpoint{Root: makeTestRoot(2), Slot: 10},
		Source: Checkpoint{Root: makeTestRoot(3), Slot: 9},
	}

	data, err := orig.MarshalSSZ()
	if err != nil {
		t.Fatalf("marshal attestation data: %v", err)
	}

	var dec AttestationData
	if err := dec.UnmarshalSSZ(data); err != nil {
		t.Fatalf("unmarshal attestation data: %v", err)
	}

	if dec != orig {
		t.Fatalf("decoded attestation data mismatch")
	}
}

func TestPhase1_SSZRoundTrip_SignedAttestation(t *testing.T) {
	orig := SignedAttestation{
		Message: Attestation{
			ValidatorID: 5,
			Data: AttestationData{
				Slot:   20,
				Head:   Checkpoint{Root: makeTestRoot(4), Slot: 19},
				Target: Checkpoint{Root: makeTestRoot(5), Slot: 18},
				Source: Checkpoint{Root: makeTestRoot(6), Slot: 17},
			},
		},
		Signature: makeTestSignature(7),
	}

	data, err := orig.MarshalSSZ()
	if err != nil {
		t.Fatalf("marshal signed attestation: %v", err)
	}

	var dec SignedAttestation
	if err := dec.UnmarshalSSZ(data); err != nil {
		t.Fatalf("unmarshal signed attestation: %v", err)
	}

	if dec.Message != orig.Message {
		t.Fatalf("decoded signed attestation message mismatch")
	}
	if dec.Signature != orig.Signature {
		t.Fatalf("decoded signed attestation signature mismatch")
	}
}

func TestPhase1_SSZRoundTrip_Validator(t *testing.T) {
	orig := Validator{
		Pubkey: [52]byte{1, 2, 3, 4, 5},
		Index:  9,
	}

	data, err := orig.MarshalSSZ()
	if err != nil {
		t.Fatalf("marshal validator: %v", err)
	}

	var dec Validator
	if err := dec.UnmarshalSSZ(data); err != nil {
		t.Fatalf("unmarshal validator: %v", err)
	}

	if dec != orig {
		t.Fatalf("decoded validator mismatch")
	}
}

func TestPhase1_SSZRoundTrip_SignedBlockWithAttestation(t *testing.T) {
	orig := SignedBlockWithAttestation{
		Message: BlockWithAttestation{
			Block: Block{
				Slot:          100,
				ProposerIndex: 2,
				ParentRoot:    makeTestRoot(10),
				StateRoot:     makeTestRoot(11),
				Body: BlockBody{
					Attestations: []Attestation{
						{
							ValidatorID: 1,
							Data: AttestationData{
								Slot:   99,
								Head:   Checkpoint{Root: makeTestRoot(12), Slot: 98},
								Target: Checkpoint{Root: makeTestRoot(13), Slot: 97},
								Source: Checkpoint{Root: makeTestRoot(14), Slot: 96},
							},
						},
						{
							ValidatorID: 3,
							Data: AttestationData{
								Slot:   99,
								Head:   Checkpoint{Root: makeTestRoot(15), Slot: 98},
								Target: Checkpoint{Root: makeTestRoot(16), Slot: 97},
								Source: Checkpoint{Root: makeTestRoot(17), Slot: 96},
							},
						},
					},
				},
			},
			ProposerAttestation: Attestation{
				ValidatorID: 2,
				Data: AttestationData{
					Slot:   100,
					Head:   Checkpoint{Root: makeTestRoot(18), Slot: 99},
					Target: Checkpoint{Root: makeTestRoot(19), Slot: 98},
					Source: Checkpoint{Root: makeTestRoot(20), Slot: 97},
				},
			},
		},
		Signature: []Signature{
			makeTestSignature(21),
			makeTestSignature(22),
			makeTestSignature(23), // proposer signature
		},
	}

	data, err := orig.MarshalSSZ()
	if err != nil {
		t.Fatalf("marshal signed block: %v", err)
	}

	var dec SignedBlockWithAttestation
	if err := dec.UnmarshalSSZ(data); err != nil {
		t.Fatalf("unmarshal signed block: %v", err)
	}

	if len(dec.Message.Block.Body.Attestations) != len(orig.Message.Block.Body.Attestations) {
		t.Fatalf("decoded body attestations count mismatch")
	}
	if len(dec.Signature) != len(orig.Signature) {
		t.Fatalf("decoded signature count mismatch")
	}
	if dec.Signature[0] != orig.Signature[0] || dec.Signature[2] != orig.Signature[2] {
		t.Fatalf("decoded block signatures mismatch")
	}

	origRoot, err := orig.HashTreeRoot()
	if err != nil {
		t.Fatalf("hash original signed block: %v", err)
	}
	decRoot, err := dec.HashTreeRoot()
	if err != nil {
		t.Fatalf("hash decoded signed block: %v", err)
	}
	if origRoot != decRoot {
		t.Fatalf("signed block hash root mismatch after roundtrip")
	}
}

func TestPhase1_SSZRoundTrip_StateWithValidators(t *testing.T) {
	justified := bitfield.NewBitlist(8)
	justified.SetBitAt(0, true)
	justified.SetBitAt(3, true)

	justificationValidators := bitfield.NewBitlist(16)
	justificationValidators.SetBitAt(1, true)
	justificationValidators.SetBitAt(7, true)

	orig := State{
		Config:            Config{GenesisTime: 1700000000},
		Slot:              42,
		LatestBlockHeader: BlockHeader{Slot: 41, ProposerIndex: 1, ParentRoot: makeTestRoot(30), StateRoot: makeTestRoot(31), BodyRoot: makeTestRoot(32)},
		LatestJustified:   Checkpoint{Root: makeTestRoot(33), Slot: 39},
		LatestFinalized:   Checkpoint{Root: makeTestRoot(34), Slot: 36},
		HistoricalBlockHashes: []Root{
			makeTestRoot(35),
			makeTestRoot(36),
		},
		JustifiedSlots: justified,
		Validators: []Validator{
			{Index: 0, Pubkey: [52]byte{1, 2, 3}},
			{Index: 1, Pubkey: [52]byte{4, 5, 6}},
		},
		JustificationRoots: []Root{
			makeTestRoot(37),
			makeTestRoot(38),
		},
		JustificationValidators: justificationValidators,
	}

	data, err := orig.MarshalSSZ()
	if err != nil {
		t.Fatalf("marshal state: %v", err)
	}

	var dec State
	if err := dec.UnmarshalSSZ(data); err != nil {
		t.Fatalf("unmarshal state: %v", err)
	}

	if len(dec.Validators) != 2 {
		t.Fatalf("decoded validator count = %d, want 2", len(dec.Validators))
	}
	if dec.Validators[0].Index != 0 || dec.Validators[1].Index != 1 {
		t.Fatalf("decoded validator indexes mismatch")
	}

	origRoot, err := orig.HashTreeRoot()
	if err != nil {
		t.Fatalf("hash original state: %v", err)
	}
	decRoot, err := dec.HashTreeRoot()
	if err != nil {
		t.Fatalf("hash decoded state: %v", err)
	}
	if origRoot != decRoot {
		t.Fatalf("state hash root mismatch after roundtrip")
	}
}

func TestPhase1_DecodeSignedBlockFixtureIfPresent(t *testing.T) {
	candidates := []string{
		"testdata/signed_block_with_attestation.ssz",
		"../testdata/signed_block_with_attestation.ssz",
		"../../devnet1/testdata/signed_block_with_attestation.ssz",
	}

	var fixture []byte
	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err == nil && len(data) > 0 {
			fixture = data
			break
		}
	}
	if len(fixture) == 0 {
		t.Skip("signed_block_with_attestation.ssz fixture not found in known paths")
	}

	var block SignedBlockWithAttestation
	if err := block.UnmarshalSSZ(fixture); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
}
