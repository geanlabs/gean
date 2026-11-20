package aggregation

import (
	"time"

	"github.com/geanlabs/gean/internal/lean/metrics"
	"github.com/geanlabs/gean/internal/lean/types"
)

// aggregationDeadlineOffset is how far into a slot an aggregation session must
// be finished: the interval-4 boundary, where new payloads are promoted to
// known and gossiped. Anything still proving past it misses the promotion it
// was produced for.
const aggregationDeadlineOffset = 4 * types.MillisecondsPerInterval

// StartSession dispatches currentSlot's aggregation session to the proving
// worker: at most once per slot, only on an aggregator, and only when there is
// something to aggregate.
func (a *Aggregator) StartSession(nowMs, currentSlot uint64, isAggregator bool) {
	if !isAggregator {
		metrics.IncAggregatorSkipped(metrics.AggregatorSkipNotAggregator)
		return
	}
	// Dispatch at most once per slot: the early attestation-arrival path and the
	// interval-2 fallback both route here, and the recursive proof is far too
	// expensive to run twice for the same slot.
	if currentSlot == a.aggregatedSlot {
		return
	}
	// Aggregation is deliberately NOT behind the sync-lag duty gate, unlike block
	// production and attestation.
	//
	// Gating it on the grounds that aggregates built on a stale view get dropped
	// anyway holds with several aggregators, but inverts with one:
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
	// deadline and MaxGroupsPerSession, so an aggregate built on a stale view
	// costs one bounded proving budget and is dropped by peers — strictly better
	// than not producing one at all.
	if a.cfg.Store.AttestationSignatures.Len() == 0 && a.cfg.Store.NewPayloads.Len() == 0 {
		metrics.IncAggregatorSkipped(metrics.AggregatorSkipOther)
		return
	}
	headState := a.cfg.Store.GetState(a.cfg.Store.Head())
	if headState == nil {
		metrics.IncAggregatorSkipped(metrics.AggregatorSkipMissingState)
		return
	}

	snap := SnapshotInputs(a.cfg.Store, headState, currentSlot)
	if snap == nil {
		metrics.IncAggregatorSkipped(metrics.AggregatorSkipOther)
		return
	}
	// A session holds the proving gate until it finishes, so a proposal duty
	// next slot waits on it however the gate's priority flag is set. Prove one
	// group in that case and leave the rest for the following session.
	maxGroups := MaxGroupsPerSession
	if a.proposingAt(currentSlot+1, headState.NumValidators()) {
		maxGroups = MaxGroupsWhenProposing
	}
	select {
	case a.dispatches <- Dispatch{
		Snapshot:  snap,
		Slot:      currentSlot,
		MaxGroups: maxGroups,
		Deadline:  a.deadline(nowMs),
	}:
		a.aggregatedSlot = currentSlot
		metrics.SetProvingQueueDepth("aggregation", len(a.dispatches))
	default:
		metrics.IncAggregationDispatchDropped()
		metrics.IncAggregatorSkipped(metrics.AggregatorSkipSpawnFailed)
	}
}

// deadline is the wall-clock instant a session dispatched now must
// stop by: this slot's interval-4 boundary. Measuring a fixed span from when
// the worker starts instead gives the early-interval-1 path a deadline earlier
// than the boundary, taking back most of the head start that path exists to
// create, and lets time spent waiting on the proving gate extend the session
// past the promotion it was produced for rather than come out of its window.
func (a *Aggregator) deadline(nowMs uint64) time.Time {
	intoSlot := a.msIntoSlot(nowMs)
	// Dispatch only runs at intervals 1 and 2, so a slot position at or past the
	// boundary is unreachable in practice. Handle it before subtracting: these
	// are unsigned milliseconds, so the difference would wrap rather than go
	// negative. Hand back a usable window instead of an expired deadline, which
	// the worker reads as "stop before the first group".
	if intoSlot >= aggregationDeadlineOffset {
		return time.UnixMilli(int64(nowMs)).Add(types.MillisecondsPerInterval * time.Millisecond)
	}
	window := time.Duration(aggregationDeadlineOffset-intoSlot) * time.Millisecond
	// Anchored to the tick's own timestamp rather than a fresh clock read, so
	// the deadline is the slot boundary itself and does not drift by however
	// long the tick took to reach here.
	return time.UnixMilli(int64(nowMs)).Add(window)
}

// MaybeEarly starts the aggregation session in late interval 1, once this
// slot's votes have reached quorum — a partial-interval lead ahead of the interval-2
// fallback — so the slow recursive proof gets a head start toward finishing inside
// its budget instead of racing the interval boundary and truncating. Gating on the
// slot's own vote count rather than a wall-clock lead keeps the timing tied to the
// slot model. It only moves *when the proving starts*: the aggregate still covers
// same-slot attestations and is gossiped within the slot, so it is spec-neutral.
// StartSession enforces the once-per-slot guard shared with the
// interval-2 fallback.
func (a *Aggregator) MaybeEarly(nowMs uint64) {
	isAgg := a.isAggregator()
	if !isAgg || a.intervalAt(nowMs) != 1 {
		return
	}
	slot := a.slotAt(nowMs)
	if slot == a.aggregatedSlot {
		return
	}
	// Validator set is fixed at genesis in lean devnet, so decode the head state
	// once and cache the count rather than on every arrival.
	if a.numValidators == 0 {
		headState := a.cfg.Store.GetState(a.cfg.Store.Head())
		if headState == nil {
			return
		}
		a.numValidators = headState.NumValidators()
		if a.numValidators == 0 {
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
	if a.cfg.Store.AttestationSignatures.SignatureCountForSlot(slot) < earlyAggregationQuorum(a.expectedVotersPerSlot()) {
		return
	}
	a.StartSession(nowMs, slot, isAgg)
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
func (a *Aggregator) expectedVotersPerSlot() uint64 {
	// Read once per verified attestation, so it is cached rather than recounted.
	// The validator set is fixed at genesis in lean devnet and the subnet
	// subscription is fixed at startup, so the answer cannot change.
	if a.expectedVoters != 0 {
		return a.expectedVoters
	}
	total := a.numValidators
	committees, subnetIDs := a.subnets()
	if committees <= 1 || len(subnetIDs) == 0 || uint64(len(subnetIDs)) >= committees {
		a.expectedVoters = total
		return total
	}
	subscribed := make(map[uint64]bool, len(subnetIDs))
	for _, id := range subnetIDs {
		subscribed[id] = true
	}
	var voters uint64
	for vid := range total {
		if subscribed[vid%committees] {
			voters++
		}
	}
	a.expectedVoters = voters
	return voters
}

func (a *Aggregator) isAggregator() bool {
	return a.cfg.IsAggregator != nil && a.cfg.IsAggregator()
}

func (a *Aggregator) proposingAt(slot, numValidators uint64) bool {
	return a.cfg.ProposingAt != nil && a.cfg.ProposingAt(slot, numValidators)
}

func (a *Aggregator) subnets() (uint64, []uint64) {
	if a.cfg.Subnets == nil {
		return 0, nil
	}
	return a.cfg.Subnets()
}
