package sync

import (
	"context"
	"log/slog"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/multiformats/go-multiaddr"
)

// ConnectionNotifier listens for peer connection events.
type ConnectionNotifier struct {
	syncer *Syncer
	logger *slog.Logger
}

// NewConnectionNotifier creates a new connection notifier.
func NewConnectionNotifier(syncer *Syncer, logger *slog.Logger) *ConnectionNotifier {
	return &ConnectionNotifier{
		syncer: syncer,
		logger: logger,
	}
}

// Listen implements network.Notifiee
func (n *ConnectionNotifier) Listen(network.Network, multiaddr.Multiaddr) {}

// ListenClose implements network.Notifiee
func (n *ConnectionNotifier) ListenClose(network.Network, multiaddr.Multiaddr) {}

// Connected is called when a new peer connection is established.
// Per spec: dialing client sends Status first.
func (n *ConnectionNotifier) Connected(net network.Network, conn network.Conn) {
	peerID := conn.RemotePeer()

	// Check if we initiated the connection (we are the dialer)
	isDialer := conn.Stat().Direction == network.DirOutbound

	if isDialer {
		// We dialed, we send status first
		n.logger.Debug("new outbound connection, initiating status exchange",
			"peer", peerID,
		)
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), reqrespTimeout)
			defer cancel()

			if err := n.syncer.InitiateStatusExchange(ctx, peerID); err != nil {
				n.logger.Warn("status exchange failed",
					"peer", peerID,
					"error", err,
				)
			}
		}()
	} else {
		n.logger.Debug("new inbound connection",
			"peer", peerID,
		)
		// If we are the listener, we wait for them to send status first
		// (handled by the stream handler when they open a Status stream)
	}
}

// Disconnected is called when a peer disconnects.
func (n *ConnectionNotifier) Disconnected(net network.Network, conn network.Conn) {
	peerID := conn.RemotePeer()
	n.logger.Debug("peer disconnected", "peer", peerID)
	n.syncer.RemovePeer(peerID)
}

// reqrespTimeout is the timeout for request/response operations.
const reqrespTimeout = 30 * time.Second

// Ensure ConnectionNotifier implements network.Notifiee
var _ network.Notifiee = (*ConnectionNotifier)(nil)
