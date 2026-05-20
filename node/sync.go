package node

import (
	"context"
	"sync"
	"time"

	libp2ppeer "github.com/libp2p/go-libp2p/core/peer"

	"github.com/geanlabs/gean/logger"
	"github.com/geanlabs/gean/p2p"
	"github.com/geanlabs/gean/types"
)

// SyncDriverP2P is the subset of *p2p.Host the SyncDriver needs. Pulled out
// as an interface so unit tests can pass a mock without standing up libp2p.
type SyncDriverP2P interface {
	Peers() []libp2ppeer.ID
	SendStatusRequest(ctx context.Context, peerID libp2ppeer.ID, ourStatus *p2p.StatusMessage) (*p2p.StatusMessage, error)
	FetchBlocksByRange(ctx context.Context, peerID libp2ppeer.ID, startSlot, count uint64) ([]*types.SignedBlock, error)
	FetchBlocksByRootBatchWithRetry(ctx context.Context, roots [][32]byte) ([]*types.SignedBlock, [][32]byte, error)
}

// SyncDriver runs a periodic status-poll + range-fetch loop for backfill.
//
// Polling is conditional on Engine.GetSyncStatus():
//   - SyncSyncing  → poll all connected peers; if any peer is far enough
//     ahead (gap > BlocksByRangeSyncThreshold), trigger BlocksByRange.
//   - SyncSynced   → no polling traffic in steady state.
//   - SyncIdle     → no peers, nothing to do.
//
// Mirrors zeam's pattern at pkgs/node/src/node.zig:1629-1634 + 1008.
type SyncDriver struct {
	engine *Engine
	p2p    SyncDriverP2P

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

// NewSyncDriver constructs a driver bound to the given engine and p2p host.
// ctx is captured for use by OnPeerConnected callbacks (see field doc); Run
// reads cancellation from the same ctx.
func NewSyncDriver(ctx context.Context, engine *Engine, p2pHost SyncDriverP2P) *SyncDriver {
	return &SyncDriver{
		ctx:      ctx,
		engine:   engine,
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

	lastStatus := sd.engine.GetSyncStatus()

	for {
		select {
		case <-sd.ctx.Done():
			return
		case <-ticker.C:
			current := sd.engine.GetSyncStatus()
			// Synced → Syncing transitions (e.g. pause-unpause recovery,
			// or a peer suddenly far ahead) get an immediate poll instead
			// of waiting for the next tick. The regular SyncSyncing branch
			// below already polls on every tick, but the transition path
			// shaves up to SyncPollInterval off the recovery latency.
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

// checkAndBackfill triggers a BlocksByRange fetch from peerID if their head is
// ahead of ours by more than BlocksByRangeSyncThreshold slots. On range-fetch
// failure, falls back to a head-by-root fetch (via any peer) so the reactive
// missing-parent flow has something to chase.
func (sd *SyncDriver) checkAndBackfill(ctx context.Context, peerID libp2ppeer.ID, peerStatus *p2p.StatusMessage) {
	ourHead := sd.engine.Store.HeadSlot()
	if peerStatus.HeadSlot <= ourHead {
		return
	}
	gap := peerStatus.HeadSlot - ourHead
	if gap <= p2p.BlocksByRangeSyncThreshold {
		// Below threshold: gossip + reactive missing-parent BlocksByRoot
		// handles small gaps efficiently. Range-fetch would be wasteful.
		return
	}

	if !sd.tryReserve(peerID) {
		return
	}
	defer sd.release(peerID)

	count := gap
	if count > types.MaxRequestBlocks {
		count = types.MaxRequestBlocks
	}
	startSlot := ourHead + 1

	logger.Info(logger.Sync, "sync: peer %s ahead by %d slots, fetching range start_slot=%d count=%d",
		peerID, gap, startSlot, count)

	blocks, err := sd.p2p.FetchBlocksByRange(ctx, peerID, startSlot, count)
	if err != nil {
		logger.Warn(logger.Sync, "sync: blocks_by_range failed peer=%s err=%v; falling back to head-by-root",
			peerID, err)
		// Fallback: at least fetch the peer's head block by root via any
		// peer. The reactive missing-parent flow can chase from there.
		rootBlocks, _, ferr := sd.p2p.FetchBlocksByRootBatchWithRetry(ctx, [][32]byte{peerStatus.HeadRoot})
		if ferr != nil {
			logger.Warn(logger.Sync, "sync: head-by-root fallback also failed: %v", ferr)
			return
		}
		for _, b := range rootBlocks {
			sd.engine.OnBlock(b)
		}
		return
	}

	logger.Info(logger.Sync, "sync: received %d blocks from peer %s, feeding to engine", len(blocks), peerID)
	for _, block := range blocks {
		sd.engine.OnBlock(block)
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
	finalized := sd.engine.Store.LatestFinalized()
	return &p2p.StatusMessage{
		FinalizedRoot: finalized.Root,
		FinalizedSlot: finalized.Slot,
		HeadRoot:      sd.engine.Store.Head(),
		HeadSlot:      sd.engine.Store.HeadSlot(),
	}
}
