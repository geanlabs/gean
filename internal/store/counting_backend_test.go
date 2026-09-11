package store_test

import (
	"sync/atomic"

	"github.com/geanlabs/gean/internal/storage"
)

// countingBackend counts the Get calls that reach the backend, so a test can
// assert that a cache actually spared the read rather than merely returning the
// right answer.
type countingBackend struct {
	storage.Backend
	gets atomic.Int64
}

func (b *countingBackend) reads() int64 { return b.gets.Load() }

func (b *countingBackend) BeginRead() (storage.ReadView, error) {
	rv, err := b.Backend.BeginRead()
	if err != nil {
		return nil, err
	}
	return &countingReadView{ReadView: rv, owner: b}, nil
}

type countingReadView struct {
	storage.ReadView
	owner *countingBackend
}

func (v *countingReadView) Get(table storage.Table, key []byte) ([]byte, error) {
	v.owner.gets.Add(1)
	return v.ReadView.Get(table, key)
}
