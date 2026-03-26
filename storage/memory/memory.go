package memory

import (
	"sync"

	"github.com/geanlabs/gean/types"
)

// Store is an in-memory implementation of storage.Store.
type Store struct {
	mu           sync.RWMutex
	blocks       map[[32]byte]*types.Block
	signedBlocks map[[32]byte]*types.SignedBlockWithAttestation
	states       map[[32]byte]*types.State
	meta         map[string][]byte
}

// New creates a new in-memory store.
func New() *Store {
	return &Store{
		blocks:       make(map[[32]byte]*types.Block),
		signedBlocks: make(map[[32]byte]*types.SignedBlockWithAttestation),
		states:       make(map[[32]byte]*types.State),
		meta:         make(map[string][]byte),
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

func (m *Store) GetSignedBlock(root [32]byte) (*types.SignedBlockWithAttestation, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	sb, ok := m.signedBlocks[root]
	return sb, ok
}

func (m *Store) PutSignedBlock(root [32]byte, sb *types.SignedBlockWithAttestation) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.signedBlocks[root] = sb
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

func (m *Store) DeleteBlocks(roots [][32]byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, root := range roots {
		delete(m.blocks, root)
		delete(m.signedBlocks, root)
	}
}

func (m *Store) DeleteStates(roots [][32]byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, root := range roots {
		delete(m.states, root)
	}
}

func (m *Store) GetMeta(key string) ([]byte, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.meta[key]
	if !ok {
		return nil, false
	}
	cp := make([]byte, len(value))
	copy(cp, value)
	return cp, true
}

func (m *Store) PutMeta(key string, value []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]byte, len(value))
	copy(cp, value)
	m.meta[key] = cp
	return nil
}

func (m *Store) DeleteMeta(key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.meta, key)
	return nil
}
