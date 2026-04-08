package storage

import (
	"bytes"
	"sort"
	"sync"
)

// InMemoryBackend is a thread-safe in-memory storage backend for tests.
type InMemoryBackend struct {
	mu     sync.RWMutex
	tables map[Table]map[string][]byte
}

// NewInMemoryBackend creates a new in-memory backend with all tables initialized.
func NewInMemoryBackend() *InMemoryBackend {
	tables := make(map[Table]map[string][]byte)
	for _, t := range AllTables {
		tables[t] = make(map[string][]byte)
	}
	return &InMemoryBackend{tables: tables}
}

func (b *InMemoryBackend) BeginRead() (ReadView, error) {
	return &inMemoryReadView{backend: b}, nil
}

func (b *InMemoryBackend) BeginWrite() (WriteBatch, error) {
	return &inMemoryWriteBatch{backend: b}, nil
}

func (b *InMemoryBackend) EstimateTableBytes(table Table) uint64 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	t, ok := b.tables[table]
	if !ok {
		return 0
	}
	var total uint64
	for k, v := range t {
		total += uint64(len(k) + len(v))
	}
	return total
}

func (b *InMemoryBackend) Close() error { return nil }

// CountEntries returns the number of entries in a table (for tests).
func (b *InMemoryBackend) CountEntries(table Table) int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.tables[table])
}

// --- ReadView ---

type inMemoryReadView struct {
	backend *InMemoryBackend
}

func (v *inMemoryReadView) Get(table Table, key []byte) ([]byte, error) {
	v.backend.mu.RLock()
	defer v.backend.mu.RUnlock()
	t, ok := v.backend.tables[table]
	if !ok {
		return nil, nil
	}
	val, ok := t[string(key)]
	if !ok {
		return nil, nil
	}
	cp := make([]byte, len(val))
	copy(cp, val)
	return cp, nil
}

func (v *inMemoryReadView) PrefixIterator(table Table, prefix []byte) (Iterator, error) {
	v.backend.mu.RLock()
	defer v.backend.mu.RUnlock()
	t, ok := v.backend.tables[table]
	if !ok {
		return &sliceIterator{}, nil
	}

	var entries []KV
	for k, val := range t {
		kb := []byte(k)
		if bytes.HasPrefix(kb, prefix) {
			kcopy := make([]byte, len(kb))
			copy(kcopy, kb)
			vcopy := make([]byte, len(val))
			copy(vcopy, val)
			entries = append(entries, KV{Key: kcopy, Value: vcopy})
		}
	}
	// Sort by key for deterministic iteration.
	sort.Slice(entries, func(i, j int) bool {
		return bytes.Compare(entries[i].Key, entries[j].Key) < 0
	})
	return &sliceIterator{entries: entries, pos: -1}, nil
}

// --- WriteBatch ---

type inMemoryWriteBatch struct {
	backend *InMemoryBackend
	ops     []batchOp
}

type batchOp struct {
	table Table
	key   string
	value []byte // nil = delete
}

func (b *inMemoryWriteBatch) PutBatch(table Table, entries []KV) error {
	for _, e := range entries {
		val := make([]byte, len(e.Value))
		copy(val, e.Value)
		b.ops = append(b.ops, batchOp{table: table, key: string(e.Key), value: val})
	}
	return nil
}

func (b *inMemoryWriteBatch) DeleteBatch(table Table, keys [][]byte) error {
	for _, k := range keys {
		b.ops = append(b.ops, batchOp{table: table, key: string(k), value: nil})
	}
	return nil
}

func (b *inMemoryWriteBatch) Commit() error {
	b.backend.mu.Lock()
	defer b.backend.mu.Unlock()
	for _, op := range b.ops {
		t, ok := b.backend.tables[op.table]
		if !ok {
			t = make(map[string][]byte)
			b.backend.tables[op.table] = t
		}
		if op.value == nil {
			delete(t, op.key)
		} else {
			t[op.key] = op.value
		}
	}
	b.ops = nil
	return nil
}

// --- sliceIterator ---

type sliceIterator struct {
	entries []KV
	pos     int
}

func (it *sliceIterator) Next() bool {
	it.pos++
	return it.pos < len(it.entries)
}

func (it *sliceIterator) Key() []byte {
	if it.pos < 0 || it.pos >= len(it.entries) {
		return nil
	}
	return it.entries[it.pos].Key
}

func (it *sliceIterator) Value() []byte {
	if it.pos < 0 || it.pos >= len(it.entries) {
		return nil
	}
	return it.entries[it.pos].Value
}

func (it *sliceIterator) Close() {}
