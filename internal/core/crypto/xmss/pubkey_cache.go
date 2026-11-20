package xmss

import (
	"sync"

	"github.com/geanlabs/gean/internal/types"
)

type PubKeyCache struct {
	mu    sync.Mutex
	cache map[[types.PubkeySize]byte]CPubKey
}

func NewPubKeyCache() *PubKeyCache {
	return &PubKeyCache{
		cache: make(map[[types.PubkeySize]byte]CPubKey),
	}
}

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

func (c *PubKeyCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.cache)
}

func (c *PubKeyCache) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for key, pk := range c.cache {
		FreePublicKey(pk)
		delete(c.cache, key)
	}
}
