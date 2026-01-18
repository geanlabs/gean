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
