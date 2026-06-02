package node

import (
	"time"

	"github.com/geanlabs/gean/internal/aggregation"
	"github.com/geanlabs/gean/internal/metrics"
	"github.com/geanlabs/gean/internal/store"
)

// onTick processes an 800ms tick event.
func (e *Engine) onTick() {
	now := time.Now()
	firstTick := e.lastTick.IsZero()
	if !firstTick {
		metrics.ObserveTickIntervalDuration(now.Sub(e.lastTick).Seconds())
	}
	e.lastTick = now

	timestampMs := uint64(now.UnixMilli())

	currentSlot := e.currentSlot(timestampMs)
	currentInterval := e.currentInterval(timestampMs)

	metrics.SetCurrentSlot(currentSlot)
	e.updateSyncStatus(currentSlot)
	e.refreshGossipMeshPeers()

	// Snapshot the aggregator role once per tick. A mid-tick toggle must
	// not cause OnTick below and the interval-2 branch to observe different
	// values (store_tick relies on the bool being stable for the tick).
	isAgg := e.AggCtl.Get()

	// Check if we're the proposer for this slot. Suppressed on the eager
	// pre-ticker tick so a fresh node doesn't grow its proto-array before
	// any RPC poll or peer observation.
	hasProposal := false
	var proposerValidatorID uint64
	if currentInterval == 0 && currentSlot > 0 && !firstTick {
		proposerValidatorID, hasProposal = e.getOurProposer(currentSlot)
	}

	// Tick the store — handles interval dispatch (promote attestations).
	// Aggregation is handled async below to avoid blocking the tick loop.
	store.OnTick(e.Store, timestampMs, hasProposal, isAgg)

	// Interval 2: snapshot synchronously, dispatch to the worker goroutine.
	// See runAggregationWorker for the off-tick rationale + drop semantics.
	if currentInterval == 2 && isAgg {
		snap := aggregation.SnapshotInputs(e.Store)
		if snap != nil {
			select {
			case e.AggregationDispatchCh <- aggregation.Dispatch{Snapshot: snap, Slot: currentSlot}:
			default:
				metrics.IncAggregationDispatchDropped()
			}
		}
	}

	// Interval 0/4: update head after attestation promotion.
	// Must run BEFORE proposal so the builder uses the freshest head.
	if currentInterval == 0 || currentInterval == 4 {
		e.updateHead()
	}

	// Interval 0: propose block if we're the proposer.
	if hasProposal {
		e.maybePropose(currentSlot, proposerValidatorID)
	}

	// Interval 1: produce attestations + chain status log.
	// Drain pending blocks first so all nodes converge on the same head
	// before attesting. Without this, Go's select may fire the tick before
	// processing a pending block, causing attestations with a stale head
	// and divergent target/source roots across nodes.
	if currentInterval == 1 {
		e.drainPendingBlocks()
		e.updateHead()
		e.produceAttestations(currentSlot)
		e.logChainStatus(currentSlot)
	}

	// Interval 3: update safe target + periodic pruning fallback.
	if currentInterval == 3 {
		e.updateSafeTarget()
		store.PeriodicPrune(e.Store, e.FC, currentSlot, e.Store.LatestFinalized().Slot)
	}
}
