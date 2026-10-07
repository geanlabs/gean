package tasks

import (
	"sync/atomic"
	"testing"
)

func TestWaitJoinsRunningWorkAndRefusesNewWork(t *testing.T) {
	var g Group
	var finished atomic.Bool
	release := make(chan struct{})
	if !g.Go(func() {
		<-release
		finished.Store(true)
	}) {
		t.Fatal("Go refused work on an open group")
	}

	go close(release)
	g.Wait()
	if !finished.Load() {
		t.Fatal("Wait returned before tracked work finished")
	}

	if g.Go(func() {}) {
		t.Fatal("Go accepted work after Wait")
	}
	if g.Do(func() {}) {
		t.Fatal("Do accepted work after Wait")
	}
}
