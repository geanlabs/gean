package node

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/geanlabs/gean/internal/storage"
)

// heldBackend holds the first EstimateTableBytes call until release is closed,
// so a test can keep a worker inside a storage call while shutdown runs.
type heldBackend struct {
	storage.Backend
	once     sync.Once
	entered  chan struct{}
	release  chan struct{}
	returned atomic.Bool
}

func (b *heldBackend) EstimateTableBytes(table storage.Table) uint64 {
	held := false
	b.once.Do(func() { held = true })
	if held {
		close(b.entered)
		<-b.release
		defer b.returned.Store(true)
	}
	return b.Backend.EstimateTableBytes(table)
}

// Wait must not return while a worker is still inside a storage call: the
// caller closes the database as soon as it does.
func TestWaitJoinsWorkerHeldInStorage(t *testing.T) {
	e := makeTestEngine()
	backend := &heldBackend{Backend: e.Store.Backend, entered: make(chan struct{}), release: make(chan struct{})}
	e.Store.Backend = backend

	ctx, cancel := context.WithCancel(context.Background())
	e.Start(ctx)
	<-backend.entered // the storage-size sampler is inside the backend

	cancel()
	waited := make(chan struct{})
	go func() {
		e.Wait()
		close(waited)
	}()
	// Every other worker returns on cancel, so a Wait that missed the sampler
	// returns well within this window. Passing never depends on it: Wait
	// cannot return before release below.
	select {
	case <-waited:
		t.Fatal("Wait returned while a worker was still inside a storage call")
	case <-time.After(earlyReturnWindow):
	}
	close(backend.release)
	<-waited
	if !backend.returned.Load() {
		t.Fatal("Wait returned before the worker left its storage call")
	}
}

// earlyReturnWindow is how long a test watches for a wait that returns too
// early. It only detects that bug; no test needs it to pass.
const earlyReturnWindow = 100 * time.Millisecond

// A worker blocked on a full result queue must still return on cancel, or
// Wait would hang shutdown.
func TestWaitReturnsWithProposalResultQueueFull(t *testing.T) {
	e := makeTestEngine()
	e.ProposalResultCh <- &proposalResult{} // nothing drains it

	ctx, cancel := context.WithCancel(context.Background())
	e.tasks.Go(func() { e.runProposalWorker(ctx) })
	// ProposalCh holds one duty, so the second send returns only once the
	// worker has taken the first. Slot 0 is stale, so the worker answers at
	// once and blocks on the full result queue.
	e.ProposalCh <- proposalDuty{}
	e.ProposalCh <- proposalDuty{}

	cancel()
	e.Wait()
}
