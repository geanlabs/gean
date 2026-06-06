package storage

import (
	"bytes"
	"testing"
)

func TestInMemoryPutAndGet(t *testing.T) {
	b := NewInMemoryBackend()
	wb, _ := b.BeginWrite()
	if err := wb.PutBatch(TableBlockHeaders, []KV{
		{Key: []byte("root1"), Value: []byte("header1")},
		{Key: []byte("root2"), Value: []byte("header2")},
	}); err != nil {
		t.Fatal(err)
	}
	if err := wb.Commit(); err != nil {
		t.Fatal(err)
	}

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

func TestInMemoryPutNilValueStoresEmptyValue(t *testing.T) {
	b := NewInMemoryBackend()
	wb, _ := b.BeginWrite()
	if err := wb.PutBatch(TableMetadata, []KV{{Key: []byte("key"), Value: nil}}); err != nil {
		t.Fatal(err)
	}
	if err := wb.Commit(); err != nil {
		t.Fatal(err)
	}

	rv, _ := b.BeginRead()
	val, err := rv.Get(TableMetadata, []byte("key"))
	if err != nil {
		t.Fatal(err)
	}
	if val == nil || len(val) != 0 {
		t.Fatalf("value=%v, want non-nil empty value", val)
	}
	if b.CountEntries(TableMetadata) != 1 {
		t.Fatalf("metadata entries=%d, want 1", b.CountEntries(TableMetadata))
	}
}

func TestInMemoryDelete(t *testing.T) {
	b := NewInMemoryBackend()
	wb, _ := b.BeginWrite()
	if err := wb.PutBatch(TableStates, []KV{
		{Key: []byte("k1"), Value: []byte("v1")},
		{Key: []byte("k2"), Value: []byte("v2")},
	}); err != nil {
		t.Fatal(err)
	}
	if err := wb.Commit(); err != nil {
		t.Fatal(err)
	}

	if b.CountEntries(TableStates) != 2 {
		t.Fatal("expected 2 entries")
	}

	wb2, _ := b.BeginWrite()
	if err := wb2.DeleteBatch(TableStates, [][]byte{[]byte("k1")}); err != nil {
		t.Fatal(err)
	}
	if err := wb2.Commit(); err != nil {
		t.Fatal(err)
	}

	if b.CountEntries(TableStates) != 1 {
		t.Fatal("expected 1 entry after delete")
	}
}

func TestInMemoryPrefixIterator(t *testing.T) {
	b := NewInMemoryBackend()
	wb, _ := b.BeginWrite()
	if err := wb.PutBatch(TableLiveChain, []KV{
		{Key: []byte("aa_1"), Value: []byte("v1")},
		{Key: []byte("aa_2"), Value: []byte("v2")},
		{Key: []byte("bb_1"), Value: []byte("v3")},
	}); err != nil {
		t.Fatal(err)
	}
	if err := wb.Commit(); err != nil {
		t.Fatal(err)
	}

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

func TestInMemoryReadResultsAreCallerOwned(t *testing.T) {
	b := NewInMemoryBackend()
	wb, _ := b.BeginWrite()
	if err := wb.PutBatch(TableMetadata, []KV{{Key: []byte("key"), Value: []byte("value")}}); err != nil {
		t.Fatal(err)
	}
	if err := wb.Commit(); err != nil {
		t.Fatal(err)
	}

	rv, _ := b.BeginRead()
	val, err := rv.Get(TableMetadata, []byte("key"))
	if err != nil {
		t.Fatal(err)
	}
	val[0] = 'X'

	fresh, err := rv.Get(TableMetadata, []byte("key"))
	if err != nil {
		t.Fatal(err)
	}
	if string(fresh) != "value" {
		t.Fatalf("stored value mutated through Get result: %q", string(fresh))
	}

	it, err := rv.PrefixIterator(TableMetadata, []byte("key"))
	if err != nil {
		t.Fatal(err)
	}
	defer it.Close()
	if !it.Next() {
		t.Fatal("expected iterator entry")
	}
	key := it.Key()
	iterVal := it.Value()
	key[0] = 'X'
	iterVal[0] = 'X'
	if string(it.Key()) != "key" || string(it.Value()) != "value" {
		t.Fatal("iterator key/value should be caller-owned copies")
	}
}

func TestInMemoryPrefixIteratorSupportsEmptyKey(t *testing.T) {
	b := NewInMemoryBackend()
	wb, _ := b.BeginWrite()
	if err := wb.PutBatch(TableMetadata, []KV{{Key: nil, Value: []byte("empty")}}); err != nil {
		t.Fatal(err)
	}
	if err := wb.Commit(); err != nil {
		t.Fatal(err)
	}

	rv, _ := b.BeginRead()
	it, err := rv.PrefixIterator(TableMetadata, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer it.Close()
	if !it.Next() {
		t.Fatal("expected empty-key entry")
	}
	if key := it.Key(); key == nil || len(key) != 0 {
		t.Fatalf("key=%v, want non-nil empty slice", key)
	}
	if string(it.Value()) != "empty" {
		t.Fatalf("value=%q, want empty", string(it.Value()))
	}
}

func TestInMemoryAtomicCommit(t *testing.T) {
	b := NewInMemoryBackend()

	wb, _ := b.BeginWrite()
	if err := wb.PutBatch(TableMetadata, []KV{
		{Key: []byte("key"), Value: []byte("val")},
	}); err != nil {
		t.Fatal(err)
	}

	rv, _ := b.BeginRead()
	val, _ := rv.Get(TableMetadata, []byte("key"))
	if val != nil {
		t.Fatal("uncommitted write should not be visible")
	}

	if err := wb.Commit(); err != nil {
		t.Fatal(err)
	}
	rv2, _ := b.BeginRead()
	val2, _ := rv2.Get(TableMetadata, []byte("key"))
	if string(val2) != "val" {
		t.Fatal("committed write should be visible")
	}
}

func TestInMemoryTableIsolation(t *testing.T) {
	b := NewInMemoryBackend()
	wb, _ := b.BeginWrite()
	if err := wb.PutBatch(TableBlockHeaders, []KV{
		{Key: []byte("root"), Value: []byte("header")},
	}); err != nil {
		t.Fatal(err)
	}
	if err := wb.Commit(); err != nil {
		t.Fatal(err)
	}

	rv, _ := b.BeginRead()
	val, _ := rv.Get(TableStates, []byte("root"))
	if val != nil {
		t.Fatal("tables should be isolated")
	}
}

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

func TestInMemoryWriteBatchClosedAfterCommit(t *testing.T) {
	b := NewInMemoryBackend()
	wb, _ := b.BeginWrite()
	if err := wb.PutBatch(TableMetadata, []KV{{Key: []byte("key"), Value: []byte("value")}}); err != nil {
		t.Fatal(err)
	}
	if err := wb.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := wb.PutBatch(TableMetadata, []KV{{Key: []byte("next"), Value: []byte("value")}}); err != errBatchClosed {
		t.Fatalf("PutBatch after commit error=%v, want %v", err, errBatchClosed)
	}
	if err := wb.DeleteBatch(TableMetadata, [][]byte{[]byte("key")}); err != errBatchClosed {
		t.Fatalf("DeleteBatch after commit error=%v, want %v", err, errBatchClosed)
	}
	if err := wb.Commit(); err != errBatchClosed {
		t.Fatalf("Commit after commit error=%v, want %v", err, errBatchClosed)
	}
}
