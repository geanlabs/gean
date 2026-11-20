// Package peers tracks connected peers and scores them by how their block
// requests went, to pick whom to ask next.
package peers

import (
	"math/rand"
	"sync"

	"github.com/libp2p/go-libp2p/core/peer"
)

// Request reputation, as in leanSpec's sync peer manager: a peer starts mid-range,
// gains for every completed block request and loses twice as much for every
// failed one. The score only weights which peer is asked; it never disconnects.
const (
	InitialScore    = 100
	minScore        = 0
	maxScore        = 200
	ScoreSuccess    = 10
	ScoreFailure    = 20
	minSelectWeight = 1 // a peer at the floor can still be picked
)

type peerEntry struct {
	score int
	// headSlot is the head the peer last reported in a status exchange; it is
	// unverified and only routes range requests.
	headSlot  uint64
	hasStatus bool
}

type Store struct {
	mu    sync.RWMutex
	peers map[peer.ID]*peerEntry
}

func NewStore() *Store {
	return &Store{peers: make(map[peer.ID]*peerEntry)}
}

// AddNew tracks a peer at the initial score and reports whether it was new.
func (ps *Store) AddNew(id peer.ID) bool {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	if _, ok := ps.peers[id]; ok {
		return false
	}
	ps.peers[id] = &peerEntry{score: InitialScore}
	OnScore(id.String(), InitialScore)
	return true
}

func (ps *Store) Remove(id peer.ID) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	delete(ps.peers, id)
	OnRemove(id.String())
}

func (ps *Store) Count() int {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return len(ps.peers)
}

// RecordRequest adjusts a tracked peer's score for one block request. The score
// gauge changes under the same lock as the peer set, so a peer removed
// concurrently cannot leave its series behind.
func (ps *Store) RecordRequest(id peer.ID, success bool) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	e, ok := ps.peers[id]
	if !ok {
		return
	}
	if success {
		e.score = min(e.score+ScoreSuccess, maxScore)
	} else {
		e.score = max(e.score-ScoreFailure, minScore)
	}
	OnScore(id.String(), e.score)
}

// UpdateStatus records the head a tracked peer reported.
func (ps *Store) UpdateStatus(id peer.ID, headSlot uint64) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	if e, ok := ps.peers[id]; ok {
		e.headSlot, e.hasStatus = headSlot, true
	}
}

// SelectPeer picks a peer outside exclude at random, weighted by score. With
// minSlot above zero it prefers peers whose reported head reaches minSlot and
// falls back to any peer when none does, since a status can be stale.
func (ps *Store) SelectPeer(exclude map[peer.ID]bool, minSlot uint64) peer.ID {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	var all, covering []peer.ID
	var allWeights, coveringWeights []int
	for id, e := range ps.peers {
		if exclude[id] {
			continue
		}
		w := max(e.score, minSelectWeight)
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

func (ps *Store) AllPeers() []peer.ID {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	ids := make([]peer.ID, 0, len(ps.peers))
	for id := range ps.peers {
		ids = append(ids, id)
	}
	return ids
}

// Score returns a tracked peer's score.
func (ps *Store) Score(id peer.ID) (int, bool) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	e, ok := ps.peers[id]
	if !ok {
		return 0, false
	}
	return e.score, true
}

// HeadSlot returns the head slot a tracked peer last reported, and whether it
// has reported one.
func (ps *Store) HeadSlot(id peer.ID) (uint64, bool) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	e, ok := ps.peers[id]
	if !ok || !e.hasStatus {
		return 0, false
	}
	return e.headSlot, true
}
