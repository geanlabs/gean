package forkchoice

import (
	"github.com/geanlabs/gean/observability/metrics"
	"github.com/geanlabs/gean/types"
)

// AdvanceTime advances the chain to the given unix time in seconds.
//
// This wrapper is kept for second-based callers.
func (c *Store) AdvanceTime(unixSeconds uint64, hasProposal bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.advanceTimeLocked(unixSeconds, hasProposal)
}

// AdvanceTimeMillis advances the chain to the given unix time in milliseconds.
func (c *Store) AdvanceTimeMillis(unixMillis uint64, hasProposal bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.advanceTimeLockedMillis(unixMillis, hasProposal)
}

func (c *Store) advanceTimeLocked(unixSeconds uint64, hasProposal bool) {
	c.advanceTimeLockedMillis(unixSeconds*1000, hasProposal)
}

func (c *Store) advanceTimeLockedMillis(unixMillis uint64, hasProposal bool) {
	genesisTimeMillis := c.genesisTime * 1000
	if unixMillis <= genesisTimeMillis {
		return
	}
	tickInterval := (unixMillis - genesisTimeMillis) / types.MillisecondsPerInterval
	for c.time < tickInterval {
		shouldSignal := hasProposal && (c.time+1) == tickInterval
		c.tickIntervalLocked(shouldSignal)
	}
}

// TickInterval advances by one interval and performs interval-specific actions.
func (c *Store) TickInterval(hasProposal bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tickIntervalLocked(hasProposal)
}

func (c *Store) tickIntervalLocked(hasProposal bool) {
	c.time++
	currentInterval := c.time % types.IntervalsPerSlot

	switch currentInterval {
	case 0:
		if hasProposal {
			c.acceptNewAttestationsLocked()
		}
	case 1:
		// Validator voting interval — no action.
	case 2:
		// Committee aggregation interval — handled outside the store.
	case 3:
		c.updateSafeTargetLocked()
	case 4:
		c.acceptNewAttestationsLocked()
	}
}

// AcceptNewAttestations moves pending attestations to known and updates head.
func (c *Store) AcceptNewAttestations() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.acceptNewAttestationsLocked()
}

func (c *Store) acceptNewAttestationsLocked() {
	for id, sa := range c.latestNewAttestations {
		c.latestKnownAttestations[id] = sa
	}
	c.latestNewAttestations = make(map[uint64]*types.SignedAttestation)
	c.updateHeadLocked()
}

func (c *Store) updateHeadLocked() {
	c.head = GetForkChoiceHead(c.storage, c.latestJustified.Root, c.latestKnownAttestations, 0)
}

// UpdateSafeTarget finds the head with sufficient (2/3+) vote support.
func (c *Store) UpdateSafeTarget() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.updateSafeTargetLocked()
}

func (c *Store) updateSafeTargetLocked() {
	minScore := int(ceilDiv(c.numValidators*2, 3))
	c.safeTarget = GetForkChoiceHead(c.storage, c.latestJustified.Root, c.latestNewAttestations, minScore)
	if block, ok := c.storage.GetBlock(c.safeTarget); ok {
		metrics.SafeTargetSlot.Set(float64(block.Slot))
	}
}
