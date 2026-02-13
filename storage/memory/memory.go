package memory

import (
	"sync"

	"github.com/geanlabs/gean/types"
)

// Store is an in-memory implementation of storage.Store.
type Store struct {
	mu     sync.RWMutex
	blocks map[[32]byte]*types.Block
	states map[[32]byte]*types.State
}

// New creates a new in-memory store.
func New() *Store {
	return &Store{
		blocks: make(map[[32]byte]*types.Block),
		states: make(map[[32]byte]*types.State),
	}
}

func (m *Store) GetBlock(root [32]byte) (*types.Block, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	b, ok := m.blocks[root]
	return b, ok
}

func (m *Store) PutBlock(root [32]byte, block *types.Block) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.blocks[root] = block
}

func (m *Store) GetState(root [32]byte) (*types.State, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.states[root]
	return s, ok
}

func (m *Store) PutState(root [32]byte, state *types.State) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.states[root] = state
}

func (m *Store) GetAllBlocks() map[[32]byte]*types.Block {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cp := make(map[[32]byte]*types.Block, len(m.blocks))
	for k, v := range m.blocks {
		cp[k] = v
	}
	return cp
}

func (m *Store) GetAllStates() map[[32]byte]*types.State {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cp := make(map[[32]byte]*types.State, len(m.states))
	for k, v := range m.states {
		cp[k] = v
	}
	return cp
}
