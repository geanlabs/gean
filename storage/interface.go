package storage

import "github.com/geanlabs/gean/types"

// Store is a storage interface for blocks and states.
type Store interface {
	GetBlock(root [32]byte) (*types.Block, bool)
	PutBlock(root [32]byte, block *types.Block)
	GetSignedBlock(root [32]byte) (*types.SignedBlockWithAttestation, bool)
	PutSignedBlock(root [32]byte, sb *types.SignedBlockWithAttestation)
	GetState(root [32]byte) (*types.State, bool)
	PutState(root [32]byte, state *types.State)
	GetAllBlocks() map[[32]byte]*types.Block
	GetAllStates() map[[32]byte]*types.State
}
