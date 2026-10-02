package storage

import "testing"

// Has must agree with Get on presence for both backends, including after a
// delete and for a key in a table that was never written.
func TestHasMatchesGet(t *testing.T) {
	pebbleBackend, err := NewPebbleBackend(t.TempDir())
	if err != nil {
		t.Fatalf("open pebble: %v", err)
	}
	t.Cleanup(func() { pebbleBackend.Close() })

	for name, backend := range map[string]Backend{"memory": NewInMemoryBackend(), "pebble": pebbleBackend} {
		wb, err := backend.BeginWrite()
		if err != nil {
			t.Fatalf("%s: begin write: %v", name, err)
		}
		if err := wb.PutBatch(TableStates, []KV{{Key: []byte("kept"), Value: []byte("state")}, {Key: []byte("gone"), Value: []byte("state")}}); err != nil {
			t.Fatalf("%s: put: %v", name, err)
		}
		if err := wb.Commit(); err != nil {
			t.Fatalf("%s: commit: %v", name, err)
		}
		wb, _ = backend.BeginWrite()
		if err := wb.DeleteBatch(TableStates, [][]byte{[]byte("gone")}); err != nil {
			t.Fatalf("%s: delete: %v", name, err)
		}
		if err := wb.Commit(); err != nil {
			t.Fatalf("%s: commit delete: %v", name, err)
		}

		rv, err := backend.BeginRead()
		if err != nil {
			t.Fatalf("%s: begin read: %v", name, err)
		}
		for _, c := range []struct {
			table Table
			key   string
		}{{TableStates, "kept"}, {TableStates, "gone"}, {TableStates, "never"}, {TableBlockHeaders, "kept"}} {
			val, err := rv.Get(c.table, []byte(c.key))
			if err != nil {
				t.Fatalf("%s: get %s/%s: %v", name, c.table, c.key, err)
			}
			has, err := rv.Has(c.table, []byte(c.key))
			if err != nil {
				t.Fatalf("%s: has %s/%s: %v", name, c.table, c.key, err)
			}
			if has != (val != nil) {
				t.Fatalf("%s: %s/%s: Has=%t but Get found=%t", name, c.table, c.key, has, val != nil)
			}
		}
	}
}
