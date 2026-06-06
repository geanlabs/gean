package syncer

import (
	"context"

	libp2ppeer "github.com/libp2p/go-libp2p/core/peer"

	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/p2p"
)

func (sd *SyncDriver) OnPeerConnected(peerID libp2ppeer.ID) {
	if !sd.ready() {
		return
	}
	ctx, cancel := context.WithTimeout(sd.ctx, peerStatusTimeout)
	defer cancel()
	sd.pollPeer(ctx, peerID, sd.makeStatusMessage())
}

func (sd *SyncDriver) refreshSyncFromPeers(ctx context.Context) {
	if !sd.ready() {
		return
	}
	ctx = sd.contextOrDefault(ctx)

	peers := sd.p2p.Peers()
	if len(peers) == 0 {
		return
	}

	ourStatus := sd.makeStatusMessage()
	for _, peerID := range peers {
		go sd.pollPeer(ctx, peerID, ourStatus)
	}
}

func (sd *SyncDriver) pollPeer(ctx context.Context, peerID libp2ppeer.ID, ourStatus *p2p.StatusMessage) {
	if !sd.ready() || ourStatus == nil {
		return
	}
	ctx = sd.contextOrDefault(ctx)

	peerStatus, err := sd.p2p.SendStatusRequest(ctx, peerID, ourStatus)
	if err != nil {
		logger.Warn(logger.Sync, "sync: status request to peer %s failed: %v", peerID, err)
		return
	}
	if peerStatus == nil {
		logger.Warn(logger.Sync, "sync: status request to peer %s returned nil status", peerID)
		return
	}

	logger.Info(logger.Sync, "sync: status request to peer %s ok: head_slot=%d finalized_slot=%d",
		peerID, peerStatus.HeadSlot, peerStatus.FinalizedSlot)
	sd.checkAndBackfill(ctx, peerID, peerStatus)
}
