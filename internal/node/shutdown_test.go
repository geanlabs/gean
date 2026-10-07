package node

import (
	"context"
	"sync/atomic"
	"testing"
)

// Shutdown closes the storage backend and releases keys as soon as Run returns.
// A worker still running at that point (an in-flight proof, a per-message
// verification) would then call into a closed Pebble instance, which panics, so
// Run must not return before every engine-owned goroutine has finished.
func TestRunReturnsOnlyAfterEngineWorkFinishes(t *testing.T) {
	e := makeTestEngine()
	ctx, cancel := context.WithCancel(context.Background())

	var finished atomic.Bool
	release := make(chan struct{})
	e.workers.Go(func() {
		<-release
		finished.Store(true)
	})

	done := make(chan struct{})
	go func() {
		e.Run(ctx)
		close(done)
	}()

	cancel()
	go close(release)
	<-done
	if !finished.Load() {
		t.Fatal("Run returned while engine-owned work was still running")
	}
	if e.workers.Go(func() {}) {
		t.Fatal("engine accepted new work after Run returned")
	}
}
