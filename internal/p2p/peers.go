package p2p

import (
	"context"
	"math/rand"
	"sync"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/geanlabs/gean/internal/metrics"
)

// Request reputation, as in leanSpec's sync peer manager: a peer starts mid-range,
// gains for every completed block request and loses twice as much for every
// failed one. The score only weights which peer is asked; it never disconnects.
const (
	initialPeerScore    = 100
	minPeerScore        = 0
	maxPeerScore        = 200
	peerScoreSuccess    = 10
	peerScoreFailure    = 20
	minPeerSelectWeight = 1 // a peer at the floor can still be picked
)

type peerEntry struct {
	score int
	// headSlot is the head the peer last reported in a status exchange; it is
	// unverified and only routes range requests.
	headSlot  uint64
	hasStatus bool
}

type PeerStore struct {
	mu    sync.RWMutex
	peers map[peer.ID]*peerEntry
}

func NewPeerStore() *PeerStore {
	return &PeerStore{peers: make(map[peer.ID]*peerEntry)}
}

// AddNew tracks a peer at the initial score and reports whether it was new.
func (ps *PeerStore) AddNew(id peer.ID) bool {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	if _, ok := ps.peers[id]; ok {
		return false
	}
	ps.peers[id] = &peerEntry{score: initialPeerScore}
	metrics.SetPeerScore(id.String(), initialPeerScore)
	return true
}

func (ps *PeerStore) Remove(id peer.ID) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	delete(ps.peers, id)
	metrics.DeletePeerScore(id.String())
}

func (ps *PeerStore) Count() int {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return len(ps.peers)
}

// RecordRequest adjusts a tracked peer's score for one block request. The score
// gauge changes under the same lock as the peer set, so a peer removed
// concurrently cannot leave its series behind.
func (ps *PeerStore) RecordRequest(id peer.ID, success bool) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	e, ok := ps.peers[id]
	if !ok {
		return
	}
	if success {
		e.score = min(e.score+peerScoreSuccess, maxPeerScore)
	} else {
		e.score = max(e.score-peerScoreFailure, minPeerScore)
	}
	metrics.SetPeerScore(id.String(), e.score)
}

// UpdateStatus records the head a tracked peer reported.
func (ps *PeerStore) UpdateStatus(id peer.ID, headSlot uint64) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	if e, ok := ps.peers[id]; ok {
		e.headSlot, e.hasStatus = headSlot, true
	}
}

// SelectPeer picks a peer outside exclude at random, weighted by score. With
// minSlot above zero it prefers peers whose reported head reaches minSlot and
// falls back to any peer when none does, since a status can be stale.
func (ps *PeerStore) SelectPeer(exclude map[peer.ID]bool, minSlot uint64) peer.ID {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	var all, covering []peer.ID
	var allWeights, coveringWeights []int
	for id, e := range ps.peers {
		if exclude[id] {
			continue
		}
		w := max(e.score, minPeerSelectWeight)
		all, allWeights = append(all, id), append(allWeights, w)
		if minSlot > 0 && e.hasStatus && e.headSlot >= minSlot {
			covering, coveringWeights = append(covering, id), append(coveringWeights, w)
		}
	}
	if len(covering) > 0 {
		all, allWeights = covering, coveringWeights
	}
	if len(all) == 0 {
		return ""
	}
	total := 0
	for _, w := range allWeights {
		total += w
	}
	return pickWeighted(all, allWeights, rand.Intn(total))
}

// pickWeighted returns the peer whose cumulative weight range holds r, for r in
// [0, sum(weights)).
func pickWeighted(ids []peer.ID, weights []int, r int) peer.ID {
	for i, w := range weights {
		if r < w {
			return ids[i]
		}
		r -= w
	}
	return ids[len(ids)-1]
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

// recordRequest scores one outbound block request against its peer: an error is
// a failure, any response a success. A request cut short by our own
// cancellation says nothing about the peer.
func (h *Host) recordRequest(parent context.Context, id peer.ID, err error) {
	if err != nil && parent.Err() != nil {
		return
	}
	outcome := "success"
	if err != nil {
		outcome = "failure"
	}
	metrics.IncPeerRequestOutcome(outcome)
	h.peerStore.RecordRequest(id, err == nil)
}

func directionLabel(d network.Direction) string {
	if d == network.DirOutbound {
		return "outbound"
	}
	return "inbound"
}
