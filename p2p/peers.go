package p2p

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"

	"github.com/geanlabs/gean/logger"
	"github.com/geanlabs/gean/types"
)

// Retry parameters rs L56-59.
const (
	MaxFetchRetries    = 10
	InitialBackoffMs   = 5
	BackoffMultiplier  = 2
	BootnodeRedialSecs = 12
	// MaxBlocksPerRequest matches leanSpec MAX_BLOCKS_PER_REQUEST.
	MaxBlocksPerRequest = 10
)

// PeerStore tracks connected peers.
type PeerStore struct {
	mu    sync.RWMutex
	peers map[peer.ID]bool
}

// NewPeerStore creates an empty peer store.
func NewPeerStore() *PeerStore {
	return &PeerStore{peers: make(map[peer.ID]bool)}
}

// Add registers a connected peer.
func (ps *PeerStore) Add(id peer.ID) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.peers[id] = true
}

// Remove unregisters a peer.
func (ps *PeerStore) Remove(id peer.ID) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	delete(ps.peers, id)
}

// Count returns number of connected peers.
func (ps *PeerStore) Count() int {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return len(ps.peers)
}

// RandomPeer returns a random connected peer, excluding the given set.
// Returns empty peer.ID if none available.
func (ps *PeerStore) RandomPeer(exclude map[peer.ID]bool) peer.ID {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	var candidates []peer.ID
	for id := range ps.peers {
		if !exclude[id] {
			candidates = append(candidates, id)
		}
	}
	if len(candidates) == 0 {
		return ""
	}
	return candidates[rand.Intn(len(candidates))]
}

// AllPeers returns all connected peer IDs.
func (ps *PeerStore) AllPeers() []peer.ID {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	ids := make([]peer.ID, 0, len(ps.peers))
	for id := range ps.peers {
		ids = append(ids, id)
	}
	return ids
}

// directionLabel maps a libp2p connection direction to the spec's
// "inbound"/"outbound" label values.
func directionLabel(d network.Direction) string {
	if d == network.DirOutbound {
		return "outbound"
	}
	return "inbound"
}

// ConnectBootnodes connects to a list of bootnode multiaddrs.
func (h *Host) ConnectBootnodes(ctx context.Context, addrs []multiaddr.Multiaddr) {
	for _, addr := range addrs {
		peerInfo, err := peer.AddrInfoFromP2pAddr(addr)
		if err != nil {
			logger.Warn(logger.Network, "invalid bootnode addr %s: %v", addr, err)
			continue
		}
		if err := h.host.Connect(ctx, *peerInfo); err != nil {
			logger.Warn(logger.Network, "bootnode connect failed %s: %v", addr, err)
		} else {
			h.peerStore.Add(peerInfo.ID)
			logger.Info(logger.Network, "connected to bootnode %s", peerInfo.ID.ShortString())
		}
	}
}

// StartBootnodeRedial periodically reconnects to bootnodes if disconnected.
func (h *Host) StartBootnodeRedial(ctx context.Context, addrs []multiaddr.Multiaddr) {
	go func() {
		ticker := time.NewTicker(BootnodeRedialSecs * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				for _, addr := range addrs {
					peerInfo, err := peer.AddrInfoFromP2pAddr(addr)
					if err != nil {
						continue
					}
					if h.host.Network().Connectedness(peerInfo.ID) != 1 { // not connected
						if err := h.host.Connect(ctx, *peerInfo); err == nil {
							h.peerStore.Add(peerInfo.ID)
							logger.Info(logger.Network, "reconnected to bootnode %s", peerInfo.ID.ShortString())
						}
					}
				}
			}
		}
	}()
}

// FetchBlocksByRootWithRetry fetches blocks with exponential backoff retry.
// Backoff: 5, 10, 20, 40, 80, 160, 320, 640, 1280, 2560 ms.
// Random peer selection, exclude previously-failed peers per root.
func (h *Host) FetchBlocksByRootWithRetry(ctx context.Context, roots [][32]byte) ([]*SignedBlockResult, error) {
	var results []*SignedBlockResult

	for _, root := range roots {
		block, err := h.fetchSingleBlockWithRetry(ctx, root)
		results = append(results, &SignedBlockResult{
			Root:  root,
			Block: block,
			Err:   err,
		})
	}

	return results, nil
}

// FetchBlocksByRootBatchWithRetry fetches up to MaxBlocksPerRequest roots
// from a single peer in one request. Retries up to MaxFetchRetries times,
// rotating peers and excluding previously-failed ones.
//
// Returns the blocks the peer delivered (may be fewer than requested if the
// peer doesn't have all of them) and the set of roots that were not delivered
// after exhausting retries.
func (h *Host) FetchBlocksByRootBatchWithRetry(ctx context.Context, roots [][32]byte) ([]*types.SignedBlock, [][32]byte, error) {
	if len(roots) == 0 {
		return nil, nil, nil
	}
	if len(roots) > MaxBlocksPerRequest {
		roots = roots[:MaxBlocksPerRequest]
	}

	excluded := make(map[peer.ID]bool)
	backoff := time.Duration(InitialBackoffMs) * time.Millisecond

	for attempt := 0; attempt < MaxFetchRetries; attempt++ {
		peerID := h.peerStore.RandomPeer(excluded)
		if peerID == "" {
			return nil, roots, fmt.Errorf("no peers available for batch block fetch")
		}

		blocks, err := h.FetchBlocksByRoot(ctx, peerID, roots)
		if err == nil && len(blocks) > 0 {
			missing := computeMissingRoots(roots, blocks)
			return blocks, missing, nil
		}

		excluded[peerID] = true
		reason := "peer returned no blocks"
		if err != nil {
			reason = err.Error()
		}
		logger.Warn(logger.Network, "batch block fetch attempt %d/%d failed for %d root(s) peer=%s reason=%s",
			attempt+1, MaxFetchRetries, len(roots), peerID, reason)

		select {
		case <-ctx.Done():
			return nil, roots, ctx.Err()
		case <-time.After(backoff):
			backoff *= BackoffMultiplier
		}
	}

	return nil, roots, fmt.Errorf("batch block fetch failed after %d retries for %d roots", MaxFetchRetries, len(roots))
}

// computeMissingRoots returns the roots that the peer did not deliver.
func computeMissingRoots(requested [][32]byte, delivered []*types.SignedBlock) [][32]byte {
	deliveredRoots := make(map[[32]byte]bool, len(delivered))
	for _, b := range delivered {
		root, err := b.Block.HashTreeRoot()
		if err != nil {
			continue
		}
		deliveredRoots[root] = true
	}
	var missing [][32]byte
	for _, r := range requested {
		if !deliveredRoots[r] {
			missing = append(missing, r)
		}
	}
	return missing
}

// SignedBlockResult holds the result of fetching a single block.
type SignedBlockResult struct {
	Root  [32]byte
	Block []*types.SignedBlock
	Err   error
}

func (h *Host) fetchSingleBlockWithRetry(ctx context.Context, root [32]byte) ([]*types.SignedBlock, error) {
	excluded := make(map[peer.ID]bool)
	backoff := time.Duration(InitialBackoffMs) * time.Millisecond

	for attempt := 0; attempt < MaxFetchRetries; attempt++ {
		peerID := h.peerStore.RandomPeer(excluded)
		if peerID == "" {
			return nil, fmt.Errorf("no peers available for block fetch")
		}

		blocks, err := h.FetchBlocksByRoot(ctx, peerID, [][32]byte{root})
		if err == nil && len(blocks) > 0 {
			return blocks, nil
		}

		excluded[peerID] = true
		reason := "peer returned no blocks"
		if err != nil {
			reason = err.Error()
		}
		logger.Warn(logger.Network, "block fetch attempt %d/%d failed for block_root=0x%x peer=%s reason=%s",
			attempt+1, MaxFetchRetries, root, peerID, reason)

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(backoff):
			backoff *= BackoffMultiplier
		}
	}

	return nil, fmt.Errorf("block fetch failed after %d retries for %x", MaxFetchRetries, root)
}
