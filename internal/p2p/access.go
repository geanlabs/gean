package p2p

import (
	libp2phost "github.com/libp2p/go-libp2p/core/host"
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

func (h *Host) TopicMeshSizes() map[string]int {
	sizes := make(map[string]int, len(h.topics))
	for name, topic := range h.topics {
		sizes[name] = len(topic.ListPeers())
	}
	return sizes
}

func (h *Host) MeshPeerCount() int {
	seen := make(map[peer.ID]struct{})
	for _, topic := range h.topics {
		for _, p := range topic.ListPeers() {
			seen[p] = struct{}{}
		}
	}
	return len(seen)
}

func (h *Host) LibP2PHost() libp2phost.Host {
	return h.host
}

func (h *Host) Close() {
	h.cancel()
	for _, sub := range h.subs {
		sub.Cancel()
	}
	h.host.Close()
}
