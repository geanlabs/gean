package forkchoice

import (
	"github.com/geanlabs/gean/observability/logging"
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
	// Expand aggregated payloads into per-validator votes.
	newAggAttestations := extractAttestationsFromAggregatedPayloads(c.latestNewAggregatedPayloads)
	for vid, sa := range newAggAttestations {
		existing, ok := c.latestNewAttestations[vid]
		if !ok || existing == nil || existing.Message == nil || existing.Message.Slot < sa.Message.Slot {
			c.latestNewAttestations[vid] = sa
		}
	}
	c.latestKnownAggregatedPayloads = mergeAggregatedPayloads(c.latestKnownAggregatedPayloads, c.latestNewAggregatedPayloads)
	c.latestNewAggregatedPayloads = make(map[[32]byte]aggregatedPayload)
	metrics.LatestNewAggregatedPayloads.Set(0)

	// Move new → known and update head.
	for id, sa := range c.latestNewAttestations {
		c.latestKnownAttestations[id] = sa
	}
	c.latestNewAttestations = make(map[uint64]*types.SignedAttestation)
	metrics.LatestKnownAggregatedPayloads.Set(float64(len(c.latestKnownAggregatedPayloads)))
	c.updateHeadLocked()
}

func (c *Store) updateHeadLocked() {
	oldHead := c.head
	c.head = GetForkChoiceHead(c.allKnownBlockSummaries(), c.latestJustified.Root, c.latestKnownAttestations, 0)

	if depth, reorged := c.reorgDepth(oldHead, c.head); reorged {
		metrics.ForkChoiceReorgsTotal.Inc()
		metrics.ForkChoiceReorgDepth.Observe(float64(depth))
		log.Warn("fork choice reorg detected", "old_head", logging.ShortHash(oldHead), "new_head", logging.ShortHash(c.head), "depth", depth)
	}
}

// reorgDepth checks if a head change is a reorg (chain divergence, not a simple extension).
// Returns (depth, true) if reorg, (0, false) otherwise.
func (c *Store) reorgDepth(oldHead, newHead [32]byte) (uint64, bool) {
	if oldHead == newHead {
		return 0, false
	}
	oldSummary, oldOk := c.lookupBlockSummary(oldHead)
	newSummary, newOk := c.lookupBlockSummary(newHead)
	if !oldOk || !newOk {
		return 0, false
	}

	// Walk back from the higher-slot head to the lower-slot head's slot.
	var current [32]byte
	var targetSlot uint64
	var targetRoot [32]byte
	if newSummary.Slot >= oldSummary.Slot {
		current, targetSlot, targetRoot = newHead, oldSummary.Slot, oldHead
	} else {
		current, targetSlot, targetRoot = oldHead, newSummary.Slot, newHead
	}

	const maxDepth = 128
	var depth uint64
	for {
		summary, ok := c.lookupBlockSummary(current)
		if !ok {
			return 0, false
		}
		if summary.Slot <= targetSlot {
			return depth, current != targetRoot
		}
		current = summary.ParentRoot
		depth++
		if depth >= maxDepth {
			return depth, true
		}
	}
}

// UpdateSafeTarget finds the head with sufficient (2/3+) vote support.
func (c *Store) UpdateSafeTarget() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.updateSafeTargetLocked()
}

func (c *Store) updateSafeTargetLocked() {
	minScore := int(ceilDiv(c.numValidators*2, 3))
	mergedPayloads := make(map[[32]byte]aggregatedPayload)
	mergedPayloads = mergeAggregatedPayloads(mergedPayloads, c.latestKnownAggregatedPayloads)
	mergedPayloads = mergeAggregatedPayloads(mergedPayloads, c.latestNewAggregatedPayloads)
	attestations := extractAttestationsFromAggregatedPayloads(mergedPayloads)
	c.safeTarget = GetForkChoiceHead(c.allKnownBlockSummaries(), c.latestJustified.Root, attestations, minScore)
	if block, ok := c.lookupBlockSummary(c.safeTarget); ok {
		metrics.SafeTargetSlot.Set(float64(block.Slot))
	}
}
