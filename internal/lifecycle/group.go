// Package lifecycle tracks the goroutines that must return before the
// resources they use are closed.
package lifecycle

import "sync"

// Group tracks goroutines for shutdown. Close stops admitting new ones before
// it waits, so a goroutine cannot start after the wait has begun. The zero
// value is ready to use.
type Group struct {
	closedMu sync.Mutex
	closed   bool
	wg       sync.WaitGroup
}

// Enter registers the calling goroutine. It reports false once Close has been
// called; on true, the caller must call Leave when it returns.
func (g *Group) Enter() bool {
	g.closedMu.Lock()
	defer g.closedMu.Unlock()
	if g.closed {
		return false
	}
	g.wg.Add(1)
	return true
}

// Leave marks a goroutine registered by Enter as returned.
func (g *Group) Leave() {
	g.wg.Done()
}

// Go runs fn on a tracked goroutine. It reports false, without running fn,
// once Close has been called.
func (g *Group) Go(fn func()) bool {
	if !g.Enter() {
		return false
	}
	go func() {
		defer g.Leave()
		fn()
	}()
	return true
}

// Close stops admitting goroutines and waits for the tracked ones to return.
// Callers cancel the work first; Close does not.
func (g *Group) Close() {
	g.closedMu.Lock()
	g.closed = true
	g.closedMu.Unlock()
	g.wg.Wait()
}
