package statetransition

// Observability hooks, assigned by the node package at engine start. Nil-safe.
// Mirrors the pattern used by p2p.GossipBlockSizeHook for cross-package metric
// emission without a circular import (statetransition must not import node).
//
// Hooks are assigned once before any state transition runs; reads never race
// with writes.
var (
	ObserveTotalTimeHook         func(seconds float64)
	ObserveSlotsTimeHook         func(seconds float64)
	ObserveBlockTimeHook         func(seconds float64)
	ObserveAttestationsTimeHook  func(seconds float64)
	IncSlotsProcessedHook        func(n uint64)
	IncAttestationsProcessedHook func(n uint64)
)
