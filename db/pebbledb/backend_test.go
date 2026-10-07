package pebbledb

import (
	"os"
	"testing"

	"github.com/geanlabs/gean/db"
	"github.com/geanlabs/gean/db/dbtest"
)

func TestPebbleBackendConformance(t *testing.T) {
	dbtest.Run(t, func(t *testing.T) db.Backend {
		b, err := Open(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { b.Close() })
		return b
	})
}

func tempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "gean-pebble-test-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func TestPebblePersistence(t *testing.T) {
	dir := tempDir(t)

	{
		b, err := Open(dir)
		if err != nil {
			t.Fatal(err)
		}
		wb, _ := b.BeginWrite()
		if err := wb.PutBatch(db.TableMetadata, []db.KV{
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
		b, err := Open(dir)
		if err != nil {
			t.Fatal(err)
		}
		defer b.Close()
		rv, _ := b.BeginRead()
		val, _ := rv.Get(db.TableMetadata, []byte("key"))
		if string(val) != "value" {
			t.Fatalf("expected value after reopen, got %s", string(val))
		}
	}
}

func TestPebbleEstimateTableBytes(t *testing.T) {
	b, err := Open(tempDir(t))
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	wb, _ := b.BeginWrite()
	if err := wb.PutBatch(db.TableMetadata, []db.KV{
		{Key: []byte("k"), Value: []byte("value")},
	}); err != nil {
		t.Fatal(err)
	}
	if err := wb.PutBatch(db.TableStates, []db.KV{
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
	if size := b.EstimateTableBytes(db.TableMetadata); size != 0 {
		t.Fatalf("unflushed metadata size=%d, want 0", size)
	}

	if err := b.db.Flush(); err != nil {
		t.Fatal(err)
	}

	metaSize := b.EstimateTableBytes(db.TableMetadata)
	if metaSize == 0 {
		t.Fatal("metadata size=0 after flush, want non-zero")
	}

	// Each table is estimated over its own prefix range, so a write to one must
	// not be attributed to another. db.TableBlockHeaders was never written.
	if size := b.EstimateTableBytes(db.TableBlockHeaders); size != 0 {
		t.Fatalf("unwritten table size=%d, want 0", size)
	}
	if statesSize := b.EstimateTableBytes(db.TableStates); statesSize == 0 {
		t.Fatal("states size=0 after flush, want non-zero")
	}
}
