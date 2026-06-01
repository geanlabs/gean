package dutygate

import (
	"sync"

	"github.com/geanlabs/gean/internal/logger"
)

// Validator duty-gate thresholds.
// Informative, not normative: they shape when this node signs without
// changing what consensus accepts. Clients may diverge without breaking
// interop.
const (
	// SyncLagThreshold is the slot lag past which the local view is too
	// stale to sign. Roughly one full justification window behind real time.
	SyncLagThreshold uint64 = 4

	// NetworkStallThreshold is the lag past which the entire network is
	// treated as stalled. Set to 2 * SyncLagThreshold so ordinary jitter
	// at the local boundary cannot trip the network-stall branch.
	NetworkStallThreshold uint64 = 8

	// HysteresisBand keeps the gate closed near the threshold. Once closed,
	// the gate reopens only when lag drops to SyncLagThreshold - HysteresisBand.
	HysteresisBand uint64 = 2
)

// Gate decides whether validator duties may run for a given slot, given
// the local store's head and the freshest block slot the store has observed.
// State is per-node (one local store, one gate); a single instance lives on
// Engine.
//
// Transitions (open ↔ closed) are logged; intermediate checks are silent.
type Gate struct {
	mu     sync.Mutex
	closed bool
}

// New returns a Gate in the open state.
func New() *Gate {
	return &Gate{}
}

// IsClosed reports the gate's current state. Concurrency-safe.
func (g *Gate) IsClosed() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.closed
}

// Decide returns true if a duty may run for wallSlot. duty is a free-form
// label ("block", "attestation", ...) included in transition log lines.
//
//   - headSlot       — slot of the canonical chain head from local store.
//   - maxStoredSlot  — max slot across all blocks already validated into
//     the store. Authenticated lower bound on the network tip because
//     only signature-verified blocks enter the map. A stale max here
//     means the network is not producing.
//
// Decision matrix:
//
//   - network stalling (network_lag > NetworkStallThreshold):
//     keep signing; reopen the gate if it was closed.
//   - gate currently closed: reopen only when lag <= SyncLagThreshold - HysteresisBand.
//   - gate currently open:   close as soon as lag > SyncLagThreshold.
func (g *Gate) Decide(duty string, wallSlot, headSlot, maxStoredSlot uint64) bool {
	lag := uint64(0)
	if wallSlot > headSlot {
		lag = wallSlot - headSlot
	}
	networkLag := uint64(0)
	if wallSlot > maxStoredSlot {
		networkLag = wallSlot - maxStoredSlot
	}
	networkStalling := networkLag > NetworkStallThreshold

	g.mu.Lock()
	defer g.mu.Unlock()

	if networkStalling {
		if g.closed {
			g.closed = false
			logger.Info(logger.Validator, "duty gate reopened: network stall detected. duty=%s slot=%d head_slot=%d lag=%d max_seen_slot=%d network_lag=%d",
				duty, wallSlot, headSlot, lag, maxStoredSlot, networkLag)
		}
		return true
	}

	if g.closed {
		allow := lag <= SyncLagThreshold-HysteresisBand
		if allow {
			g.closed = false
			logger.Info(logger.Validator, "duty gate reopened: local view caught up. duty=%s slot=%d head_slot=%d lag=%d",
				duty, wallSlot, headSlot, lag)
		}
		return allow
	}

	allow := lag <= SyncLagThreshold
	if !allow {
		g.closed = true
		logger.Info(logger.Validator, "duty gate closed: local view is stale. duty=%s slot=%d head_slot=%d lag=%d max_seen_slot=%d network_lag=%d",
			duty, wallSlot, headSlot, lag, maxStoredSlot, networkLag)
	}
	return allow
}
