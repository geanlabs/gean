// notifier.go contains libp2p connection event hooks for status exchange and cleanup.
package sync

import (
	"context"
	"log/slog"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/multiformats/go-multiaddr"
)

// connectionNotifier listens for peer connection events.
type connectionNotifier struct {
	syncer *Syncer
	logger *slog.Logger
}

// Listen implements network.Notifiee.
func (n *connectionNotifier) Listen(network.Network, multiaddr.Multiaddr) {}

// ListenClose implements network.Notifiee.
func (n *connectionNotifier) ListenClose(network.Network, multiaddr.Multiaddr) {}

// Connected is called when a new peer connection is established.
// The dialer sends Status first; the listener responds with its own.
func (n *connectionNotifier) Connected(net network.Network, conn network.Conn) {
	peerID := conn.RemotePeer()

	// Check if we initiated the connection (we are the dialer)
	if conn.Stat().Direction == network.DirOutbound {
		// We dialed, we send status first
		n.logger.Debug("new outbound connection, initiating status exchange", "peer", peerID)
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), reqrespTimeout)
			defer cancel()
			if err := n.syncer.InitiateStatusExchange(ctx, peerID); err != nil {
				n.logger.Warn("status exchange failed", "peer", peerID, "error", err)
			}
		}()
	} else {
		n.logger.Debug("new inbound connection", "peer", peerID)
		// If we are the listener, we wait for them to send status first
		// (handled by the stream handler when they open a Status stream)
	}
}

// Disconnected is called when a peer disconnects.
func (n *connectionNotifier) Disconnected(net network.Network, conn network.Conn) {
	peerID := conn.RemotePeer()
	n.logger.Debug("peer disconnected", "peer", peerID)
	n.syncer.RemovePeer(peerID)
}

var _ network.Notifiee = (*connectionNotifier)(nil)
