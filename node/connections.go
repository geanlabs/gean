package node

import (
	"context"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/multiformats/go-multiaddr"
)

// peerNotifee implements network.Notifiee so we can react to peer connections.
type peerNotifee struct {
	n   *Node
	ctx context.Context
}

func (p *peerNotifee) Connected(_ network.Network, conn network.Conn) {
	pid := conn.RemotePeer()
	// Run sync in a goroutine so we don't block the notifier.
	go p.n.onPeerConnected(p.ctx, pid)
}

func (p *peerNotifee) Disconnected(_ network.Network, conn network.Conn) {}
func (p *peerNotifee) Listen(_ network.Network, _ multiaddr.Multiaddr)   {}
func (p *peerNotifee) ListenClose(_ network.Network, _ multiaddr.Multiaddr) {}

// registerPeerNotifications wires up connection lifecycle callbacks.
// Call this after the host is created, before Run().
func (n *Node) registerPeerNotifications(ctx context.Context) {
	n.Host.P2P.Network().Notify(&peerNotifee{n: n, ctx: ctx})
}