package db

import (
	"bytes"
	"testing"
)

func TestLiveChainKeyEncoding(t *testing.T) {
	root := [32]byte{0xab, 0xcd}
	key := EncodeLiveChainKey(42, root)
	if len(key) != LiveChainKeySize {
		t.Fatalf("key length=%d, want %d", len(key), LiveChainKeySize)
	}

	slot, decoded := DecodeLiveChainKey(key)
	if slot != 42 {
		t.Fatalf("expected slot 42, got %d", slot)
	}
	if decoded != root {
		t.Fatal("root mismatch")
	}
}

func TestDecodeLiveChainKeyShortInput(t *testing.T) {
	for _, key := range [][]byte{
		nil,
		{},
		{0x01},
		make([]byte, LiveChainKeySize-1),
	} {
		slot, root := DecodeLiveChainKey(key)
		if slot != 0 {
			t.Fatalf("slot=%d, want 0", slot)
		}
		if root != ([32]byte{}) {
			t.Fatalf("root=%x, want zero", root)
		}
	}
}

func TestLiveChainKeyOrdering(t *testing.T) {
	rootA := [32]byte{1}
	rootB := [32]byte{2}
	key1 := EncodeLiveChainKey(10, rootA)
	key2 := EncodeLiveChainKey(20, rootB)
	key3 := EncodeLiveChainKey(10, rootB)

	if bytes.Compare(key1, key2) >= 0 {
		t.Fatal("slot 10 should sort before slot 20")
	}
	if bytes.Compare(key1, key3) >= 0 {
		t.Fatal("same slot, rootA should sort before rootB")
	}
}

func TestEstimateTableBytes(t *testing.T) {
	b := NewInMemoryBackend()
	wb, _ := b.BeginWrite()
	if err := wb.PutBatch(TableMetadata, []KV{
		{Key: []byte("k"), Value: []byte("value")},
	}); err != nil {
		t.Fatal(err)
	}
	if err := wb.Commit(); err != nil {
		t.Fatal(err)
	}

	size := b.EstimateTableBytes(TableMetadata)
	if size == 0 {
		t.Fatal("should report non-zero size")
	}
}
