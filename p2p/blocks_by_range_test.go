package p2p

import (
	"testing"

	"github.com/geanlabs/gean/types"
)

func TestBlocksByRangeRequestSSZRoundtrip(t *testing.T) {
	req := &types.BlocksByRangeRequest{
		StartSlot: 100,
		Count:     32,
	}

	encoded, err := req.MarshalSSZ()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// Two uint64 fields = 16 bytes, no variable-size content.
	if len(encoded) != 16 {
		t.Fatalf("expected 16 bytes, got %d", len(encoded))
	}

	decoded := &types.BlocksByRangeRequest{}
	if err := decoded.UnmarshalSSZ(encoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.StartSlot != 100 || decoded.Count != 32 {
		t.Fatalf("roundtrip mismatch: got start=%d count=%d, want start=100 count=32",
			decoded.StartSlot, decoded.Count)
	}
}

func TestBlocksByRangeRequestZeroCount(t *testing.T) {
	req := &types.BlocksByRangeRequest{StartSlot: 0, Count: 0}
	encoded, err := req.MarshalSSZ()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if len(encoded) != 16 {
		t.Fatalf("expected 16 bytes, got %d", len(encoded))
	}
	decoded := &types.BlocksByRangeRequest{}
	if err := decoded.UnmarshalSSZ(encoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.StartSlot != 0 || decoded.Count != 0 {
		t.Fatalf("zero-count roundtrip mismatch: %+v", decoded)
	}
}

func TestBlocksByRangeRequestMaxValues(t *testing.T) {
	req := &types.BlocksByRangeRequest{
		StartSlot: 1<<63 + 7,
		Count:     types.MaxRequestBlocks,
	}
	encoded, err := req.MarshalSSZ()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	decoded := &types.BlocksByRangeRequest{}
	if err := decoded.UnmarshalSSZ(encoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.StartSlot != req.StartSlot || decoded.Count != req.Count {
		t.Fatalf("max-values roundtrip mismatch: got %+v, want %+v", decoded, req)
	}
}
