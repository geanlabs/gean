package storage

import (
	"bytes"
	"fmt"
	"path/filepath"

	"github.com/cockroachdb/pebble"
)

// PebbleBackend is a persistent storage backend using CockroachDB's Pebble.
// Follows the same pattern as ethlambda's RocksDB backend.
//
// Pebble doesn't have column families, so we prefix keys with the table name
// to achieve table isolation: "{table_name}\x00{key}".
type PebbleBackend struct {
	db *pebble.DB
}

// NewPebbleBackend opens or creates a Pebble database at the given path.
func NewPebbleBackend(dir string) (*PebbleBackend, error) {
	db, err := pebble.Open(filepath.Clean(dir), &pebble.Options{})
	if err != nil {
		return nil, fmt.Errorf("pebble open: %w", err)
	}
	return &PebbleBackend{db: db}, nil
}

func (p *PebbleBackend) BeginRead() (ReadView, error) {
	return &pebbleReadView{db: p.db}, nil
}

func (p *PebbleBackend) BeginWrite() (WriteBatch, error) {
	return &pebbleWriteBatch{db: p.db, batch: p.db.NewBatch()}, nil
}

func (p *PebbleBackend) EstimateTableBytes(table Table) uint64 {
	// Pebble doesn't have per-prefix size estimates like RocksDB column families.
	return 0
}

func (p *PebbleBackend) Close() error {
	return p.db.Close()
}

// tableKey creates a prefixed key: "{table}\x00{key}".
func tableKey(table Table, key []byte) []byte {
	prefix := []byte(table)
	result := make([]byte, len(prefix)+1+len(key))
	copy(result, prefix)
	result[len(prefix)] = 0x00
	copy(result[len(prefix)+1:], key)
	return result
}

// tablePrefix returns the prefix for all keys in a table: "{table}\x00".
func tablePrefix(table Table) []byte {
	prefix := []byte(table)
	result := make([]byte, len(prefix)+1)
	copy(result, prefix)
	result[len(prefix)] = 0x00
	return result
}

// stripTablePrefix removes the table prefix from a key.
func stripTablePrefix(table Table, fullKey []byte) []byte {
	prefixLen := len([]byte(table)) + 1
	if len(fullKey) <= prefixLen {
		return nil
	}
	return fullKey[prefixLen:]
}

// --- ReadView ---

type pebbleReadView struct {
	db *pebble.DB
}

func (v *pebbleReadView) Get(table Table, key []byte) ([]byte, error) {
	val, closer, err := v.db.Get(tableKey(table, key))
	if err == pebble.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer closer.Close()
	cp := make([]byte, len(val))
	copy(cp, val)
	return cp, nil
}

func (v *pebbleReadView) PrefixIterator(table Table, prefix []byte) (Iterator, error) {
	fullPrefix := tableKey(table, prefix)
	iter, err := v.db.NewIter(&pebble.IterOptions{
		LowerBound: fullPrefix,
		UpperBound: prefixUpperBound(fullPrefix),
	})
	if err != nil {
		return nil, err
	}
	iter.First()
	return &pebbleIterator{iter: iter, table: table}, nil
}

// prefixUpperBound computes the upper bound for a prefix scan.
// Increments the last byte; if it overflows, the prefix has no upper bound.
func prefixUpperBound(prefix []byte) []byte {
	if len(prefix) == 0 {
		return nil
	}
	upper := make([]byte, len(prefix))
	copy(upper, prefix)
	for i := len(upper) - 1; i >= 0; i-- {
		upper[i]++
		if upper[i] != 0 {
			return upper
		}
	}
	return nil // all 0xFF — no upper bound
}

// --- WriteBatch ---

type pebbleWriteBatch struct {
	db    *pebble.DB
	batch *pebble.Batch
}

func (b *pebbleWriteBatch) PutBatch(table Table, entries []KV) error {
	for _, e := range entries {
		if err := b.batch.Set(tableKey(table, e.Key), e.Value, nil); err != nil {
			return err
		}
	}
	return nil
}

func (b *pebbleWriteBatch) DeleteBatch(table Table, keys [][]byte) error {
	for _, k := range keys {
		if err := b.batch.Delete(tableKey(table, k), nil); err != nil {
			return err
		}
	}
	return nil
}

func (b *pebbleWriteBatch) Commit() error {
	return b.batch.Commit(pebble.NoSync)
}

// --- Iterator ---

type pebbleIterator struct {
	iter    *pebble.Iterator
	table   Table
	started bool
}

func (it *pebbleIterator) Next() bool {
	if !it.started {
		it.started = true
		return it.iter.Valid()
	}
	if !it.iter.Valid() {
		return false
	}
	it.iter.Next()
	return it.iter.Valid()
}

func (it *pebbleIterator) Key() []byte {
	if !it.iter.Valid() {
		return nil
	}
	return stripTablePrefix(it.table, bytes.Clone(it.iter.Key()))
}

func (it *pebbleIterator) Value() []byte {
	if !it.iter.Valid() {
		return nil
	}
	return bytes.Clone(it.iter.Value())
}

func (it *pebbleIterator) Close() {
	it.iter.Close()
}
