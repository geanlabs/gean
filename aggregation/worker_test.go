package aggregation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/geanlabs/gean/crypto/insecure"
	"github.com/geanlabs/gean/shadow"
	"github.com/geanlabs/gean/types"
)

type recordingPublisher struct {
	count int
	err   error
}

func (p *recordingPublisher) PublishAggregatedAttestation(context.Context, *types.SignedAggregatedAttestation) error {
	p.count++
	return p.err
}

func TestRunWorkerReturnsWhenDispatchChannelCloses(t *testing.T) {
	dispatches := make(chan Dispatch)
	close(dispatches)

	done := make(chan struct{})
	go func() {
		NewWorker(nil, nil, nil, nil, shadow.Rates{}, time.Now).Run(context.Background(), dispatches)
		close(done)
	}()

	waitForWorker(t, done)
}

func TestRunWorkerSkipsNilSnapshot(t *testing.T) {
	dispatches := make(chan Dispatch, 1)
	dispatches <- Dispatch{Slot: 1}
	close(dispatches)

	done := make(chan struct{})
	go func() {
		NewWorker(nil, nil, nil, nil, shadow.Rates{}, time.Now).Run(context.Background(), dispatches)
		close(done)
	}()

	waitForWorker(t, done)
}

func TestPublishAggregatesContinuesAfterPublishError(t *testing.T) {
	publisher := &recordingPublisher{err: errors.New("publish failed")}
	aggs := []*types.SignedAggregatedAttestation{{}, {}}

	publishAggregates(context.Background(), publisher, aggs)

	if publisher.count != len(aggs) {
		t.Fatalf("published=%d, want %d", publisher.count, len(aggs))
	}
}

func waitForWorker(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("worker did not exit")
	}
}

// The deadline is the slot's promotion boundary, so waiting on the prover or
// behind a previous session can consume it before this one starts. Running
// anyway produces nothing and raises the starvation warning, which is meant for
// a session that had time and still produced nothing — the alarm that surfaced
// the estimator latch. It must stay unambiguous.
func TestSessionSkipsDispatchPastItsDeadline(t *testing.T) {
	publisher := &recordingPublisher{}
	now := time.Unix(1_000, 0)
	worker := NewWorker(nil, insecure.Scheme{}, publisher, nil, shadow.Rates{}, func() time.Time { return now })

	worker.Session(context.Background(), Dispatch{
		Slot:     1,
		Snapshot: aggregateTestSnapshot(1),
		Deadline: now.Add(-time.Second),
	})

	if publisher.count != 0 {
		t.Fatalf("published=%d, want 0 for a dispatch past its deadline", publisher.count)
	}
}
