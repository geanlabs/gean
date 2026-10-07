package p2p

import (
	"math/rand"
	"sync"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

type PeerStore struct {
	mu    sync.RWMutex
	peers map[peer.ID]bool
}

func NewPeerStore() *PeerStore {
	return &PeerStore{peers: make(map[peer.ID]bool)}
}

func (ps *PeerStore) Add(id peer.ID) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.peers[id] = true
}

func (ps *PeerStore) AddNew(id peer.ID) bool {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	if ps.peers[id] {
		return false
	}
	ps.peers[id] = true
	return true
}

func (ps *PeerStore) Remove(id peer.ID) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	delete(ps.peers, id)
}

func (ps *PeerStore) Count() int {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return len(ps.peers)
}

func (ps *PeerStore) RandomPeer(exclude map[peer.ID]bool) peer.ID {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	var candidates []peer.ID
	for id := range ps.peers {
		if !exclude[id] {
			candidates = append(candidates, id)
		}
	}
	if len(candidates) == 0 {
		return ""
	}
	return candidates[rand.Intn(len(candidates))]
}

func (ps *PeerStore) AllPeers() []peer.ID {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	ids := make([]peer.ID, 0, len(ps.peers))
	for id := range ps.peers {
		ids = append(ids, id)
	}
	return ids
}

func directionLabel(d network.Direction) string {
	if d == network.DirOutbound {
		return "outbound"
	}
	return "inbound"
}
