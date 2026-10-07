package launch

import (
	"context"
	"errors"
	"testing"
)

// A service the node cannot run without must take the node down with it:
// Wait reports the failure and releases what Launch opened.
func TestServiceFailureStopsNode(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	n := &Node{ctx: ctx, cancel: cancel}
	released := false
	n.closers = append(n.closers, func() { released = true })

	n.services.GoCritical("api server", func() error { return errors.New("listen: address in use") }, n.fail)
	n.services.GoCritical("engine", func() error { <-ctx.Done(); return nil }, n.fail)

	err := n.Wait()
	if err == nil || err.Error() != "api server: listen: address in use" {
		t.Fatalf("Wait()=%v, want the api server failure", err)
	}
	if !released {
		t.Fatal("resources were not released after the services stopped")
	}
}
