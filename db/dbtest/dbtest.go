// Package dbtest is the conformance suite every db.Backend must pass, so the
// backends stay interchangeable under the consensus store.
package dbtest

import (
	"bytes"
	"testing"

	"github.com/geanlabs/gean/db"
)

// Run checks the backends that open returns against the db.Backend contract.
// open must return a fresh, empty backend each call.
func Run(t *testing.T, open func(t *testing.T) db.Backend) {
	t.Helper()
	t.Run("PutAndGet", func(t *testing.T) {
		b := open(t)
		wb, _ := b.BeginWrite()
		if err := wb.PutBatch(db.TableBlockHeaders, []db.KV{
			{Key: []byte("root1"), Value: []byte("header1")},
			{Key: []byte("root2"), Value: []byte("header2")},
		}); err != nil {
			t.Fatal(err)
		}
		if err := wb.Commit(); err != nil {
			t.Fatal(err)
		}

		rv, _ := b.BeginRead()
		val, err := rv.Get(db.TableBlockHeaders, []byte("root1"))
		if err != nil {
			t.Fatal(err)
		}
		if string(val) != "header1" {
			t.Fatalf("expected header1, got %s", string(val))
		}

		val, err = rv.Get(db.TableBlockHeaders, []byte("root2"))
		if err != nil {
			t.Fatal(err)
		}
		if string(val) != "header2" {
			t.Fatalf("expected header2, got %s", string(val))
		}
	})

	t.Run("GetMissing", func(t *testing.T) {
		b := open(t)
		rv, _ := b.BeginRead()
		val, err := rv.Get(db.TableBlockHeaders, []byte("nonexistent"))
		if err != nil {
			t.Fatal(err)
		}
		if val != nil {
			t.Fatal("expected nil for missing key")
		}
	})

	t.Run("PutNilValueStoresEmptyValue", func(t *testing.T) {
		b := open(t)
		wb, _ := b.BeginWrite()
		if err := wb.PutBatch(db.TableMetadata, []db.KV{{Key: []byte("key"), Value: nil}}); err != nil {
			t.Fatal(err)
		}
		if err := wb.Commit(); err != nil {
			t.Fatal(err)
		}

		rv, _ := b.BeginRead()
		val, err := rv.Get(db.TableMetadata, []byte("key"))
		if err != nil {
			t.Fatal(err)
		}
		if val == nil || len(val) != 0 {
			t.Fatalf("value=%v, want non-nil empty value", val)
		}
		if countEntries(t, b, db.TableMetadata) != 1 {
			t.Fatalf("metadata entries=%d, want 1", countEntries(t, b, db.TableMetadata))
		}
	})

	t.Run("Delete", func(t *testing.T) {
		b := open(t)
		wb, _ := b.BeginWrite()
		if err := wb.PutBatch(db.TableStates, []db.KV{
			{Key: []byte("k1"), Value: []byte("v1")},
			{Key: []byte("k2"), Value: []byte("v2")},
		}); err != nil {
			t.Fatal(err)
		}
		if err := wb.Commit(); err != nil {
			t.Fatal(err)
		}

		if countEntries(t, b, db.TableStates) != 2 {
			t.Fatal("expected 2 entries")
		}

		wb2, _ := b.BeginWrite()
		if err := wb2.DeleteBatch(db.TableStates, [][]byte{[]byte("k1")}); err != nil {
			t.Fatal(err)
		}
		if err := wb2.Commit(); err != nil {
			t.Fatal(err)
		}

		if countEntries(t, b, db.TableStates) != 1 {
			t.Fatal("expected 1 entry after delete")
		}
	})

	t.Run("PrefixIterator", func(t *testing.T) {
		b := open(t)
		wb, _ := b.BeginWrite()
		if err := wb.PutBatch(db.TableLiveChain, []db.KV{
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
		it, err := rv.PrefixIterator(db.TableLiveChain, []byte("aa"))
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
	})

	t.Run("ReadResultsAreCallerOwned", func(t *testing.T) {
		b := open(t)
		wb, _ := b.BeginWrite()
		if err := wb.PutBatch(db.TableMetadata, []db.KV{{Key: []byte("key"), Value: []byte("value")}}); err != nil {
			t.Fatal(err)
		}
		if err := wb.Commit(); err != nil {
			t.Fatal(err)
		}

		rv, _ := b.BeginRead()
		val, err := rv.Get(db.TableMetadata, []byte("key"))
		if err != nil {
			t.Fatal(err)
		}
		val[0] = 'X'

		fresh, err := rv.Get(db.TableMetadata, []byte("key"))
		if err != nil {
			t.Fatal(err)
		}
		if string(fresh) != "value" {
			t.Fatalf("stored value mutated through Get result: %q", string(fresh))
		}

		it, err := rv.PrefixIterator(db.TableMetadata, []byte("key"))
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
	})

	t.Run("PrefixIteratorSupportsEmptyKey", func(t *testing.T) {
		b := open(t)
		wb, _ := b.BeginWrite()
		if err := wb.PutBatch(db.TableMetadata, []db.KV{{Key: nil, Value: []byte("empty")}}); err != nil {
			t.Fatal(err)
		}
		if err := wb.Commit(); err != nil {
			t.Fatal(err)
		}

		rv, _ := b.BeginRead()
		it, err := rv.PrefixIterator(db.TableMetadata, nil)
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
	})

	t.Run("AtomicCommit", func(t *testing.T) {
		b := open(t)

		wb, _ := b.BeginWrite()
		if err := wb.PutBatch(db.TableMetadata, []db.KV{
			{Key: []byte("key"), Value: []byte("val")},
		}); err != nil {
			t.Fatal(err)
		}

		rv, _ := b.BeginRead()
		val, _ := rv.Get(db.TableMetadata, []byte("key"))
		if val != nil {
			t.Fatal("uncommitted write should not be visible")
		}

		if err := wb.Commit(); err != nil {
			t.Fatal(err)
		}
		rv2, _ := b.BeginRead()
		val2, _ := rv2.Get(db.TableMetadata, []byte("key"))
		if string(val2) != "val" {
			t.Fatal("committed write should be visible")
		}
	})

	t.Run("TableIsolation", func(t *testing.T) {
		b := open(t)
		wb, _ := b.BeginWrite()
		if err := wb.PutBatch(db.TableBlockHeaders, []db.KV{
			{Key: []byte("root"), Value: []byte("header")},
		}); err != nil {
			t.Fatal(err)
		}
		if err := wb.Commit(); err != nil {
			t.Fatal(err)
		}

		rv, _ := b.BeginRead()
		val, _ := rv.Get(db.TableStates, []byte("root"))
		if val != nil {
			t.Fatal("tables should be isolated")
		}
	})

	t.Run("WriteBatchClosedAfterCommit", func(t *testing.T) {
		b := open(t)
		wb, _ := b.BeginWrite()
		if err := wb.PutBatch(db.TableMetadata, []db.KV{{Key: []byte("key"), Value: []byte("value")}}); err != nil {
			t.Fatal(err)
		}
		if err := wb.Commit(); err != nil {
			t.Fatal(err)
		}
		if err := wb.PutBatch(db.TableMetadata, []db.KV{{Key: []byte("next"), Value: []byte("value")}}); err != db.ErrBatchClosed {
			t.Fatalf("PutBatch after commit error=%v, want %v", err, db.ErrBatchClosed)
		}
		if err := wb.DeleteBatch(db.TableMetadata, [][]byte{[]byte("key")}); err != db.ErrBatchClosed {
			t.Fatalf("DeleteBatch after commit error=%v, want %v", err, db.ErrBatchClosed)
		}
		if err := wb.Commit(); err != db.ErrBatchClosed {
			t.Fatalf("Commit after commit error=%v, want %v", err, db.ErrBatchClosed)
		}
	})
}

// countEntries counts a table's entries through the contract's iterator.
func countEntries(t *testing.T, b db.Backend, table db.Table) int {
	t.Helper()
	rv, err := b.BeginRead()
	if err != nil {
		t.Fatal(err)
	}
	it, err := rv.PrefixIterator(table, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer it.Close()
	n := 0
	for it.Next() {
		n++
	}
	if err := it.Err(); err != nil {
		t.Fatal(err)
	}
	return n
}
