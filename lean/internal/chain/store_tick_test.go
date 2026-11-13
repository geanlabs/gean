package chain

import (
	"errors"
	"testing"
	"time"

	"github.com/geanlabs/gean/internal/storage"
	"github.com/geanlabs/gean/lean/consensus/types"
	"github.com/geanlabs/gean/lean/internal/pools"
	"github.com/geanlabs/gean/lean/internal/store"
)

type tickWriteFailure struct{ storage.Backend }

func (tickWriteFailure) BeginWrite() (storage.WriteBatch, error) {
	return nil, errors.New("time write failed")
}

func TestOnTickReturnsWhenTimeWriteFails(t *testing.T) {
	backend := storage.NewInMemoryBackend()
	store.NewConsensusStore(backend).SetTime(3)
	s := store.NewConsensusStore(tickWriteFailure{Backend: backend})
	p := pools.New()
	p.NewPayloads.PushData([32]byte{1}, &types.AttestationData{Slot: 1})
	done := make(chan struct{})
	go func() {
		OnTick(s, p, types.MillisecondsPerInterval*4, false)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("OnTick did not return after metadata write failure")
	}
	if s.Time() != 3 || p.NewPayloads.Len() != 1 || p.KnownPayloads.Len() != 0 {
		t.Fatal("failed time write changed the clock or promoted payloads")
	}
}

func TestOnTickPromotesAtTheExistingBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name        string
		from, to    uint64
		hasProposal bool
		promoted    bool
	}{
		{"interval four", 3, 4, false, true},
		{"proposer interval zero", 4, 5, true, true},
		{"non-proposer interval zero", 4, 5, false, false},
		{"intermediate interval zero", 4, 6, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := makeTestStore()
			s.SetTime(tc.from)
			p := pools.New()
			p.NewPayloads.Push([32]byte{1}, &types.AttestationData{Slot: 1}, &types.SingleMessageAggregate{
				Participants: types.BitlistFromIndices([]uint64{0}),
				Proof:        []byte{1},
			})

			timestamp := s.Config().GenesisTime*1000 + tc.to*types.MillisecondsPerInterval
			OnTick(s, p, timestamp, tc.hasProposal)

			if got := s.Time(); got != tc.to {
				t.Fatalf("time=%d, want %d", got, tc.to)
			}
			wantNew, wantKnown := 1, 0
			if tc.promoted {
				wantNew, wantKnown = 0, 1
			}
			if p.NewPayloads.Len() != wantNew || p.KnownPayloads.Len() != wantKnown {
				t.Fatalf("new=%d known=%d, want new=%d known=%d", p.NewPayloads.Len(), p.KnownPayloads.Len(), wantNew, wantKnown)
			}
		})
	}
}
