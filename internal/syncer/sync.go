package syncer

import (
	"context"
	"sync"
	"time"

	libp2ppeer "github.com/libp2p/go-libp2p/core/peer"

	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/p2p"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

// SyncDriverP2P is the subset of *p2p.Host the SyncDriver needs. Pulled out
// as an interface so unit tests can pass a mock without standing up libp2p.
type SyncDriverP2P interface {
	Peers() []libp2ppeer.ID
	SendStatusRequest(ctx context.Context, peerID libp2ppeer.ID, ourStatus *p2p.StatusMessage) (*p2p.StatusMessage, error)
	FetchBlocksByRange(ctx context.Context, peerID libp2ppeer.ID, startSlot, count uint64) ([]*types.SignedBlock, error)
	FetchBlocksByRootBatchWithRetry(ctx context.Context, roots [][32]byte) ([]*types.SignedBlock, [][32]byte, error)
}

// LocalNode is the sync driver's boundary back into the consensus loop.
type LocalNode interface {
	GetSyncStatus() SyncStatus
	OnBlock(*types.SignedBlock)
}

// SyncDriver runs a periodic status-poll + range-fetch loop for backfill.
//
// Polling is conditional on LocalNode.GetSyncStatus():
//   - SyncSyncing  → poll all connected peers; if any peer is far enough
//     ahead (gap > BlocksByRangeSyncThreshold), trigger BlocksByRange.
//   - SyncSynced   → no polling traffic in steady state.
//   - SyncIdle     → no peers, nothing to do.
type SyncDriver struct {
	node  LocalNode
	store *store.ConsensusStore
	p2p   SyncDriverP2P

	// ctx captured at construction so OnPeerConnected (invoked from libp2p's
	// network goroutine via p2p.PeerStatusHook) can derive request contexts
	// without a data race against Run setting it. Construction happens in
	// main before any goroutine can call OnPeerConnected, so the field is
	// safely published by happens-before of subsequent goroutine starts.
	ctx context.Context

	// Per-peer in-flight fetch dedup. Prevents stacking range requests to the
	// same peer across overlapping polling cycles.
	mu       sync.Mutex
	inFlight map[libp2ppeer.ID]bool
}

// NewSyncDriver constructs a driver bound to a local node, store, and p2p host.
// ctx is captured for use by OnPeerConnected callbacks (see field doc); Run
// reads cancellation from the same ctx.
func NewSyncDriver(ctx context.Context, node LocalNode, store *store.ConsensusStore, p2pHost SyncDriverP2P) *SyncDriver {
	return &SyncDriver{
		ctx:      ctx,
		node:     node,
		store:    store,
		p2p:      p2pHost,
		inFlight: make(map[libp2ppeer.ID]bool),
	}
}

// Run is the polling loop. Cancel the ctx passed to NewSyncDriver to stop.
func (sd *SyncDriver) Run() {
	ticker := time.NewTicker(p2p.SyncPollInterval)
	defer ticker.Stop()

	logger.Info(logger.Sync, "sync driver started: poll_interval=%s threshold=%d slots",
		p2p.SyncPollInterval, p2p.BlocksByRangeSyncThreshold)

	lastStatus := sd.node.GetSyncStatus()

	for {
		select {
		case <-sd.ctx.Done():
			return
		case <-ticker.C:
			current := sd.node.GetSyncStatus()
			// Synced → Syncing transitions (pause-unpause recovery, peer surge ahead)
			// get an immediate poll instead of waiting for the next ticker fire.
			if lastStatus == SyncSynced && current == SyncSyncing {
				sd.refreshSyncFromPeers(sd.ctx)
			}
			lastStatus = current
			switch current {
			case SyncSyncing:
				sd.refreshSyncFromPeers(sd.ctx)
			case SyncIdle, SyncSynced:
				// No work: no peers to poll, or already at chain head.
			}
		}
	}
}

// OnPeerConnected is the p2p.PeerStatusHook callback. Fires once per
// newly-connected peer (gated by PeerStore.AddNew upstream) and initiates
// the lean P2P Status reqresp handshake using the same pollPeer code path
// the periodic loop uses. Bounded by a 10s timeout so a slow or
// unresponsive peer cannot hold the goroutine indefinitely; on success the
// resulting peer status drives the usual checkAndBackfill dispatch.
//
// Wired by cmd/gean/main.go: p2p.PeerStatusHook = syncDriver.OnPeerConnected
func (sd *SyncDriver) OnPeerConnected(peerID libp2ppeer.ID) {
	ctx, cancel := context.WithTimeout(sd.ctx, 10*time.Second)
	defer cancel()
	ourStatus := sd.makeStatusMessage()
	sd.pollPeer(ctx, peerID, ourStatus)
}

// refreshSyncFromPeers polls every connected peer for status, in parallel.
func (sd *SyncDriver) refreshSyncFromPeers(ctx context.Context) {
	peers := sd.p2p.Peers()
	if len(peers) == 0 {
		return
	}
	ourStatus := sd.makeStatusMessage()
	for _, peerID := range peers {
		go sd.pollPeer(ctx, peerID, ourStatus)
	}
}

// pollPeer sends a status request to peerID and dispatches backfill if needed.
func (sd *SyncDriver) pollPeer(ctx context.Context, peerID libp2ppeer.ID, ourStatus *p2p.StatusMessage) {
	peerStatus, err := sd.p2p.SendStatusRequest(ctx, peerID, ourStatus)
	if err != nil {
		logger.Warn(logger.Sync, "sync: status request to peer %s failed: %v", peerID, err)
		return
	}
	logger.Info(logger.Sync, "sync: status request to peer %s ok: head_slot=%d finalized_slot=%d",
		peerID, peerStatus.HeadSlot, peerStatus.FinalizedSlot)
	sd.checkAndBackfill(ctx, peerID, peerStatus)
}

// checkAndBackfill triggers BlocksByRange fetches from peerID if their head
// is ahead of ours by more than BlocksByRangeSyncThreshold slots. Loops
// within the per-peer reservation until either the peer's advertised head
// slot has been requested or the peer stops returning blocks. On range-fetch
// failure, falls back to a head-by-root fetch (via any peer) so the reactive
// missing-parent flow has something to chase.
func (sd *SyncDriver) checkAndBackfill(ctx context.Context, peerID libp2ppeer.ID, peerStatus *p2p.StatusMessage) {
	ourHead := sd.store.HeadSlot()
	if peerStatus.HeadSlot <= ourHead {
		return
	}
	if peerStatus.HeadSlot-ourHead <= p2p.BlocksByRangeSyncThreshold {
		// Below threshold: gossip + reactive missing-parent BlocksByRoot
		// handles small gaps efficiently. Range-fetch would be wasteful.
		return
	}

	if !sd.tryReserve(peerID) {
		return
	}
	defer sd.release(peerID)

	startSlot := ourHead + 1
	for startSlot <= peerStatus.HeadSlot {
		if err := ctx.Err(); err != nil {
			return
		}
		count := peerStatus.HeadSlot - startSlot + 1
		if count > types.MaxRequestBlocks {
			count = types.MaxRequestBlocks
		}

		logger.Info(logger.Sync, "sync: fetching range from peer %s start_slot=%d count=%d peer_head=%d",
			peerID, startSlot, count, peerStatus.HeadSlot)

		blocks, err := sd.p2p.FetchBlocksByRange(ctx, peerID, startSlot, count)
		if err != nil {
			logger.Warn(logger.Sync, "sync: blocks_by_range failed peer=%s err=%v; falling back to head-by-root",
				peerID, err)
			rootBlocks, _, ferr := sd.p2p.FetchBlocksByRootBatchWithRetry(ctx, [][32]byte{peerStatus.HeadRoot})
			if ferr != nil {
				logger.Warn(logger.Sync, "sync: head-by-root fallback also failed: %v", ferr)
				return
			}
			for _, b := range rootBlocks {
				sd.node.OnBlock(b)
			}
			return
		}

		if len(blocks) == 0 {
			// Peer ran out of blocks below its advertised head — either it
			// pruned the requested range or empty slots fill it. Either way,
			// looping further against this peer is pointless; the next poll
			// cycle picks up where we left off (or tries another peer).
			logger.Info(logger.Sync, "sync: peer %s returned empty range at start_slot=%d, stopping", peerID, startSlot)
			return
		}

		logger.Info(logger.Sync, "sync: received %d blocks from peer %s, feeding to engine", len(blocks), peerID)
		for _, block := range blocks {
			sd.node.OnBlock(block)
		}

		// Advance past the highest slot the peer actually returned so the
		// next iteration requests strictly forward, regardless of empty-slot
		// gaps in the response.
		lastSlot := blocks[len(blocks)-1].Block.Slot
		if lastSlot < startSlot {
			// Defensive: peer returned blocks below the requested start.
			return
		}
		startSlot = lastSlot + 1
	}
}

// tryReserve marks peerID as having an in-flight backfill. Returns false if
// one is already in flight.
func (sd *SyncDriver) tryReserve(peerID libp2ppeer.ID) bool {
	sd.mu.Lock()
	defer sd.mu.Unlock()
	if sd.inFlight[peerID] {
		return false
	}
	sd.inFlight[peerID] = true
	return true
}

// release clears the in-flight marker for peerID.
func (sd *SyncDriver) release(peerID libp2ppeer.ID) {
	sd.mu.Lock()
	defer sd.mu.Unlock()
	delete(sd.inFlight, peerID)
}

// makeStatusMessage assembles the local node's status to advertise to peers.
func (sd *SyncDriver) makeStatusMessage() *p2p.StatusMessage {
	finalized := sd.store.LatestFinalized()
	return &p2p.StatusMessage{
		FinalizedRoot: finalized.Root,
		FinalizedSlot: finalized.Slot,
		HeadRoot:      sd.store.Head(),
		HeadSlot:      sd.store.HeadSlot(),
	}
}
