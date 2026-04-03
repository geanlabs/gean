package p2p

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"

	"github.com/geanlabs/gean/logger"
	"github.com/geanlabs/gean/types"
)

// Retry parameters matching ethlambda p2p/lib.rs L56-59.
const (
	MaxFetchRetries    = 10
	InitialBackoffMs   = 5
	BackoffMultiplier  = 2
	BootnodeRedialSecs = 12
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
// Matches ethlambda p2p retry logic: random peer selection, exclude failed.
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

// ConnectBootnodes connects to a list of bootnode multiaddrs.
// Matches ethlambda p2p/lib.rs bootnode connection logic.
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
// Matches ethlambda p2p/lib.rs redial every 12 seconds.
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
// Matches ethlambda p2p/lib.rs block fetch retry logic (L56-59).
func (h *Host) FetchBlocksByRootWithRetry(ctx context.Context, roots [][32]byte) ([]*SignedBlockWithAttestationResult, error) {
	var results []*SignedBlockWithAttestationResult

	for _, root := range roots {
		block, err := h.fetchSingleBlockWithRetry(ctx, root)
		results = append(results, &SignedBlockWithAttestationResult{
			Root:  root,
			Block: block,
			Err:   err,
		})
	}

	return results, nil
}

// SignedBlockWithAttestationResult holds the result of fetching a single block.
type SignedBlockWithAttestationResult struct {
	Root  [32]byte
	Block []*types.SignedBlockWithAttestation
	Err   error
}

func (h *Host) fetchSingleBlockWithRetry(ctx context.Context, root [32]byte) ([]*types.SignedBlockWithAttestation, error) {
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

