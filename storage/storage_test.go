package storage

import (
	"bytes"
	"testing"
)

func TestInMemoryPutAndGet(t *testing.T) {
	b := NewInMemoryBackend()
	wb, _ := b.BeginWrite()
	wb.PutBatch(TableBlockHeaders, []KV{
		{Key: []byte("root1"), Value: []byte("header1")},
		{Key: []byte("root2"), Value: []byte("header2")},
	})
	wb.Commit()

	rv, _ := b.BeginRead()
	val, err := rv.Get(TableBlockHeaders, []byte("root1"))
	if err != nil {
		t.Fatal(err)
	}
	if string(val) != "header1" {
		t.Fatalf("expected header1, got %s", string(val))
	}

	val, err = rv.Get(TableBlockHeaders, []byte("root2"))
	if err != nil {
		t.Fatal(err)
	}
	if string(val) != "header2" {
		t.Fatalf("expected header2, got %s", string(val))
	}
}

func TestInMemoryGetMissing(t *testing.T) {
	b := NewInMemoryBackend()
	rv, _ := b.BeginRead()
	val, err := rv.Get(TableBlockHeaders, []byte("nonexistent"))
	if err != nil {
		t.Fatal(err)
	}
	if val != nil {
		t.Fatal("expected nil for missing key")
	}
}

func TestInMemoryDelete(t *testing.T) {
	b := NewInMemoryBackend()
	wb, _ := b.BeginWrite()
	wb.PutBatch(TableStates, []KV{
		{Key: []byte("k1"), Value: []byte("v1")},
		{Key: []byte("k2"), Value: []byte("v2")},
	})
	wb.Commit()

	if b.CountEntries(TableStates) != 2 {
		t.Fatal("expected 2 entries")
	}

	wb2, _ := b.BeginWrite()
	wb2.DeleteBatch(TableStates, [][]byte{[]byte("k1")})
	wb2.Commit()

	if b.CountEntries(TableStates) != 1 {
		t.Fatal("expected 1 entry after delete")
	}
}

func TestInMemoryPrefixIterator(t *testing.T) {
	b := NewInMemoryBackend()
	wb, _ := b.BeginWrite()
	wb.PutBatch(TableLiveChain, []KV{
		{Key: []byte("aa_1"), Value: []byte("v1")},
		{Key: []byte("aa_2"), Value: []byte("v2")},
		{Key: []byte("bb_1"), Value: []byte("v3")},
	})
	wb.Commit()

	rv, _ := b.BeginRead()
	it, err := rv.PrefixIterator(TableLiveChain, []byte("aa"))
	if err != nil {
		t.Fatal(err)
	}
	defer it.Close()

	count := 0
	for it.Next() {
		if !bytes.HasPrefix(it.Key(), []byte("aa")) {
			t.Fatalf("key %s doesn't have prefix aa", string(it.Key()))
		}
		count++
	}
	if count != 2 {
		t.Fatalf("expected 2 entries with prefix aa, got %d", count)
	}
}

func TestInMemoryAtomicCommit(t *testing.T) {
	b := NewInMemoryBackend()

	// Write batch but don't commit
	wb, _ := b.BeginWrite()
	wb.PutBatch(TableMetadata, []KV{
		{Key: []byte("key"), Value: []byte("val")},
	})
	// Don't commit — data should not be visible.

	rv, _ := b.BeginRead()
	val, _ := rv.Get(TableMetadata, []byte("key"))
	if val != nil {
		t.Fatal("uncommitted write should not be visible")
	}

	// Now commit.
	wb.Commit()
	rv2, _ := b.BeginRead()
	val2, _ := rv2.Get(TableMetadata, []byte("key"))
	if string(val2) != "val" {
		t.Fatal("committed write should be visible")
	}
}

func TestInMemoryTableIsolation(t *testing.T) {
	b := NewInMemoryBackend()
	wb, _ := b.BeginWrite()
	wb.PutBatch(TableBlockHeaders, []KV{
		{Key: []byte("root"), Value: []byte("header")},
	})
	wb.Commit()

	rv, _ := b.BeginRead()
	// Same key, different table — should not exist.
	val, _ := rv.Get(TableStates, []byte("root"))
	if val != nil {
		t.Fatal("tables should be isolated")
	}
}

func TestLiveChainKeyEncoding(t *testing.T) {
	root := [32]byte{0xab, 0xcd}
	key := EncodeLiveChainKey(42, root)

	slot, decoded := DecodeLiveChainKey(key)
	if slot != 42 {
		t.Fatalf("expected slot 42, got %d", slot)
	}
	if decoded != root {
		t.Fatal("root mismatch")
	}
}

func TestLiveChainKeyOrdering(t *testing.T) {
	// Big-endian encoding ensures lexicographic order matches numeric order.
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
	wb.PutBatch(TableMetadata, []KV{
		{Key: []byte("k"), Value: []byte("value")},
	})
	wb.Commit()

	size := b.EstimateTableBytes(TableMetadata)
	if size == 0 {
		t.Fatal("should report non-zero size")
	}
}
