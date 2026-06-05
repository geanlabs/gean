package storage

import (
	"bytes"
	"fmt"
	"path/filepath"

	"github.com/cockroachdb/pebble"
)

type PebbleBackend struct {
	db *pebble.DB
}

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
	return &pebbleWriteBatch{batch: p.db.NewBatch()}, nil
}

func (p *PebbleBackend) EstimateTableBytes(table Table) uint64 {
	rv, err := p.BeginRead()
	if err != nil {
		return 0
	}
	it, err := rv.PrefixIterator(table, nil)
	if err != nil {
		return 0
	}
	defer it.Close()

	var total uint64
	for it.Next() {
		total += uint64(len(it.Key()) + len(it.Value()))
	}
	return total
}

func (p *PebbleBackend) Close() error {
	return p.db.Close()
}

func tableKey(table Table, key []byte) []byte {
	prefix := []byte(table)
	result := make([]byte, len(prefix)+1+len(key))
	copy(result, prefix)
	result[len(prefix)] = 0x00
	copy(result[len(prefix)+1:], key)
	return result
}

func stripTablePrefix(table Table, fullKey []byte) []byte {
	prefixLen := len([]byte(table)) + 1
	if len(fullKey) < prefixLen {
		return nil
	}
	return fullKey[prefixLen:]
}

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
	return bytes.Clone(val), nil
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
	return nil
}

type pebbleWriteBatch struct {
	batch  *pebble.Batch
	closed bool
}

func (b *pebbleWriteBatch) PutBatch(table Table, entries []KV) error {
	if b.closed {
		return errBatchClosed
	}
	for _, e := range entries {
		if err := b.batch.Set(tableKey(table, e.Key), e.Value, nil); err != nil {
			return err
		}
	}
	return nil
}

func (b *pebbleWriteBatch) DeleteBatch(table Table, keys [][]byte) error {
	if b.closed {
		return errBatchClosed
	}
	for _, k := range keys {
		if err := b.batch.Delete(tableKey(table, k), nil); err != nil {
			return err
		}
	}
	return nil
}

func (b *pebbleWriteBatch) Commit() error {
	if b.closed {
		return errBatchClosed
	}
	err := b.batch.Commit(pebble.NoSync)
	closeErr := b.batch.Close()
	b.closed = true
	if err != nil {
		return err
	}
	return closeErr
}

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
