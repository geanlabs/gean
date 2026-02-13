package forkchoice

import (
	"github.com/geanlabs/gean/observability/metrics"
	"github.com/geanlabs/gean/types"
)

// AdvanceTime advances the chain to the given wall-clock time.
func (c *Store) AdvanceTime(time uint64, hasProposal bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.advanceTimeLocked(time, hasProposal)
}

func (c *Store) advanceTimeLocked(time uint64, hasProposal bool) {
	if time <= c.GenesisTime {
		return
	}
	tickInterval := (time - c.GenesisTime) / types.SecondsPerInterval
	for c.Time < tickInterval {
		shouldSignal := hasProposal && (c.Time+1) == tickInterval
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
	c.Time++
	currentInterval := c.Time % types.IntervalsPerSlot

	switch currentInterval {
	case 0:
		if hasProposal {
			c.acceptNewAttestationsLocked()
		}
	case 1:
		// Validator voting interval — no action.
	case 2:
		c.updateSafeTargetLocked()
	case 3:
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
	for id, att := range c.LatestNewAttestations {
		c.LatestKnownAttestations[id] = att
	}
	c.LatestNewAttestations = make(map[uint64]*types.Attestation)
	c.updateHeadLocked()
}

func (c *Store) updateHeadLocked() {
	if latest := GetLatestJustified(c.Storage); latest != nil {
		c.LatestJustified = latest
	}

	c.Head = GetForkChoiceHead(c.Storage, c.LatestJustified.Root, c.LatestKnownAttestations, 0)

	if headState, ok := c.Storage.GetState(c.Head); ok {
		c.LatestFinalized = headState.LatestFinalized
	}
}

// UpdateSafeTarget finds the head with sufficient (2/3+) vote support.
func (c *Store) UpdateSafeTarget() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.updateSafeTargetLocked()
}

func (c *Store) updateSafeTargetLocked() {
	minScore := int(ceilDiv(c.NumValidators*2, 3))
	c.SafeTarget = GetForkChoiceHead(c.Storage, c.LatestJustified.Root, c.LatestNewAttestations, minScore)
	if block, ok := c.Storage.GetBlock(c.SafeTarget); ok {
		metrics.SafeTargetSlot.Set(float64(block.Slot))
	}
}
