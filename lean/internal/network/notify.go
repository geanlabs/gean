package network

import (
	libp2pnetwork "github.com/libp2p/go-libp2p/core/network"

	"github.com/geanlabs/gean/internal/logger"
)

func (h *Host) installPeerNotifier() {
	h.host.Network().Notify(&libp2pnetwork.NotifyBundle{
		ConnectedF: func(n libp2pnetwork.Network, conn libp2pnetwork.Conn) {
			peerID := conn.RemotePeer()
			isNew := h.peerStore.AddNew(peerID)
			direction := directionLabel(conn.Stat().Direction)
			count := h.peerStore.Count()

			logger.Info(logger.Network, "peer connected peer_id=%s direction=%s peers=%d",
				peerID, conn.Stat().Direction, count)
			if h.Hooks.PeerConnected != nil {
				h.Hooks.PeerConnected(direction)
			}
			if h.Hooks.PeerCount != nil {
				h.Hooks.PeerCount(count)
			}
			if isNew && h.Hooks.PeerStatus != nil && conn.Stat().Direction == libp2pnetwork.DirOutbound {
				h.tasks.Go(func() { h.Hooks.PeerStatus(peerID) })
			}
		},
		DisconnectedF: func(n libp2pnetwork.Network, conn libp2pnetwork.Conn) {
			peerID := conn.RemotePeer()
			if n.Connectedness(peerID) == libp2pnetwork.Connected {
				return
			}

			h.peerStore.Remove(peerID)
			direction := directionLabel(conn.Stat().Direction)
			count := h.peerStore.Count()

			logger.Info(logger.Network, "peer disconnected peer_id=%s peers=%d", peerID, count)
			if h.Hooks.PeerDisconnected != nil {
				h.Hooks.PeerDisconnected(direction, "remote_close")
			}
			if h.Hooks.PeerCount != nil {
				h.Hooks.PeerCount(count)
			}
		},
	})
}
