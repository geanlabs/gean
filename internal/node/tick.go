package node

import (
	"time"

	"github.com/geanlabs/gean/internal/aggregation"
	"github.com/geanlabs/gean/internal/metrics"
	"github.com/geanlabs/gean/internal/store"
)

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

	isAgg := e.AggCtl != nil && e.AggCtl.Get()

	hasProposal := false
	var proposerValidatorID uint64
	if currentInterval == 0 && currentSlot > 0 && !firstTick {
		proposerValidatorID, hasProposal = e.getOurProposer(currentSlot)
	}

	store.OnTick(e.Store, timestampMs, hasProposal)

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

	if currentInterval == 0 || currentInterval == 4 {
		e.updateHead()
	}

	if hasProposal {
		e.maybePropose(currentSlot, proposerValidatorID)
	}

	if currentInterval == 1 {
		e.runAttestationInterval(currentSlot)
	}

	if currentInterval == 3 {
		e.updateSafeTarget()
		store.PeriodicPrune(e.Store, e.FC, currentSlot, e.Store.LatestFinalized().Slot)
	}
}

func (e *Engine) runAttestationInterval(currentSlot uint64) {
	e.drainPendingBlocks()
	e.updateHead()
	e.produceAttestations(currentSlot)
	e.logChainStatus(currentSlot)
}
