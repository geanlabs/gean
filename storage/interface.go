package storage

import "github.com/devylongs/gean/types"

// Store is a storage interface for blocks and states.
type Store interface {
	GetBlock(root [32]byte) (*types.Block, bool)
	PutBlock(root [32]byte, block *types.Block)
	GetState(root [32]byte) (*types.State, bool)
	PutState(root [32]byte, state *types.State)
	GetAllBlocks() map[[32]byte]*types.Block
	GetAllStates() map[[32]byte]*types.State
}
