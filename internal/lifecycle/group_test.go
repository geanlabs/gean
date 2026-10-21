package lifecycle

import (
	"testing"
	"time"
)

func TestCloseWaitsForRunningGoroutine(t *testing.T) {
	var g Group
	started, release := make(chan struct{}), make(chan struct{})
	g.Go(func() {
		close(started)
		<-release
	})
	<-started

	closed := make(chan struct{})
	go func() {
		g.Close()
		close(closed)
	}()
	// Only detects an early return; passing never depends on the window.
	select {
	case <-closed:
		t.Fatal("Close returned while a goroutine was still running")
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	<-closed
}

// Go admits through Enter, so a closed group starts nothing.
func TestClosedGroupAdmitsNothing(t *testing.T) {
	var g Group
	g.Close()
	if g.Enter() {
		t.Fatal("Enter reported true after Close")
	}
}
