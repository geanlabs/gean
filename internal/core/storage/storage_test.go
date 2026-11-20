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

// forEachBackend runs test against a fresh in-memory and a fresh Pebble backend,
// so both are held to the same contract.
func forEachBackend(t *testing.T, test func(t *testing.T, b Backend)) {
	t.Run("memory", func(t *testing.T) { test(t, NewInMemoryBackend()) })
	t.Run("pebble", func(t *testing.T) {
		b, err := NewPebbleBackend(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { b.Close() })
		test(t, b)
	})
}

func put(t *testing.T, b Backend, table Table, kvs ...KV) {
	t.Helper()
	wb, err := b.BeginWrite()
	if err != nil {
		t.Fatal(err)
	}
	if err := wb.PutBatch(table, kvs); err != nil {
		t.Fatal(err)
	}
	if err := wb.Commit(); err != nil {
		t.Fatal(err)
	}
}

func get(t *testing.T, b Backend, table Table, key string) []byte {
	t.Helper()
	rv, err := b.BeginRead()
	if err != nil {
		t.Fatal(err)
	}
	val, err := rv.Get(table, []byte(key))
	if err != nil {
		t.Fatal(err)
	}
	return val
}

func kv(key, value string) KV { return KV{Key: []byte(key), Value: []byte(value)} }

func TestPutAndGet(t *testing.T) {
	forEachBackend(t, func(t *testing.T, b Backend) {
		put(t, b, tableC, kv("root1", "header1"), kv("root2", "header2"))
		if got := get(t, b, tableC, "root1"); string(got) != "header1" {
			t.Fatalf("root1=%q, want header1", got)
		}
		if got := get(t, b, tableC, "root2"); string(got) != "header2" {
			t.Fatalf("root2=%q, want header2", got)
		}
		if got := get(t, b, tableC, "missing"); got != nil {
			t.Fatalf("missing=%q, want nil", got)
		}
	})
}

func TestPutNilValueStoresEmptyValue(t *testing.T) {
	forEachBackend(t, func(t *testing.T, b Backend) {
		put(t, b, tableA, KV{Key: []byte("key"), Value: nil})
		if val := get(t, b, tableA, "key"); val == nil || len(val) != 0 {
			t.Fatalf("value=%v, want non-nil empty value", val)
		}
	})
}

func TestDelete(t *testing.T) {
	forEachBackend(t, func(t *testing.T, b Backend) {
		put(t, b, tableB, kv("k1", "v1"), kv("k2", "v2"))
		wb, _ := b.BeginWrite()
		if err := wb.DeleteBatch(tableB, [][]byte{[]byte("k1")}); err != nil {
			t.Fatal(err)
		}
		if err := wb.Commit(); err != nil {
			t.Fatal(err)
		}
		if get(t, b, tableB, "k1") != nil {
			t.Fatal("k1 should be deleted")
		}
		if string(get(t, b, tableB, "k2")) != "v2" {
			t.Fatal("k2 should still exist")
		}
	})
}

// Has must agree with Get on presence, including after a delete and for a key
// in a table that was never written.
func TestHasMatchesGet(t *testing.T) {
	forEachBackend(t, func(t *testing.T, b Backend) {
		put(t, b, tableB, kv("kept", "state"), kv("gone", "state"))
		wb, _ := b.BeginWrite()
		if err := wb.DeleteBatch(tableB, [][]byte{[]byte("gone")}); err != nil {
			t.Fatal(err)
		}
		if err := wb.Commit(); err != nil {
			t.Fatal(err)
		}

		rv, err := b.BeginRead()
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range []struct {
			table Table
			key   string
		}{{tableB, "kept"}, {tableB, "gone"}, {tableB, "never"}, {tableC, "kept"}} {
			val, err := rv.Get(c.table, []byte(c.key))
			if err != nil {
				t.Fatalf("get %s/%s: %v", c.table, c.key, err)
			}
			has, err := rv.Has(c.table, []byte(c.key))
			if err != nil {
				t.Fatalf("has %s/%s: %v", c.table, c.key, err)
			}
			if has != (val != nil) {
				t.Fatalf("%s/%s: Has=%t but Get found=%t", c.table, c.key, has, val != nil)
			}
		}
	})
}

func TestTableIsolation(t *testing.T) {
	forEachBackend(t, func(t *testing.T, b Backend) {
		put(t, b, tableC, kv("root", "header"))
		if get(t, b, tableB, "root") != nil {
			t.Fatal("tables should be isolated")
		}
	})
}

func TestUncommittedWriteIsInvisible(t *testing.T) {
	forEachBackend(t, func(t *testing.T, b Backend) {
		wb, _ := b.BeginWrite()
		if err := wb.PutBatch(tableA, []KV{kv("key", "val")}); err != nil {
			t.Fatal(err)
		}
		if get(t, b, tableA, "key") != nil {
			t.Fatal("uncommitted write should not be visible")
		}
		if err := wb.Commit(); err != nil {
			t.Fatal(err)
		}
		if string(get(t, b, tableA, "key")) != "val" {
			t.Fatal("committed write should be visible")
		}
	})
}

func TestPrefixIterator(t *testing.T) {
	forEachBackend(t, func(t *testing.T, b Backend) {
		put(t, b, tableD, kv("aa_1", "v1"), kv("aa_2", "v2"), kv("bb_1", "v3"))
		rv, _ := b.BeginRead()
		it, err := rv.PrefixIterator(tableD, []byte("aa"))
		if err != nil {
			t.Fatal(err)
		}
		defer it.Close()

		count := 0
		for it.Next() {
			if !bytes.HasPrefix(it.Key(), []byte("aa")) {
				t.Fatalf("key %s doesn't have prefix aa", it.Key())
			}
			count++
		}
		if count != 2 {
			t.Fatalf("got %d entries with prefix aa, want 2", count)
		}
	})
}

func TestPrefixIteratorSupportsEmptyKey(t *testing.T) {
	forEachBackend(t, func(t *testing.T, b Backend) {
		put(t, b, tableA, KV{Key: nil, Value: []byte("empty")})
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
			t.Fatalf("value=%q, want empty", it.Value())
		}
	})
}

func TestReadResultsAreCallerOwned(t *testing.T) {
	forEachBackend(t, func(t *testing.T, b Backend) {
		put(t, b, tableA, kv("key", "value"))
		get(t, b, tableA, "key")[0] = 'X'
		if got := get(t, b, tableA, "key"); string(got) != "value" {
			t.Fatalf("stored value mutated through Get result: %q", got)
		}

		rv, _ := b.BeginRead()
		it, err := rv.PrefixIterator(tableA, []byte("key"))
		if err != nil {
			t.Fatal(err)
		}
		defer it.Close()
		if !it.Next() {
			t.Fatal("expected iterator entry")
		}
		it.Key()[0] = 'X'
		it.Value()[0] = 'X'
		if string(it.Key()) != "key" || string(it.Value()) != "value" {
			t.Fatal("iterator key/value should be caller-owned copies")
		}
	})
}

func TestWriteBatchClosedAfterCommit(t *testing.T) {
	forEachBackend(t, func(t *testing.T, b Backend) {
		wb, _ := b.BeginWrite()
		if err := wb.PutBatch(tableA, []KV{kv("key", "value")}); err != nil {
			t.Fatal(err)
		}
		if err := wb.Commit(); err != nil {
			t.Fatal(err)
		}
		if err := wb.PutBatch(tableA, []KV{kv("next", "value")}); err != errBatchClosed {
			t.Fatalf("PutBatch after commit error=%v, want %v", err, errBatchClosed)
		}
		if err := wb.DeleteBatch(tableA, [][]byte{[]byte("key")}); err != errBatchClosed {
			t.Fatalf("DeleteBatch after commit error=%v, want %v", err, errBatchClosed)
		}
		if err := wb.Commit(); err != errBatchClosed {
			t.Fatalf("Commit after commit error=%v, want %v", err, errBatchClosed)
		}
	})
}

// The in-memory backend counts entries directly, so a write shows at once.
// Pebble's estimate needs a flush; see TestPebbleEstimateTableBytes.
func TestInMemoryEstimateTableBytes(t *testing.T) {
	b := NewInMemoryBackend()
	put(t, b, tableA, kv("k", "value"))
	if b.EstimateTableBytes(tableA) == 0 {
		t.Fatal("should report non-zero size")
	}
}
