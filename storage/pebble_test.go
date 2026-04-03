package storage

import (
	"bytes"
	"os"
	"testing"
)

func tempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "geany-pebble-test-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func TestPebblePutAndGet(t *testing.T) {
	b, err := NewPebbleBackend(tempDir(t))
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	wb, _ := b.BeginWrite()
	wb.PutBatch(TableBlockHeaders, []KV{
		{Key: []byte("root1"), Value: []byte("header1")},
		{Key: []byte("root2"), Value: []byte("header2")},
	})
	wb.Commit()

	rv, _ := b.BeginRead()
	val, _ := rv.Get(TableBlockHeaders, []byte("root1"))
	if string(val) != "header1" {
		t.Fatalf("expected header1, got %s", string(val))
	}
	val, _ = rv.Get(TableBlockHeaders, []byte("root2"))
	if string(val) != "header2" {
		t.Fatalf("expected header2, got %s", string(val))
	}
}

func TestPebbleGetMissing(t *testing.T) {
	b, err := NewPebbleBackend(tempDir(t))
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	rv, _ := b.BeginRead()
	val, _ := rv.Get(TableBlockHeaders, []byte("nonexistent"))
	if val != nil {
		t.Fatal("expected nil for missing key")
	}
}

func TestPebbleDelete(t *testing.T) {
	b, err := NewPebbleBackend(tempDir(t))
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	wb, _ := b.BeginWrite()
	wb.PutBatch(TableStates, []KV{
		{Key: []byte("k1"), Value: []byte("v1")},
		{Key: []byte("k2"), Value: []byte("v2")},
	})
	wb.Commit()

	wb2, _ := b.BeginWrite()
	wb2.DeleteBatch(TableStates, [][]byte{[]byte("k1")})
	wb2.Commit()

	rv, _ := b.BeginRead()
	val, _ := rv.Get(TableStates, []byte("k1"))
	if val != nil {
		t.Fatal("k1 should be deleted")
	}
	val, _ = rv.Get(TableStates, []byte("k2"))
	if string(val) != "v2" {
		t.Fatal("k2 should still exist")
	}
}

func TestPebbleTableIsolation(t *testing.T) {
	b, err := NewPebbleBackend(tempDir(t))
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	wb, _ := b.BeginWrite()
	wb.PutBatch(TableBlockHeaders, []KV{
		{Key: []byte("root"), Value: []byte("header")},
	})
	wb.Commit()

	rv, _ := b.BeginRead()
	val, _ := rv.Get(TableStates, []byte("root"))
	if val != nil {
		t.Fatal("tables should be isolated")
	}
}

func TestPebblePrefixIterator(t *testing.T) {
	b, err := NewPebbleBackend(tempDir(t))
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

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

func TestPebblePersistence(t *testing.T) {
	dir := tempDir(t)

	// Write data.
	{
		b, err := NewPebbleBackend(dir)
		if err != nil {
			t.Fatal(err)
		}
		wb, _ := b.BeginWrite()
		wb.PutBatch(TableMetadata, []KV{
			{Key: []byte("key"), Value: []byte("value")},
		})
		wb.Commit()
		b.Close()
	}

	// Reopen and read.
	{
		b, err := NewPebbleBackend(dir)
		if err != nil {
			t.Fatal(err)
		}
		defer b.Close()
		rv, _ := b.BeginRead()
		val, _ := rv.Get(TableMetadata, []byte("key"))
		if string(val) != "value" {
			t.Fatalf("expected value after reopen, got %s", string(val))
		}
	}
}
