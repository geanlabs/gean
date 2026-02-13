package forkchoice

import (
	"github.com/devylongs/gean/observability/metrics"
	"github.com/devylongs/gean/types"
)

// AdvanceTime advances the chain to the given wall-clock time.
func (c *Store) AdvanceTime(time uint64, hasProposal bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.advanceTimeLocked(time, hasProposal)
}

func (c *Store) advanceTimeLocked(time uint64, hasProposal bool) {
	if time <= c.Config.GenesisTime {
		return
	}
	tickInterval := (time - c.Config.GenesisTime) / types.SecondsPerInterval
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
			c.acceptNewVotesLocked()
		}
	case 1:
		// Validator voting interval — no action.
	case 2:
		c.updateSafeTargetLocked()
	case 3:
		c.acceptNewVotesLocked()
	}
}

// AcceptNewVotes moves pending votes to known and updates head.
func (c *Store) AcceptNewVotes() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.acceptNewVotesLocked()
}

func (c *Store) acceptNewVotesLocked() {
	for id, vote := range c.LatestNewVotes {
		c.LatestKnownVotes[id] = vote
	}
	c.LatestNewVotes = make(map[uint64]*types.Checkpoint)
	c.updateHeadLocked()
}

func (c *Store) updateHeadLocked() {
	if latest := GetLatestJustified(c.Storage); latest != nil {
		c.LatestJustified = latest
	}

	c.Head = GetForkChoiceHead(c.Storage, c.LatestJustified.Root, c.LatestKnownVotes, 0)

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
	minScore := int(ceilDiv(c.Config.NumValidators*2, 3))
	c.SafeTarget = GetForkChoiceHead(c.Storage, c.LatestJustified.Root, c.LatestNewVotes, minScore)
	if block, ok := c.Storage.GetBlock(c.SafeTarget); ok {
		metrics.SafeTargetSlot.Set(float64(block.Slot))
	}
}
