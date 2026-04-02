package storage

import "github.com/geanlabs/gean/types"

// Store is a storage interface for blocks and states.
type Store interface {
	GetBlock(root [32]byte) (*types.Block, bool)
	PutBlock(root [32]byte, block *types.Block)
	DeleteBlock(root [32]byte)
	GetSignedBlock(root [32]byte) (*types.SignedBlockWithAttestation, bool)
	PutSignedBlock(root [32]byte, sb *types.SignedBlockWithAttestation)
	DeleteSignedBlock(root [32]byte)
	GetState(root [32]byte) (*types.State, bool)
	PutState(root [32]byte, state *types.State)
	DeleteState(root [32]byte)
	GetAllBlocks() map[[32]byte]*types.Block
	GetAllStates() map[[32]byte]*types.State
	// ForEachBlock iterates over all blocks without copying the full map.
	// Return false from fn to stop iteration early.
	ForEachBlock(fn func(root [32]byte, block *types.Block) bool)
}
