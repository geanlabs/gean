package store

import (
	"bytes"
	"testing"
)

func TestLiveChainKeyEncoding(t *testing.T) {
	root := [32]byte{0xab, 0xcd}
	key := encodeLiveChainKey(42, root)
	if len(key) != liveChainKeySize {
		t.Fatalf("key length=%d, want %d", len(key), liveChainKeySize)
	}

	slot, decoded, ok := decodeLiveChainKey(key)
	if !ok || slot != 42 {
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
		make([]byte, liveChainKeySize-1),
	} {
		if _, _, ok := decodeLiveChainKey(key); ok {
			t.Fatalf("decoded a %d-byte key", len(key))
		}
	}
}

func TestLiveChainKeyOrdering(t *testing.T) {
	rootA := [32]byte{1}
	rootB := [32]byte{2}
	key1 := encodeLiveChainKey(10, rootA)
	key2 := encodeLiveChainKey(20, rootB)
	key3 := encodeLiveChainKey(10, rootB)

	if bytes.Compare(key1, key2) >= 0 {
		t.Fatal("slot 10 should sort before slot 20")
	}
	if bytes.Compare(key1, key3) >= 0 {
		t.Fatal("same slot, rootA should sort before rootB")
	}
}
