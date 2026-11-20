package storage

import (
	"bytes"
	"testing"
)

// The backend tests need tables; any names do.
const (
	tableA Table = "a"
	tableB Table = "b"
	tableC Table = "c"
	tableD Table = "d"
)

func TestInMemoryPutAndGet(t *testing.T) {
	b := NewInMemoryBackend()
	wb, _ := b.BeginWrite()
	if err := wb.PutBatch(tableC, []KV{
		{Key: []byte("root1"), Value: []byte("header1")},
		{Key: []byte("root2"), Value: []byte("header2")},
	}); err != nil {
		t.Fatal(err)
	}
	if err := wb.Commit(); err != nil {
		t.Fatal(err)
	}

	rv, _ := b.BeginRead()
	val, err := rv.Get(tableC, []byte("root1"))
	if err != nil {
		t.Fatal(err)
	}
	if string(val) != "header1" {
		t.Fatalf("expected header1, got %s", string(val))
	}

	val, err = rv.Get(tableC, []byte("root2"))
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
	val, err := rv.Get(tableC, []byte("nonexistent"))
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
	if err := wb.PutBatch(tableA, []KV{{Key: []byte("key"), Value: nil}}); err != nil {
		t.Fatal(err)
	}
	if err := wb.Commit(); err != nil {
		t.Fatal(err)
	}

	rv, _ := b.BeginRead()
	val, err := rv.Get(tableA, []byte("key"))
	if err != nil {
		t.Fatal(err)
	}
	if val == nil || len(val) != 0 {
		t.Fatalf("value=%v, want non-nil empty value", val)
	}
	if b.CountEntries(tableA) != 1 {
		t.Fatalf("tableA entries=%d, want 1", b.CountEntries(tableA))
	}
}

func TestInMemoryDelete(t *testing.T) {
	b := NewInMemoryBackend()
	wb, _ := b.BeginWrite()
	if err := wb.PutBatch(tableB, []KV{
		{Key: []byte("k1"), Value: []byte("v1")},
		{Key: []byte("k2"), Value: []byte("v2")},
	}); err != nil {
		t.Fatal(err)
	}
	if err := wb.Commit(); err != nil {
		t.Fatal(err)
	}

	if b.CountEntries(tableB) != 2 {
		t.Fatal("expected 2 entries")
	}

	wb2, _ := b.BeginWrite()
	if err := wb2.DeleteBatch(tableB, [][]byte{[]byte("k1")}); err != nil {
		t.Fatal(err)
	}
	if err := wb2.Commit(); err != nil {
		t.Fatal(err)
	}

	if b.CountEntries(tableB) != 1 {
		t.Fatal("expected 1 entry after delete")
	}
}

func TestInMemoryPrefixIterator(t *testing.T) {
	b := NewInMemoryBackend()
	wb, _ := b.BeginWrite()
	if err := wb.PutBatch(tableD, []KV{
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
	it, err := rv.PrefixIterator(tableD, []byte("aa"))
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
	if err := wb.PutBatch(tableA, []KV{{Key: []byte("key"), Value: []byte("value")}}); err != nil {
		t.Fatal(err)
	}
	if err := wb.Commit(); err != nil {
		t.Fatal(err)
	}

	rv, _ := b.BeginRead()
	val, err := rv.Get(tableA, []byte("key"))
	if err != nil {
		t.Fatal(err)
	}
	val[0] = 'X'

	fresh, err := rv.Get(tableA, []byte("key"))
	if err != nil {
		t.Fatal(err)
	}
	if string(fresh) != "value" {
		t.Fatalf("stored value mutated through Get result: %q", string(fresh))
	}

	it, err := rv.PrefixIterator(tableA, []byte("key"))
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
	if err := wb.PutBatch(tableA, []KV{{Key: nil, Value: []byte("empty")}}); err != nil {
		t.Fatal(err)
	}
	if err := wb.Commit(); err != nil {
		t.Fatal(err)
	}

	rv, _ := b.BeginRead()
	it, err := rv.PrefixIterator(tableA, nil)
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
	if err := wb.PutBatch(tableA, []KV{
		{Key: []byte("key"), Value: []byte("val")},
	}); err != nil {
		t.Fatal(err)
	}

	rv, _ := b.BeginRead()
	val, _ := rv.Get(tableA, []byte("key"))
	if val != nil {
		t.Fatal("uncommitted write should not be visible")
	}

	if err := wb.Commit(); err != nil {
		t.Fatal(err)
	}
	rv2, _ := b.BeginRead()
	val2, _ := rv2.Get(tableA, []byte("key"))
	if string(val2) != "val" {
		t.Fatal("committed write should be visible")
	}
}

func TestInMemoryTableIsolation(t *testing.T) {
	b := NewInMemoryBackend()
	wb, _ := b.BeginWrite()
	if err := wb.PutBatch(tableC, []KV{
		{Key: []byte("root"), Value: []byte("header")},
	}); err != nil {
		t.Fatal(err)
	}
	if err := wb.Commit(); err != nil {
		t.Fatal(err)
	}

	rv, _ := b.BeginRead()
	val, _ := rv.Get(tableB, []byte("root"))
	if val != nil {
		t.Fatal("tables should be isolated")
	}
}

func TestEstimateTableBytes(t *testing.T) {
	b := NewInMemoryBackend()
	wb, _ := b.BeginWrite()
	if err := wb.PutBatch(tableA, []KV{
		{Key: []byte("k"), Value: []byte("value")},
	}); err != nil {
		t.Fatal(err)
	}
	if err := wb.Commit(); err != nil {
		t.Fatal(err)
	}

	size := b.EstimateTableBytes(tableA)
	if size == 0 {
		t.Fatal("should report non-zero size")
	}
}

func TestInMemoryWriteBatchClosedAfterCommit(t *testing.T) {
	b := NewInMemoryBackend()
	wb, _ := b.BeginWrite()
	if err := wb.PutBatch(tableA, []KV{{Key: []byte("key"), Value: []byte("value")}}); err != nil {
		t.Fatal(err)
	}
	if err := wb.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := wb.PutBatch(tableA, []KV{{Key: []byte("next"), Value: []byte("value")}}); err != errBatchClosed {
		t.Fatalf("PutBatch after commit error=%v, want %v", err, errBatchClosed)
	}
	if err := wb.DeleteBatch(tableA, [][]byte{[]byte("key")}); err != errBatchClosed {
		t.Fatalf("DeleteBatch after commit error=%v, want %v", err, errBatchClosed)
	}
	if err := wb.Commit(); err != errBatchClosed {
		t.Fatalf("Commit after commit error=%v, want %v", err, errBatchClosed)
	}
}
