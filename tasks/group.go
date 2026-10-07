// Package tasks tracks the goroutines a component owns so it can wait for all
// of them before releasing the resources they use.
package tasks

import (
	"fmt"
	"runtime/debug"
	"sync"
)

// Group tracks goroutines and synchronous calls started on behalf of one owner.
// Once Wait begins, the group is closed: Go and Do refuse new work, so nothing
// can start after the owner has decided to release its resources.
type Group struct {
	mu     sync.Mutex
	closed bool
	wg     sync.WaitGroup
}

// Go runs fn on a new goroutine tracked by the group. It returns false without
// running fn if the group is closed.
func (g *Group) Go(fn func()) bool {
	if !g.enter() {
		return false
	}
	go func() {
		defer g.wg.Done()
		fn()
	}()
	return true
}

// GoCritical runs fn like Go for a service its owner cannot run without. If
// fn returns an error or panics, fail is called with the cause so the owner
// can shut down rather than keep running without the service.
func (g *Group) GoCritical(name string, fn func() error, fail func(error)) bool {
	return g.Go(func() {
		defer func() {
			if r := recover(); r != nil {
				fail(fmt.Errorf("%s panicked: %v\n%s", name, r, debug.Stack()))
			}
		}()
		if err := fn(); err != nil {
			fail(fmt.Errorf("%s: %w", name, err))
		}
	})
}

// Do runs fn on the calling goroutine, tracked by the group, for work started
// by goroutines the group does not own (network stream handlers, callbacks).
// It returns false without running fn if the group is closed.
func (g *Group) Do(fn func()) bool {
	if !g.enter() {
		return false
	}
	defer g.wg.Done()
	fn()
	return true
}

// Wait closes the group and blocks until every tracked goroutine and call has
// returned. Cancelling a context does not stop native calls in progress, so
// Wait also waits for those to finish.
func (g *Group) Wait() {
	g.mu.Lock()
	g.closed = true
	g.mu.Unlock()
	g.wg.Wait()
}

func (g *Group) enter() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return false
	}
	g.wg.Add(1)
	return true
}
