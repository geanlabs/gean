package peers

import (
	"testing"

	"github.com/libp2p/go-libp2p/core/peer"
)

func TestPeerStoreAddRemove(t *testing.T) {
	ps := NewStore()
	if ps.Count() != 0 {
		t.Fatal("should start empty")
	}

	ps.AddNew("peer1")
	ps.AddNew("peer2")
	if ps.Count() != 2 {
		t.Fatalf("expected 2, got %d", ps.Count())
	}

	ps.Remove("peer1")
	if ps.Count() != 1 {
		t.Fatalf("expected 1, got %d", ps.Count())
	}
}

func peerScore(ps *Store, id peer.ID) int {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return ps.peers[id].score
}

// leanSpec's request reputation: +10 per completed request, -20 per failure,
// clamped to [0, 200] from a start of 100.
func TestPeerStoreRecordRequestScores(t *testing.T) {
	tests := []struct {
		name      string
		successes int
		failures  int
		want      int
	}{
		{"new peer", 0, 0, 100},
		{"one success", 1, 0, 110},
		{"one failure", 0, 1, 80},
		{"capped at 200", 20, 0, 200},
		{"floored at 0", 0, 10, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ps := NewStore()
			ps.AddNew("p")
			for range tt.successes {
				ps.RecordRequest("p", true)
			}
			for range tt.failures {
				ps.RecordRequest("p", false)
			}
			if got := peerScore(ps, "p"); got != tt.want {
				t.Fatalf("score = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestPickWeightedFollowsCumulativeWeights(t *testing.T) {
	ids := []peer.ID{"good", "floor"}
	weights := []int{200, 1}
	for _, tt := range []struct {
		r    int
		want peer.ID
	}{{0, "good"}, {199, "good"}, {200, "floor"}} {
		if got := pickWeighted(ids, weights, tt.r); got != tt.want {
			t.Fatalf("pickWeighted(r=%d) = %s, want %s", tt.r, got, tt.want)
		}
	}
}

func TestPeerStoreSelectPeer(t *testing.T) {
	ps := NewStore()
	for _, id := range []peer.ID{"ahead", "behind", "unknown"} {
		ps.AddNew(id)
	}
	ps.UpdateStatus("ahead", 50)
	ps.UpdateStatus("behind", 10)
	ps.UpdateStatus("untracked", 99)

	tests := []struct {
		name    string
		exclude map[peer.ID]bool
		minSlot uint64
		want    map[peer.ID]bool
	}{
		{"only the peer covering minSlot", nil, 40, map[peer.ID]bool{"ahead": true}},
		{"any peer when none covers it", nil, 60, map[peer.ID]bool{"ahead": true, "behind": true, "unknown": true}},
		{"any other peer when the covering one is excluded", map[peer.ID]bool{"ahead": true}, 40, map[peer.ID]bool{"behind": true, "unknown": true}},
		{"none left", map[peer.ID]bool{"ahead": true, "behind": true, "unknown": true}, 0, map[peer.ID]bool{"": true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for range 50 {
				if got := ps.SelectPeer(tt.exclude, tt.minSlot); !tt.want[got] {
					t.Fatalf("SelectPeer = %q, want one of %v", got, tt.want)
				}
			}
		})
	}
}
