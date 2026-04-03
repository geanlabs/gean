package storage

// Backend is a pluggable storage backend.
// Matches ethlambda: crates/storage/src/api/traits.rs
type Backend interface {
	// BeginRead returns a read-only view of the storage.
	BeginRead() (ReadView, error)

	// BeginWrite returns an atomic write batch.
	BeginWrite() (WriteBatch, error)

	// EstimateTableBytes returns estimated live data size for a table.
	EstimateTableBytes(table Table) uint64

	// Close releases backend resources.
	Close() error
}

// ReadView provides read-only access to storage.
type ReadView interface {
	// Get retrieves a value by key from a table. Returns nil if not found.
	Get(table Table, key []byte) ([]byte, error)

	// PrefixIterator iterates over all entries with a given key prefix.
	PrefixIterator(table Table, prefix []byte) (Iterator, error)
}

// WriteBatch provides atomic batched writes.
type WriteBatch interface {
	// PutBatch inserts multiple key-value pairs into a table.
	PutBatch(table Table, entries []KV) error

	// DeleteBatch removes multiple keys from a table.
	DeleteBatch(table Table, keys [][]byte) error

	// Commit atomically writes all batched operations.
	Commit() error
}

// Iterator yields key-value pairs.
type Iterator interface {
	// Next advances the iterator. Returns false when exhausted.
	Next() bool

	// Key returns the current key.
	Key() []byte

	// Value returns the current value.
	Value() []byte

	// Close releases iterator resources.
	Close()
}

// KV is a key-value pair for batch operations.
type KV struct {
	Key   []byte
	Value []byte
}
