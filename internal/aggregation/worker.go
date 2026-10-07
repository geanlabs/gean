package aggregation

import (
	"context"
	"time"

	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/metrics"
	"github.com/geanlabs/gean/internal/proving"
	"github.com/geanlabs/gean/internal/shadow"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
	"github.com/geanlabs/gean/xmss"
)

type Publisher interface {
	PublishAggregatedAttestation(context.Context, *types.SignedAggregatedAttestation) error
}

type Dispatch struct {
	Snapshot *Snapshot
	Slot     uint64
	// Deadline is the instant the session must stop starting groups by, set by
	// the dispatcher from the slot clock. Zero falls back to SessionBudget
	// measured from when the worker acquires the prover.
	Deadline time.Time
	// Backlog marks a session the dispatch loop offered because the worker sat
	// idle outside the interval-2 session. Its proving time comes out of the
	// pacer's budget, it starts only when a whole proof fits, and it gives way
	// to the next dispatch as well as to a proposal.
	Backlog bool
	// Early marks a backlog window before interval 2. This slot's votes are
	// left to its interval-2 session: they are still arriving, and proving them
	// then spends a proof on part of the coverage. The window runs out of work
	// separately from the one after interval 2.
	Early bool
}

// SessionBudget caps one aggregation session's proving time. Dispatch fires
// at interval 2, so two intervals of proving still leaves interval 4 for the
// results to be promoted and gossiped, and bounds how long the proving gate
// is held away from proposals. Exported so other prover users can stay clear
// of the window this session occupies rather than fork the constant.
const SessionBudget = 2 * types.MillisecondsPerInterval * time.Millisecond

// AcquirePatience is how long a dispatched session waits for the prover before
// giving up on the slot's aggregate.
const AcquirePatience = 750 * time.Millisecond

// provedAtRetention is how long the worker remembers a proved data root. A
// session starts within a slot of its snapshot, so older entries can go.
const provedAtRetention = 2 * types.SecondsPerSlot * time.Second

type worker struct {
	dispatches <-chan Dispatch
	store      *store.ConsensusStore
	cache      *xmss.PubKeyCache
	publisher  Publisher
	gate       *proving.Gate
	pacer      *Pacer
	shadow     shadow.Rates
	// One estimator lives across dispatches so it keeps calibrating to this
	// node's real per-unit prover cost. The worker is single-threaded, so no
	// locking is needed.
	estimator *unitCostEstimator
	// provedAt is when each data root was last proved. Several sessions can
	// run in a slot, and the interval-2 snapshot may be taken while a backlog
	// session is still proving, so a snapshot can carry signatures a later
	// proof already consumed; its session skips those roots.
	provedAt map[[32]byte]time.Time
	prove    func([]xmss.CPubKey, []xmss.CSig, []xmss.ChildProof, [32]byte, uint32) ([]byte, error)
}

func RunWorker(
	ctx context.Context,
	dispatches <-chan Dispatch,
	consensusStore *store.ConsensusStore,
	cache *xmss.PubKeyCache,
	publisher Publisher,
	gate *proving.Gate,
	pacer *Pacer,
	shadowRates shadow.Rates,
) {
	if pacer == nil {
		pacer = NewPacer()
	}
	w := &worker{
		dispatches: dispatches,
		store:      consensusStore,
		cache:      cache,
		publisher:  publisher,
		gate:       gate,
		pacer:      pacer,
		shadow:     shadowRates,
		estimator:  newUnitCostEstimator(),
		provedAt:   make(map[[32]byte]time.Time),
		prove:      xmss.AggregateWithChildren,
	}
	for {
		select {
		case <-ctx.Done():
			return
		case dispatch, ok := <-dispatches:
			if !ok {
				return
			}
			metrics.SetProvingQueueDepth("aggregation", len(dispatches))
			if dispatch.Snapshot == nil {
				continue
			}
			pacer.idle.Store(false)
			w.session(ctx, dispatch)
			pacer.idle.Store(true)
		}
	}
}

func (w *worker) session(ctx context.Context, dispatch Dispatch) {
	w.pacer.beginSlot(dispatch.Slot)
	acquireCtx, cancelAcquire := context.WithTimeout(ctx, AcquirePatience)
	acquired := w.gate == nil || w.gate.Acquire(acquireCtx, false)
	cancelAcquire()
	if !acquired {
		// Backlog work is optional: its votes stay pooled for a later session.
		if dispatch.Backlog {
			return
		}
		metrics.IncProofOperation("aggregation", "canceled")
		// A lost prover costs this slot its aggregate and slows justification;
		// without this line the loss shows only in metrics, a silent gap in the logs.
		logger.Warn(logger.Signature, "aggregation skipped: prover unavailable slot=%d", dispatch.Slot)
		return
	}
	release := func() {
		if w.gate != nil {
			w.gate.Release(false)
		}
	}

	// The session budget bounds how long the gate is held, not which
	// results survive: groups are proven frontier-first (ascending
	// target slot, see orderedGroups) and every completed aggregate is
	// applied and published even when the budget cuts the session
	// short. Discarding finished aggregates
	// (and their signature deletes) regrows the next snapshot until
	// no session can ever finish inside a slot.
	workerStart := time.Now()
	deadline := dispatch.Deadline
	if deadline.IsZero() {
		deadline = workerStart.Add(SessionBudget)
	}
	yield := w.yieldFor(dispatch.Backlog)
	budgetBound := false
	if dispatch.Backlog {
		if limit := workerStart.Add(w.pacer.remaining()); limit.Before(deadline) {
			deadline, budgetBound = limit, true
		}
		// A proof cannot be stopped once started, so a backlog proof begun
		// without room to finish would hold the prover into the window the
		// interval-2 session or a proposal needs. Starting only when the
		// estimate fits also covers an expired deadline; strictFit holds every
		// later proof, children included, to the same rule.
		if time.Until(deadline) < w.estimator.nextGroupDuration() {
			if budgetBound {
				w.pacer.refuse(dispatch.Slot)
			} else {
				w.pacer.finish(dispatch.Slot, dispatch.Early)
			}
			release()
			return
		}
	} else if !time.Now().Before(deadline) {
		// The deadline is the slot's promotion boundary, so waiting on the
		// gate or behind a previous session can consume it entirely. An
		// aggregate finished after that boundary misses the promotion it was
		// produced for, so there is nothing to gain by proving one. Report it
		// as its own case: running the session anyway would produce no output
		// and raise the starvation warning, which is meant for a session that
		// had time and still produced nothing.
		metrics.IncProofOperation("aggregation", "expired")
		logger.Warn(logger.Signature, "aggregation skipped: past the promotion boundary slot=%d late_by=%v", dispatch.Slot, time.Since(deadline))
		release()
		return
	}
	snap := dispatch.Snapshot
	snap.skipCurrent = dispatch.Early
	snap.strictFit = dispatch.Backlog
	snap.skipRoots = w.provedSince(snap.takenAt)

	// The window is what the dispatcher actually allowed, which is less
	// than SessionBudget whenever the gate was held for a while.
	budget := deadline.Sub(workerStart)
	aggs, payloads, deletes, truncated, skips := aggregateFromSnapshotWithProver(yield, snap, w.cache, deadline, w.shadow, w.estimator, w.prove)
	workerElapsed := time.Since(workerStart)
	release()
	if dispatch.Backlog {
		w.pacer.charge(workerElapsed)
		switch {
		case budgetBound && skips[metrics.AggGroupSkipBudget] > 0:
			w.pacer.refuse(dispatch.Slot)
		case !truncated:
			w.pacer.finish(dispatch.Slot, dispatch.Early)
		}
	}
	if len(aggs) > 0 && w.estimator.perGroupSeconds > 0 {
		w.pacer.observeCost(w.estimator.nextGroupDuration())
	}
	w.recordProved(payloads)
	if truncated && !dispatch.Backlog {
		metrics.IncProofOperation("aggregation", "truncated")
		switch {
		case skips[metrics.AggGroupSkipProposalPending] > 0:
			logger.Info(logger.Signature, "aggregation yielded to proposal: slot=%d produced=%d duration=%v", dispatch.Slot, len(aggs), workerElapsed)
		case len(aggs) == 0:
			// No output plus a budget stop is the actionable starvation case.
			logger.Warn(logger.Signature, "aggregation session hit budget without output: slot=%d produced=0 duration=%v", dispatch.Slot, workerElapsed)
		case workerElapsed > budget:
			// A proof exceeded the wall-clock budget; keep this visible even
			// though partial results were preserved and published.
			logger.Warn(logger.Signature, "aggregation session overran budget: slot=%d produced=%d duration=%v budget=%v", dispatch.Slot, len(aggs), workerElapsed, budget)
		default:
			// Normal partial completion: the admission estimate stopped a
			// later group while the completed output stayed within budget.
			logger.Info(logger.Signature, "aggregation session truncated after partial output: slot=%d produced=%d duration=%v budget=%v", dispatch.Slot, len(aggs), workerElapsed, budget)
		}
	}
	applyAggregationMutations(w.store, payloads, deletes)
	publishCtx, cancelPublish := context.WithTimeout(ctx, types.MillisecondsPerInterval*time.Millisecond)
	publishAggregates(publishCtx, w.publisher, aggs)
	cancelPublish()
	// A session that dropped every group is not a success. Counting it
	// as one is what let an aggregator produce nothing for 355
	// consecutive slots on devnet-5 while the success rate read 100%.
	// A backlog session that found nothing provable is routine, not a
	// failed slot, so it is counted only when it produced something.
	switch {
	case len(aggs) > 0:
		metrics.IncProofOperation("aggregation", "success")
	case !dispatch.Backlog:
		metrics.IncProofOperation("aggregation", "empty")
	}
	metrics.ObserveProvingDuration("aggregation", workerElapsed.Seconds())
	metrics.ObserveAggregationWorkerTotalTime(workerElapsed.Seconds())
	for reason, n := range skips {
		metrics.IncAggregationGroupSkipped(reason, n)
	}
	// A backlog session that ran out of groups with nothing provable is the
	// normal shape of a pool of single votes; logging it would repeat every
	// slot of a stall.
	if dispatch.Backlog && len(aggs) == 0 && !truncated {
		return
	}
	// Report why a session produced little or nothing. produced=0 alone
	// cannot distinguish an idle aggregator from one dropping every group.
	skipSummary := ""
	if s := skips.summary(); s != "" {
		skipSummary = " skipped=" + s
	}
	kind := ""
	if dispatch.Backlog {
		kind = " kind=backlog"
	}
	logger.Info(logger.Signature, "aggregation worker: slot=%d produced=%d duration=%v%s%s",
		dispatch.Slot, len(aggs), workerElapsed, skipSummary, kind)
}

// yieldFor is asked before each proof. Every session gives way to a waiting
// proposal; a backlog session also gives way to the next dispatch, which is
// the interval-2 session or a fresher snapshot.
func (w *worker) yieldFor(backlog bool) func() string {
	return func() string {
		if w.gate.ProposalPending() {
			return metrics.AggGroupSkipProposalPending
		}
		if backlog && len(w.dispatches) > 0 {
			return metrics.AggGroupSkipSuperseded
		}
		return ""
	}
}

// provedSince returns the roots proved after t.
func (w *worker) provedSince(t time.Time) map[[32]byte]bool {
	var roots map[[32]byte]bool
	for root, at := range w.provedAt {
		if at.After(t) {
			if roots == nil {
				roots = make(map[[32]byte]bool)
			}
			roots[root] = true
		}
	}
	return roots
}

func (w *worker) recordProved(payloads []store.PayloadKV) {
	now := time.Now()
	for root, at := range w.provedAt {
		if now.Sub(at) > provedAtRetention {
			delete(w.provedAt, root)
		}
	}
	for _, p := range payloads {
		w.provedAt[p.DataRoot] = now
	}
}
