package node

import (
	"time"

	"github.com/geanlabs/gean/internal/aggregation"
	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/metrics"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

// aggregationDeadlineOffset is how far into a slot an aggregation session must
// be finished: the interval-4 boundary, where new payloads are promoted to
// known and gossiped. Anything still proving past it misses the promotion it
// was produced for.
const aggregationDeadlineOffset = 4 * types.MillisecondsPerInterval

// proposingDeadlineOffset ends the interval-2 session an interval early in the
// slot before this node proposes. A proof cannot be interrupted, so the gap
// leaves room for one overrunning proof to finish before the proposal needs
// the prover.
const proposingDeadlineOffset = 3 * types.MillisecondsPerInterval

func (e *Engine) onTick() {
	now := time.Now()
	firstTick := e.lastTick.IsZero()
	var sinceLastTick time.Duration
	if !firstTick {
		sinceLastTick = now.Sub(e.lastTick)
		metrics.ObserveTickIntervalDuration(sinceLastTick.Seconds())
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
		e.AggregationPacer.ReportTickLag(tickLateness(phaseMs, sinceLastTick))
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
		e.dispatchAggregationCycle(timestampMs, currentSlot, isAgg)
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
		finalizedSlot := e.Store.LatestFinalized().Slot
		store.PruneStaleAttestationPools(e.Store, e.Store.HeadSlot(), finalizedSlot)
		store.PeriodicPrune(e.Store, e.FC, currentSlot, finalizedSlot)
	}

	// Last, so building a snapshot never delays this interval's duties.
	if currentInterval != 2 {
		e.dispatchBacklog(timestampMs, currentSlot, currentInterval, isAgg)
	}
}

// tickLateness is how long after its interval boundary a tick was handled. The
// phase wraps once a tick is a whole interval late, so the gap since the
// previous tick bounds it as well.
func tickLateness(phaseMs uint64, sinceLastTick time.Duration) time.Duration {
	return max(time.Duration(phaseMs)*time.Millisecond, sinceLastTick-types.MillisecondsPerInterval*time.Millisecond)
}

func (e *Engine) dispatchAggregationCycle(nowMs, currentSlot uint64, isAggregator bool) {
	if !isAggregator {
		metrics.IncAggregatorSkipped(metrics.AggregatorSkipNotAggregator)
		return
	}
	// Dispatch at most once per slot: the early attestation-arrival path and the
	// interval-2 fallback both route here, and the recursive proof is far too
	// expensive to run twice for the same slot.
	if currentSlot == e.aggregatedSlot {
		return
	}
	// Aggregation is deliberately NOT behind the sync-lag duty gate, unlike block
	// production and attestation.
	//
	// gean used to gate it, reasoning that aggregates built on a stale view get
	// dropped anyway. That holds with several aggregators. It inverts with one:
	// the sole aggregator withholds the aggregates the network is waiting on at
	// exactly the moment it is furthest behind, so nothing justifies, nothing
	// finalizes, finalization pruning never runs, and the lag that closed the
	// gate gets worse. Observed on devnet-5 as not_synced climbing to ~4,100 per
	// node with justification frozen; stopping two lagging nodes advanced
	// justification 1,520 slots in five minutes.
	//
	// The spec agrees: leanSpec timeline.py gates interval 2 on `is_aggregator`
	// alone. Gating it on anything else is gean's own addition, and the paragraph
	// above is why it has to go.
	//
	// The work is bounded without the gate: a session has a slot-anchored
	// deadline, so an aggregate built on a stale view costs one bounded proving
	// budget and is dropped by peers — strictly better than not producing one
	// at all.
	if e.Store.AttestationSignatures.Len() == 0 && e.Store.NewPayloads.Len() == 0 {
		metrics.IncAggregatorSkipped(metrics.AggregatorSkipOther)
		return
	}
	headState := e.aggregationHeadState()
	if headState == nil {
		metrics.IncAggregatorSkipped(metrics.AggregatorSkipMissingState)
		return
	}

	snap := aggregation.SnapshotInputs(e.Store, headState, currentSlot)
	if snap == nil {
		metrics.IncAggregatorSkipped(metrics.AggregatorSkipOther)
		return
	}
	deadlineOffset := uint64(aggregationDeadlineOffset)
	if e.proposingAt(currentSlot+1, headState.NumValidators()) {
		deadlineOffset = proposingDeadlineOffset
	}
	select {
	case e.AggregationDispatchCh <- aggregation.Dispatch{
		Snapshot: snap,
		Slot:     currentSlot,
		Deadline: e.aggregationDeadline(nowMs, deadlineOffset),
	}:
		e.aggregatedSlot = currentSlot
		metrics.SetProvingQueueDepth("aggregation", len(e.AggregationDispatchCh))
	default:
		metrics.IncAggregationDispatchDropped()
		metrics.IncAggregatorSkipped(metrics.AggregatorSkipSpawnFailed)
	}
}

// dispatchBacklog offers the worker backlog work while it sits idle outside
// the interval-2 session, so votes the interval-2 session had no time for are
// proved during the rest of the slot instead of waiting a slot each. The
// pacer decides how much of that time it may use; this only picks the window,
// each ending inside the slot so its proving is charged to the slot it uses:
//
//   - interval 0: until interval 2, leaving this slot's votes to its own
//     session. Not offered again at interval 1, where the early aggregation
//     path fires on this slot's votes.
//   - interval 3: until interval 4, like the interval-2 session.
//   - interval 4: until the end of the slot.
//
// None is offered while the block this node proposes needs the prover: at
// interval 0 of that slot, and from interval 3 of the slot before, where the
// interval-2 session already stops early to leave room for an overrun.
//
// A backlog session gives way as soon as another dispatch arrives.
func (e *Engine) dispatchBacklog(nowMs, slot, interval uint64, isAggregator bool) {
	var deadlineOffset uint64
	switch interval {
	case 0:
		deadlineOffset = 2 * types.MillisecondsPerInterval
	case 3:
		deadlineOffset = aggregationDeadlineOffset
	case 4:
		deadlineOffset = types.MillisecondsPerSlot
	default:
		return
	}
	early := interval == 0
	if !isAggregator || !e.AggregationPacer.BacklogOpen(slot, early) {
		return
	}
	if e.Store.AttestationSignatures.Len() == 0 && e.Store.NewPayloads.Len() == 0 {
		return
	}
	headState := e.aggregationHeadState()
	if headState == nil {
		return
	}
	proposalSlot := slot + 1
	if early {
		proposalSlot = slot
	}
	if e.proposingAt(proposalSlot, headState.NumValidators()) {
		return
	}
	snap := aggregation.SnapshotInputs(e.Store, headState, slot)
	if snap == nil {
		return
	}
	select {
	case e.AggregationDispatchCh <- aggregation.Dispatch{
		Snapshot: snap,
		Slot:     slot,
		Deadline: e.aggregationDeadline(nowMs, deadlineOffset),
		Backlog:  true,
		Early:    early,
	}:
	default:
	}
}

// aggregationHeadState returns the head state aggregation snapshots are built
// on, decoding it only when the head has moved: GetState decodes the whole
// state from SSZ, and backlog dispatches can ask several times a slot. Sessions
// only read it, so one decoded copy is shared.
func (e *Engine) aggregationHeadState() *types.State {
	head := e.Store.Head()
	if e.aggHeadState == nil || head != e.aggHeadRoot {
		e.aggHeadState = e.Store.GetState(head)
		e.aggHeadRoot = head
	}
	return e.aggHeadState
}

// aggregationDeadline is the wall-clock instant a session dispatched now must
// stop by: offsetMs into the current slot, which may reach into the next one.
// Measuring a fixed span from when the worker starts instead gives the
// early-interval-1 path a deadline earlier than the boundary, taking back most
// of the head start that path exists to create, and lets time spent waiting on
// the proving gate extend the session past the promotion it was produced for
// rather than come out of its window.
func (e *Engine) aggregationDeadline(nowMs, offsetMs uint64) time.Time {
	intoSlot := e.millisIntoSlot(nowMs)
	// Each dispatch point sits before its boundary, so a slot position at or
	// past it is unreachable in practice. Handle it before subtracting: these
	// are unsigned milliseconds, so the difference would wrap rather than go
	// negative. Hand back a usable window instead of an expired deadline, which
	// the worker reads as "stop before the first group".
	if intoSlot >= offsetMs {
		return time.UnixMilli(int64(nowMs)).Add(types.MillisecondsPerInterval * time.Millisecond)
	}
	window := time.Duration(offsetMs-intoSlot) * time.Millisecond
	// Anchored to the tick's own timestamp rather than a fresh clock read, so
	// the deadline is the slot boundary itself and does not drift by however
	// long the tick took to reach here.
	return time.UnixMilli(int64(nowMs)).Add(window)
}

// maybeEarlyAggregate starts the aggregation session in late interval 1, once this
// slot's votes have reached quorum — a partial-interval lead ahead of the interval-2
// fallback — so the slow recursive proof gets a head start toward finishing inside
// its budget instead of racing the interval boundary and truncating. Gating on the
// slot's own vote count rather than a wall-clock lead keeps the timing tied to the
// slot model. It only moves *when the proving starts*: the aggregate still covers
// same-slot attestations and is gossiped within the slot, so it is spec-neutral.
// dispatchAggregationCycle enforces the once-per-slot guard shared with the
// interval-2 fallback.
func (e *Engine) maybeEarlyAggregate(nowMs uint64) {
	isAgg := e.AggCtl != nil && e.AggCtl.Get()
	if !isAgg || e.currentInterval(nowMs) != 1 {
		return
	}
	slot := e.currentSlot(nowMs)
	if slot == e.aggregatedSlot {
		return
	}
	// Validator set is fixed at genesis in lean devnet, so decode the head state
	// once and cache the count rather than on every arrival.
	if e.numValidators == 0 {
		headState := e.Store.GetState(e.Store.Head())
		if headState == nil {
			return
		}
		e.numValidators = headState.NumValidators()
		if e.numValidators == 0 {
			return
		}
	}
	// Only pull the session forward once a finalizing supermajority of *this slot's*
	// votes is already collected. The count must be scoped to the current slot: a
	// cross-slot backlog would satisfy the threshold at the very start of interval 1,
	// before the slot's own attestations have propagated, and bundling then starves
	// justification. Scoped to the slot, the threshold is reached only in late
	// interval 1 once votes are in — a modest proving lead that still carries
	// decisive weight, with the interval-2 dispatch as the fallback below quorum.
	if e.Store.AttestationSignatures.SignatureCountForSlot(slot) < earlyAggregationQuorum(e.expectedVotersPerSlot()) {
		return
	}
	e.dispatchAggregationCycle(nowMs, slot, isAgg)
}

// earlyAggregationQuorum is the 3SF supermajority ceil(2n/3) — the same threshold
// the fork choice uses for justification. At that many collected votes an early
// aggregate already carries finalizing weight, so proving it ahead of interval 2
// is worthwhile.
func earlyAggregationQuorum(voters uint64) int {
	return int((2*voters + 2) / 3)
}

// expectedVotersPerSlot is how many validators this node can expect to hear from
// in a slot: those assigned to the subnets it subscribes to. A validator votes on
// subnet index % CommitteeCount, and the node only receives the subnets it joined.
//
// Measuring the quorum against the whole registry instead makes it unreachable
// for any aggregator covering a subset of subnets, so the early path never fires
// and the head start it exists to give is never taken. With one committee, or an
// aggregator subscribed to every subnet, this is the whole registry as before.
func (e *Engine) expectedVotersPerSlot() uint64 {
	// Read once per verified attestation, so it is cached rather than recounted.
	// The validator set is fixed at genesis in lean devnet and the subnet
	// subscription is fixed at startup, so the answer cannot change.
	if e.expectedVoters != 0 {
		return e.expectedVoters
	}
	total := e.numValidators
	committees := e.CommitteeCount
	if committees <= 1 || len(e.AggregateSubnetIDs) == 0 || uint64(len(e.AggregateSubnetIDs)) >= committees {
		e.expectedVoters = total
		return total
	}
	subscribed := make(map[uint64]bool, len(e.AggregateSubnetIDs))
	for _, id := range e.AggregateSubnetIDs {
		subscribed[id] = true
	}
	var voters uint64
	for vid := range total {
		if subscribed[vid%committees] {
			voters++
		}
	}
	e.expectedVoters = voters
	return voters
}

func (e *Engine) runAttestationInterval(currentSlot uint64) {
	e.drainPendingBlocks()
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
