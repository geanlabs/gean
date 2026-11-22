package host

import (
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/geanlabs/gean/internal/core/logger"
)

func (h *Host) installPeerNotifier() {
	h.host.Network().Notify(&network.NotifyBundle{
		ConnectedF: func(n network.Network, conn network.Conn) {
			peerID := conn.RemotePeer()
			isNew := h.peerStore.AddNew(peerID)
			direction := directionLabel(conn.Stat().Direction)
			count := h.peerStore.Count()

			logger.Info(logger.Network, "peer connected peer_id=%s direction=%s peers=%d",
				peerID, conn.Stat().Direction, count)
			if h.hooks.PeerConnected != nil {
				h.hooks.PeerConnected(direction)
			}
			if h.hooks.PeerCount != nil {
				h.hooks.PeerCount(count)
			}
			if isNew && conn.Stat().Direction == network.DirOutbound {
				if status := h.peerStatus.Load(); status != nil {
					h.tasks.Go(func() { (*status)(peerID) })
				}
			}
		},
		DisconnectedF: func(n network.Network, conn network.Conn) {
			peerID := conn.RemotePeer()
			if n.Connectedness(peerID) == network.Connected {
				return
			}

			h.peerStore.Remove(peerID)
			direction := directionLabel(conn.Stat().Direction)
			count := h.peerStore.Count()

			logger.Info(logger.Network, "peer disconnected peer_id=%s peers=%d", peerID, count)
			if h.hooks.PeerDisconnected != nil {
				h.hooks.PeerDisconnected(direction, "remote_close")
			}
			if h.hooks.PeerCount != nil {
				h.hooks.PeerCount(count)
			}
		},
	})
}

func directionLabel(d network.Direction) string {
	if d == network.DirOutbound {
		return "outbound"
	}
	return "inbound"
}

// SetPeerStatus sets what runs, as one of the host's tasks, for each new
// outbound peer. Peers that connected before it is set are not reported to it.
func (h *Host) SetPeerStatus(status func(peer.ID)) {
	h.peerStatus.Store(&status)
}
