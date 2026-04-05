package xmss

import (
	"sync"

	"github.com/geanlabs/gean/types"
)

// PubKeyCache caches parsed C PublicKey handles to avoid repeated FFI calls.
// Keyed by the raw 52-byte pubkey bytes. Thread-safe via mutex.
//
// The cache owns all handles and frees them on Close().
// Callers must NOT call FreePublicKey on cached handles.
type PubKeyCache struct {
	mu    sync.Mutex
	cache map[[types.PubkeySize]byte]CPubKey
}

// NewPubKeyCache creates an empty pubkey cache.
func NewPubKeyCache() *PubKeyCache {
	return &PubKeyCache{
		cache: make(map[[types.PubkeySize]byte]CPubKey),
	}
}

// Get returns a cached pubkey handle, parsing and caching it on first access.
// The returned handle is owned by the cache — do NOT free it.
func (c *PubKeyCache) Get(pubkeyBytes [types.PubkeySize]byte) (CPubKey, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if pk, ok := c.cache[pubkeyBytes]; ok {
		return pk, nil
	}

	pk, err := ParsePublicKey(pubkeyBytes)
	if err != nil {
		return nil, err
	}

	c.cache[pubkeyBytes] = pk
	return pk, nil
}

// Len returns the number of cached pubkeys.
func (c *PubKeyCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.cache)
}

// Close frees all cached pubkey handles.
func (c *PubKeyCache) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for key, pk := range c.cache {
		FreePublicKey(pk)
		delete(c.cache, key)
	}
}
