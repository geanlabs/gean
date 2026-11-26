package host

import (
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
)

func (h *Host) PeerID() peer.ID {
	return h.host.ID()
}

func (h *Host) Addrs() []multiaddr.Multiaddr {
	return h.host.Addrs()
}

func (h *Host) ConnectedPeers() int {
	return h.peerStore.Count()
}

func (h *Host) Peers() []peer.ID {
	return h.peerStore.AllPeers()
}

// Close shuts the host down and waits for its gossip listeners, req/resp
// handlers and other goroutines to return.
func (h *Host) Close() {
	h.cancel()
	for _, sub := range h.subs {
		sub.Cancel()
	}
	h.enrMu.Lock()
	if h.discovery != nil {
		h.discovery.Close()
	}
	h.enrMu.Unlock()
	h.host.Close()
	h.tasks.Close()
}
