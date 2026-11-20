package node

import (
	"time"

	"github.com/geanlabs/gean/internal/core/logger"
	"github.com/geanlabs/gean/internal/core/metrics"
	"github.com/geanlabs/gean/internal/lean/store"
	"github.com/geanlabs/gean/internal/lean/types"
)

func (e *Engine) onTick() {
	e.tickAt(e.Clock.Now())
}

// tickAt runs the duties of the interval containing now.
func (e *Engine) tickAt(now time.Time) {
	firstTick := e.lastTick.IsZero()
	if !firstTick {
		metrics.ObserveTickIntervalDuration(now.Sub(e.lastTick).Seconds())
	}
	e.lastTick = now
	e.lastTickMs.Store(now.UnixMilli())

	timestampMs := uint64(now.UnixMilli())

	currentSlot := e.currentSlot(timestampMs)
	currentInterval := e.currentInterval(timestampMs)

	// The startup tick lands wherever the process began, so only scheduled
	// ticks say anything about the clock's phase.
	if !firstTick {
		phaseMs := e.millisIntoSlot(timestampMs) % types.MillisecondsPerInterval
		metrics.ObserveTickPhase(float64(phaseMs) / 1000)
	}

	if !e.claimInterval(timestampMs) {
		logger.Warn(logger.Node, "tick skipped: interval already handled slot=%d interval=%d", currentSlot, currentInterval)
		return
	}

	metrics.SetCurrentSlot(currentSlot)
	e.updateSyncStatus(currentSlot)

	isAgg := e.AggCtl != nil && e.AggCtl.Get()

	hasProposal := false
	var proposerValidatorID uint64
	if currentInterval == 0 && currentSlot > 0 && !firstTick {
		proposerValidatorID, hasProposal = e.getOurProposer(currentSlot)
	}

	// Capture before OnTick promotes new payloads into known, so the timely
	// section reflects what had arrived by the promotion boundary.
	if snap := snapshotNewPayloadParticipants(e.Store); snap != nil {
		e.coveragePreMerge = snap
	}

	store.OnTick(e.Store, timestampMs, hasProposal)

	if currentInterval == 2 {
		e.reportAggStartNewCoverage()
		// Dispatch unconditionally, matching leanSpec's interval-2 aggregation:
		// a sole aggregator that also proposes next would otherwise never
		// aggregate at all. The proving gate's proposal priority only defers
		// next proof attempt. The session yields between proofs when a proposal
		// is waiting, but a native proof already running cannot be interrupted.
		e.Aggregator.StartSession(timestampMs, currentSlot, isAgg)
	}

	if currentInterval == 0 || currentInterval == 4 {
		e.updateHead()
	}
	// Like the interval-0 proposal, never propose from the startup tick.
	if currentInterval == 4 && !firstTick {
		e.maybeProposeEarly()
	}

	if hasProposal {
		e.maybePropose(currentSlot, proposerValidatorID)
	}

	if currentInterval == 1 {
		e.runAttestationInterval(currentSlot)
	}

	if currentInterval == 3 {
		e.updateSafeTarget()
		finalized := e.Store.LatestFinalized()
		headSlot := e.Store.HeadSlot()
		store.PruneStaleAttestationPools(e.Store, headSlot, finalized.Slot)
		store.PruneStaleStates(e.Store, e.FC, e.Store.Head(), headSlot, finalized, e.Store.LatestJustified())
		store.PeriodicPrune(e.Store, e.FC, currentSlot, finalized.Slot)
	}
}

func (e *Engine) runAttestationInterval(currentSlot uint64) {
	e.Importer.Drain()
	e.updateHead()
	e.produceAttestations(currentSlot)
	// Report the previous round: by now the head normally carries the block
	// proposed at currentSlot, which is the first block able to include votes
	// for currentSlot-1.
	if currentSlot > 0 {
		e.reportPostBlockCoverage(currentSlot - 1)
	}
	e.logChainStatus(currentSlot)
}
