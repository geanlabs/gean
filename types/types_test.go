package types

import (
	"bytes"
	"testing"
)

func TestSlotToTime(t *testing.T) {
	genesis := uint64(1700000000)
	if SlotToTime(0, genesis) != 1700000000 {
		t.Error("slot 0")
	}
	if SlotToTime(1, genesis) != 1700000004 {
		t.Error("slot 1")
	}
	if SlotToTime(100, genesis) != 1700000400 {
		t.Error("slot 100")
	}
}

func TestTimeToSlot(t *testing.T) {
	genesis := uint64(1700000000)
	if TimeToSlot(1700000000, genesis) != 0 {
		t.Error("time at genesis")
	}
	if TimeToSlot(1700000004, genesis) != 1 {
		t.Error("time +4s")
	}
	if TimeToSlot(1699999999, genesis) != 0 {
		t.Error("time before genesis")
	}
}

func TestRootIsZero(t *testing.T) {
	var zero Root
	if !zero.IsZero() {
		t.Error("zero root")
	}
	if (Root{1}).IsZero() {
		t.Error("non-zero root")
	}
}

func TestCheckpointSSZRoundTrip(t *testing.T) {
	original := &Checkpoint{
		Root: Root{0xab, 0xcd, 0xef},
		Slot: 100,
	}

	// Serialize
	data, err := original.MarshalSSZ()
	if err != nil {
		t.Fatalf("MarshalSSZ failed: %v", err)
	}

	// Check expected size (32 bytes for Root + 8 bytes for Slot = 40 bytes)
	if len(data) != 40 {
		t.Errorf("expected 40 bytes, got %d", len(data))
	}

	// Deserialize
	decoded := &Checkpoint{}
	err = decoded.UnmarshalSSZ(data)
	if err != nil {
		t.Fatalf("UnmarshalSSZ failed: %v", err)
	}

	// Compare
	if !bytes.Equal(decoded.Root[:], original.Root[:]) {
		t.Errorf("Root mismatch: got %x, want %x", decoded.Root, original.Root)
	}
	if decoded.Slot != original.Slot {
		t.Errorf("Slot mismatch: got %d, want %d", decoded.Slot, original.Slot)
	}
}

func TestCheckpointHashTreeRoot(t *testing.T) {
	checkpoint := &Checkpoint{
		Root: Root{0xab, 0xcd, 0xef},
		Slot: 100,
	}

	root, err := checkpoint.HashTreeRoot()
	if err != nil {
		t.Fatalf("HashTreeRoot failed: %v", err)
	}

	// Root should be 32 bytes and non-zero
	if len(root) != 32 {
		t.Errorf("expected 32 byte root, got %d", len(root))
	}

	var zeroRoot [32]byte
	if root == zeroRoot {
		t.Error("hash tree root should not be zero")
	}

	// Same input should produce same root
	root2, _ := checkpoint.HashTreeRoot()
	if root != root2 {
		t.Error("hash tree root should be deterministic")
	}
}

func TestConfigSSZRoundTrip(t *testing.T) {
	original := &Config{
		NumValidators: 100,
		GenesisTime:   1700000000,
	}

	data, err := original.MarshalSSZ()
	if err != nil {
		t.Fatalf("MarshalSSZ failed: %v", err)
	}
	// 8 bytes for NumValidators + 8 bytes for GenesisTime = 16 bytes
	if len(data) != 16 {
		t.Errorf("expected 16 bytes, got %d", len(data))
	}

	decoded := &Config{}
	if err := decoded.UnmarshalSSZ(data); err != nil {
		t.Fatalf("UnmarshalSSZ failed: %v", err)
	}
	if decoded.NumValidators != original.NumValidators {
		t.Errorf("NumValidators mismatch: got %d, want %d", decoded.NumValidators, original.NumValidators)
	}
	if decoded.GenesisTime != original.GenesisTime {
		t.Errorf("GenesisTime mismatch: got %d, want %d", decoded.GenesisTime, original.GenesisTime)
	}
}

func TestVoteSSZRoundTrip(t *testing.T) {
	original := &Vote{
		ValidatorID: 42,
		Slot:        100,
		Head:        Checkpoint{Root: Root{0x01}, Slot: 99},
		Target:      Checkpoint{Root: Root{0x02}, Slot: 98},
		Source:      Checkpoint{Root: Root{0x03}, Slot: 97},
	}

	data, err := original.MarshalSSZ()
	if err != nil {
		t.Fatalf("MarshalSSZ failed: %v", err)
	}
	// 8 (ValidatorID) + 8 (Slot) + 40*3 (Head, Target, Source) = 136 bytes
	if len(data) != 136 {
		t.Errorf("expected 136 bytes, got %d", len(data))
	}

	decoded := &Vote{}
	if err := decoded.UnmarshalSSZ(data); err != nil {
		t.Fatalf("UnmarshalSSZ failed: %v", err)
	}
	if decoded.ValidatorID != original.ValidatorID {
		t.Errorf("ValidatorID mismatch: got %d, want %d", decoded.ValidatorID, original.ValidatorID)
	}
	if decoded.Slot != original.Slot {
		t.Errorf("Slot mismatch: got %d, want %d", decoded.Slot, original.Slot)
	}
	if decoded.Head.Slot != original.Head.Slot {
		t.Errorf("Head.Slot mismatch: got %d, want %d", decoded.Head.Slot, original.Head.Slot)
	}
}

func TestSignedVoteSSZRoundTrip(t *testing.T) {
	original := &SignedVote{
		Data: Vote{
			ValidatorID: 42,
			Slot:        100,
			Head:        Checkpoint{Root: Root{0x01}, Slot: 99},
			Target:      Checkpoint{Root: Root{0x02}, Slot: 98},
			Source:      Checkpoint{Root: Root{0x03}, Slot: 97},
		},
		Signature: Bytes32{0xaa, 0xbb, 0xcc},
	}

	data, err := original.MarshalSSZ()
	if err != nil {
		t.Fatalf("MarshalSSZ failed: %v", err)
	}
	// 136 (Vote) + 32 (Signature) = 168 bytes
	if len(data) != 168 {
		t.Errorf("expected 168 bytes, got %d", len(data))
	}

	decoded := &SignedVote{}
	if err := decoded.UnmarshalSSZ(data); err != nil {
		t.Fatalf("UnmarshalSSZ failed: %v", err)
	}
	if decoded.Data.ValidatorID != original.Data.ValidatorID {
		t.Errorf("Data.ValidatorID mismatch")
	}
	if !bytes.Equal(decoded.Signature[:], original.Signature[:]) {
		t.Errorf("Signature mismatch")
	}
}

func TestBlockSSZRoundTrip(t *testing.T) {
	original := &Block{
		Slot:          100,
		ProposerIndex: 5,
		ParentRoot:    Root{0xaa},
		StateRoot:     Root{0xbb},
		Body: BlockBody{
			Attestations: []SignedVote{},
		},
	}

	data, err := original.MarshalSSZ()
	if err != nil {
		t.Fatalf("MarshalSSZ failed: %v", err)
	}

	decoded := &Block{}
	if err := decoded.UnmarshalSSZ(data); err != nil {
		t.Fatalf("UnmarshalSSZ failed: %v", err)
	}
	if decoded.Slot != original.Slot {
		t.Errorf("Slot mismatch")
	}
	if decoded.ProposerIndex != original.ProposerIndex {
		t.Errorf("ProposerIndex mismatch")
	}
}

func TestSignedBlockSSZRoundTrip(t *testing.T) {
	original := &SignedBlock{
		Message: Block{
			Slot:          100,
			ProposerIndex: 5,
			ParentRoot:    Root{0xaa},
			StateRoot:     Root{0xbb},
			Body: BlockBody{
				Attestations: []SignedVote{},
			},
		},
		Signature: Bytes32{0xdd, 0xee, 0xff},
	}

	data, err := original.MarshalSSZ()
	if err != nil {
		t.Fatalf("MarshalSSZ failed: %v", err)
	}

	decoded := &SignedBlock{}
	if err := decoded.UnmarshalSSZ(data); err != nil {
		t.Fatalf("UnmarshalSSZ failed: %v", err)
	}
	if decoded.Message.Slot != original.Message.Slot {
		t.Errorf("Message.Slot mismatch")
	}
	if !bytes.Equal(decoded.Signature[:], original.Signature[:]) {
		t.Errorf("Signature mismatch")
	}
}

func TestBlockHeaderHashTreeRoot(t *testing.T) {
	header := &BlockHeader{
		Slot:          100,
		ProposerIndex: 5,
		ParentRoot:    Root{0xaa},
		StateRoot:     Root{0xbb},
		BodyRoot:      Root{0xcc},
	}

	root, err := header.HashTreeRoot()
	if err != nil {
		t.Fatalf("HashTreeRoot failed: %v", err)
	}

	var zeroRoot [32]byte
	if root == zeroRoot {
		t.Error("hash tree root should not be zero")
	}

	root2, _ := header.HashTreeRoot()
	if root != root2 {
		t.Error("hash tree root should be deterministic")
	}
}
