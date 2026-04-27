package node

import "sync/atomic"

// AggregatorController holds the runtime aggregator-role flag exposed by the
// admin API (leanSpec PR #636). Reads are lock-free via atomic.Bool; Set
// atomically swaps the value and returns the previous state so the HTTP
// handler can report {"previous": ...}. The Prometheus lean_is_aggregator
// gauge is synced on every transition so observability stays accurate across
// toggles.
//
// Contract: each logical decision that branches on is_aggregator should call
// Get() exactly once and cache the result locally if the same value is
// needed in multiple places within the same decision. Re-reading mid-
// decision can observe a toggle in flight.
type AggregatorController struct {
	flag atomic.Bool
}

// NewAggregatorController seeds the controller with initial and syncs the
// Prometheus gauge to match, so metrics are correct from boot onward.
func NewAggregatorController(initial bool) *AggregatorController {
	c := &AggregatorController{}
	c.flag.Store(initial)
	SetIsAggregator(initial)
	return c
}

// Get returns the current aggregator-role flag.
func (c *AggregatorController) Get() bool {
	return c.flag.Load()
}

// Set atomically swaps the flag to v and returns the previous value. The
// Prometheus gauge is updated only when the value actually changes.
func (c *AggregatorController) Set(v bool) bool {
	prev := c.flag.Swap(v)
	if prev != v {
		SetIsAggregator(v)
	}
	return prev
}
