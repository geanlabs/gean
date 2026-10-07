package syncer

import (
	"context"

	"github.com/geanlabs/gean/types"
)

// PeerID identifies a peer to the sync driver. It is opaque here: the network
// that implements SyncDriverP2P decides what it holds.
type PeerID string

// SyncDriverP2P is what the sync driver needs from the network.
type SyncDriverP2P interface {
	Peers() []PeerID
	SendStatusRequest(ctx context.Context, peerID PeerID, ourStatus *types.Status) (*types.Status, error)
	FetchBlocksByRange(ctx context.Context, peerID PeerID, startSlot, count uint64) ([]*types.SignedBlock, error)
	FetchBlocksByRootBatchWithRetry(ctx context.Context, roots [][32]byte) ([]*types.SignedBlock, [][32]byte, error)
}

type LocalNode interface {
	GetSyncStatus() types.SyncStatus
	OnBlock(*types.SignedBlock)
	// OnSyncBlock delivers a requested block with backpressure; it blocks until
	// the node accepts it and returns false only if ctx ends first.
	OnSyncBlock(context.Context, *types.SignedBlock) bool
}
