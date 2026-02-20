package unit

import (
	"testing"

	"github.com/geanlabs/gean/types"
)

func TestCheckpointSSZRoundTrip(t *testing.T) {
	cp := &types.Checkpoint{Root: [32]byte{1, 2, 3}, Slot: 42}
	data, err := cp.MarshalSSZ()
	if err != nil {
		t.Fatal(err)
	}

	decoded := new(types.Checkpoint)
	if err := decoded.UnmarshalSSZ(data); err != nil {
		t.Fatal(err)
	}

	if decoded.Root != cp.Root || decoded.Slot != cp.Slot {
		t.Fatalf("round trip failed: got %+v, want %+v", decoded, cp)
	}
}

func TestCheckpointHashTreeRoot(t *testing.T) {
	cp := &types.Checkpoint{Slot: 0}
	root, err := cp.HashTreeRoot()
	if err != nil {
		t.Fatal(err)
	}
	if root == [32]byte{} {
		t.Fatal("hash tree root should not be zero for zero checkpoint")
	}
}

func TestConfigSSZRoundTrip(t *testing.T) {
	cfg := &types.Config{GenesisTime: 1770407233}
	data, err := cfg.MarshalSSZ()
	if err != nil {
		t.Fatal(err)
	}

	decoded := new(types.Config)
	if err := decoded.UnmarshalSSZ(data); err != nil {
		t.Fatal(err)
	}

	if decoded.GenesisTime != cfg.GenesisTime {
		t.Fatalf("round trip failed: got %+v, want %+v", decoded, cfg)
	}
}

func TestBlockHeaderSSZRoundTrip(t *testing.T) {
	h := &types.BlockHeader{
		Slot:          5,
		ProposerIndex: 2,
		ParentRoot:    [32]byte{0xaa},
		StateRoot:     [32]byte{0xbb},
		BodyRoot:      [32]byte{0xcc},
	}
	data, err := h.MarshalSSZ()
	if err != nil {
		t.Fatal(err)
	}

	decoded := new(types.BlockHeader)
	if err := decoded.UnmarshalSSZ(data); err != nil {
		t.Fatal(err)
	}

	if decoded.Slot != h.Slot || decoded.ProposerIndex != h.ProposerIndex ||
		decoded.ParentRoot != h.ParentRoot || decoded.StateRoot != h.StateRoot || decoded.BodyRoot != h.BodyRoot {
		t.Fatalf("round trip failed")
	}
}

func TestSignedAttestationSSZRoundTrip(t *testing.T) {
	sa := &types.SignedAttestation{
		Message: &types.Attestation{
			ValidatorID: 3,
			Data: &types.AttestationData{
				Slot:   10,
				Head:   &types.Checkpoint{Root: [32]byte{1}, Slot: 9},
				Target: &types.Checkpoint{Root: [32]byte{2}, Slot: 8},
				Source: &types.Checkpoint{Root: [32]byte{3}, Slot: 7},
			},
		},
	}
	data, err := sa.MarshalSSZ()
	if err != nil {
		t.Fatal(err)
	}

	decoded := new(types.SignedAttestation)
	if err := decoded.UnmarshalSSZ(data); err != nil {
		t.Fatal(err)
	}

	if decoded.Message.ValidatorID != 3 || decoded.Message.Data.Slot != 10 {
		t.Fatalf("attestation round trip failed: got validator_id=%d slot=%d", decoded.Message.ValidatorID, decoded.Message.Data.Slot)
	}
	if decoded.Message.Data.Head.Slot != 9 || decoded.Message.Data.Target.Slot != 8 || decoded.Message.Data.Source.Slot != 7 {
		t.Fatal("checkpoint round trip failed")
	}
}

func TestBlockSSZRoundTrip(t *testing.T) {
	block := &types.Block{
		Slot:          1,
		ProposerIndex: 0,
		ParentRoot:    [32]byte{0xaa},
		StateRoot:     [32]byte{0xbb},
		Body:          &types.BlockBody{Attestations: []*types.Attestation{}},
	}
	data, err := block.MarshalSSZ()
	if err != nil {
		t.Fatal(err)
	}

	decoded := new(types.Block)
	if err := decoded.UnmarshalSSZ(data); err != nil {
		t.Fatal(err)
	}

	if decoded.Slot != 1 || decoded.ProposerIndex != 0 || decoded.ParentRoot != block.ParentRoot {
		t.Fatalf("block round trip failed")
	}
}

func TestSignedBlockWithAttestationSSZRoundTrip(t *testing.T) {
	sb := &types.SignedBlockWithAttestation{
		Message: &types.BlockWithAttestation{
			Block: &types.Block{
				Slot:          1,
				ProposerIndex: 0,
				ParentRoot:    [32]byte{0xaa},
				StateRoot:     [32]byte{0xbb},
				Body:          &types.BlockBody{Attestations: []*types.Attestation{}},
			},
		},
	}
	data, err := sb.MarshalSSZ()
	if err != nil {
		t.Fatal(err)
	}

	decoded := new(types.SignedBlockWithAttestation)
	if err := decoded.UnmarshalSSZ(data); err != nil {
		t.Fatal(err)
	}

	if decoded.Message.Block.Slot != 1 {
		t.Fatalf("signed block with attestation round trip failed")
	}
}

func TestIsJustifiableAfter(t *testing.T) {
	tests := []struct {
		slot, finalized uint64
		want            bool
	}{
		{0, 0, true},           // delta=0, <=5
		{5, 0, true},           // delta=5, <=5
		{6, 0, true},           // delta=6, pronic (2*3)
		{7, 0, false},          // delta=7, not square or pronic
		{9, 0, true},           // delta=9, perfect square (3^2)
		{10, 0, false},         // delta=10, neither
		{12, 0, true},          // delta=12, pronic (3*4)
		{16, 0, true},          // delta=16, perfect square (4^2)
		{20, 0, true},          // delta=20, pronic (4*5)
		{25, 0, true},          // delta=25, perfect square (5^2)
		{30, 0, true},          // delta=30, pronic (5*6)
		{7, 2, true},           // delta=5, <=5
		{8, 2, true},           // delta=6, pronic
		{1 << 26, 0, true},     // delta=2^26 = 67108864, perfect square (8192^2)
		{1<<26 + 1, 0, false},  // not square or pronic
		{8192 * 8193, 0, true}, // pronic: 8192*8193 = 67117056
	}

	for _, tt := range tests {
		got := types.IsJustifiableAfter(tt.slot, tt.finalized)
		if got != tt.want {
			t.Errorf("IsJustifiableAfter(%d, %d) = %v, want %v", tt.slot, tt.finalized, got, tt.want)
		}
	}
}

func TestIsJustifiableAfterPanicsOnInvalidOrder(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic when slot < finalized slot")
		}
	}()
	_ = types.IsJustifiableAfter(1, 2)
}
