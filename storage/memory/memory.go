package memory

import (
	"sync"

	"github.com/geanlabs/gean/storage"
	"github.com/geanlabs/gean/types"
)

// MemoryStore is an in-memory implementation of storage.Store.
type MemoryStore struct {
	mu           sync.RWMutex
	blocks       map[[32]byte]*types.Block
	signedBlocks map[[32]byte]*types.SignedBlockWithAttestation
	states       map[[32]byte]*types.State
}

var _ storage.Store = (*MemoryStore)(nil)

// New creates a new in-memory store.
func New() *MemoryStore {
	return &MemoryStore{
		blocks:       make(map[[32]byte]*types.Block),
		signedBlocks: make(map[[32]byte]*types.SignedBlockWithAttestation),
		states:       make(map[[32]byte]*types.State),
	}
}

func (m *MemoryStore) GetBlock(root [32]byte) (*types.Block, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	b, ok := m.blocks[root]
	return b, ok
}

func (m *MemoryStore) PutBlock(root [32]byte, block *types.Block) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.blocks[root] = block
}

func (m *MemoryStore) GetSignedBlock(root [32]byte) (*types.SignedBlockWithAttestation, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	sb, ok := m.signedBlocks[root]
	return sb, ok
}

func (m *MemoryStore) PutSignedBlock(root [32]byte, sb *types.SignedBlockWithAttestation) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.signedBlocks[root] = sb
}

func (m *MemoryStore) GetState(root [32]byte) (*types.State, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.states[root]
	return s, ok
}

func (m *MemoryStore) PutState(root [32]byte, state *types.State) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.states[root] = state
}

func (m *MemoryStore) GetAllBlocks() map[[32]byte]*types.Block {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cp := make(map[[32]byte]*types.Block, len(m.blocks))
	for k, v := range m.blocks {
		cp[k] = v
	}
	return cp
}

func (m *MemoryStore) GetAllStates() map[[32]byte]*types.State {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cp := make(map[[32]byte]*types.State, len(m.states))
	for k, v := range m.states {
		cp[k] = v
	}
	return cp
}
