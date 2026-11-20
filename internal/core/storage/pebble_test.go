package storage

import "testing"

func TestPebblePersistence(t *testing.T) {
	dir := t.TempDir()

	{
		b, err := NewPebbleBackend(dir)
		if err != nil {
			t.Fatal(err)
		}
		wb, _ := b.BeginWrite()
		if err := wb.PutBatch(tableA, []KV{
			{Key: []byte("key"), Value: []byte("value")},
		}); err != nil {
			t.Fatal(err)
		}
		if err := wb.Commit(); err != nil {
			t.Fatal(err)
		}
		if err := b.Close(); err != nil {
			t.Fatal(err)
		}
	}

	{
		b, err := NewPebbleBackend(dir)
		if err != nil {
			t.Fatal(err)
		}
		defer b.Close()
		rv, _ := b.BeginRead()
		val, _ := rv.Get(tableA, []byte("key"))
		if string(val) != "value" {
			t.Fatalf("expected value after reopen, got %s", string(val))
		}
	}
}

func TestPebbleEstimateTableBytes(t *testing.T) {
	b, err := NewPebbleBackend(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	wb, _ := b.BeginWrite()
	if err := wb.PutBatch(tableA, []KV{
		{Key: []byte("k"), Value: []byte("value")},
	}); err != nil {
		t.Fatal(err)
	}
	if err := wb.PutBatch(tableB, []KV{
		{Key: []byte("k"), Value: []byte("larger-value")},
	}); err != nil {
		t.Fatal(err)
	}
	if err := wb.Commit(); err != nil {
		t.Fatal(err)
	}

	// EstimateTableBytes reports SST bytes for the table's key range, so it sees
	// nothing until the memtable is flushed. That is the documented contract:
	// the gauge is coarse and sampled on a slow cadence, and the alternative —
	// summing every key and value — is a full-table scan.
	if size := b.EstimateTableBytes(tableA); size != 0 {
		t.Fatalf("unflushed tableA size=%d, want 0", size)
	}

	if err := b.db.Flush(); err != nil {
		t.Fatal(err)
	}

	metaSize := b.EstimateTableBytes(tableA)
	if metaSize == 0 {
		t.Fatal("tableA size=0 after flush, want non-zero")
	}

	// Each table is estimated over its own prefix range, so a write to one must
	// not be attributed to another. tableC was never written.
	if size := b.EstimateTableBytes(tableC); size != 0 {
		t.Fatalf("unwritten table size=%d, want 0", size)
	}
	if statesSize := b.EstimateTableBytes(tableB); statesSize == 0 {
		t.Fatal("tableB size=0 after flush, want non-zero")
	}
}
