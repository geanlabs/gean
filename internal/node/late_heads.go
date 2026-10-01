package node

import "github.com/geanlabs/gean/internal/types"

// lateHeadMs is how far into its slot a block must first reach this node to count
// as late. A block first seen at or after interval 3 has usually not reached the
// next slot's proposer either, so that proposer builds beside it and the votes
// naming it are the ones that decide the fork.
const lateHeadMs = 3 * types.MillisecondsPerInterval

// arrivalWindowSlots bounds the arrival record. Aggregation only ever weighs
// votes for recent heads, so older entries are dropped as the slot advances.
const arrivalWindowSlots = 64

type blockArrival struct {
	slot uint64
	ms   uint64
}

// recordBlockArrival keeps the first time this node saw a block. Later sightings
// (a re-gossip, a range response) say nothing about when it reached the network.
func (e *Engine) recordBlockArrival(root [32]byte, slot, nowMs uint64) {
	if _, seen := e.blockArrivals[root]; seen {
		return
	}
	if e.blockArrivals == nil {
		e.blockArrivals = make(map[[32]byte]blockArrival)
	}
	e.blockArrivals[root] = blockArrival{slot: slot, ms: nowMs}
}

// lateHeads returns the recent blocks this node first saw at or after lateHeadMs
// into their slot, and drops arrivals older than the window.
func (e *Engine) lateHeads(currentSlot uint64) map[[32]byte]bool {
	genesisMs := e.Store.Config().GenesisTime * 1000
	late := make(map[[32]byte]bool)
	for root, a := range e.blockArrivals {
		if a.slot+arrivalWindowSlots < currentSlot {
			delete(e.blockArrivals, root)
			continue
		}
		slotStart := genesisMs + a.slot*types.MillisecondsPerSlot
		if a.ms >= slotStart+lateHeadMs {
			late[root] = true
		}
	}
	return late
}
