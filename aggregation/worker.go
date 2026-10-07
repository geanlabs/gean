package aggregation

import (
	"context"
	"time"

	"github.com/geanlabs/gean/crypto"
	"github.com/geanlabs/gean/logger"
	"github.com/geanlabs/gean/metrics"
	"github.com/geanlabs/gean/proving"
	"github.com/geanlabs/gean/shadow"
	"github.com/geanlabs/gean/store"
	"github.com/geanlabs/gean/types"
)

type Publisher interface {
	PublishAggregatedAttestation(context.Context, *types.SignedAggregatedAttestation) error
}

type Dispatch struct {
	Snapshot *Snapshot
	Slot     uint64
	// MaxGroups bounds how many groups this session hands to the prover. Zero
	// means MaxGroupsPerSession; the dispatcher lowers it when this node
	// proposes next slot and would otherwise wait on the session for the gate.
	MaxGroups int
	// Deadline is the instant the session must stop starting groups by, set by
	// the dispatcher from the slot clock. Zero falls back to SessionBudget
	// measured from when the worker acquires the prover.
	Deadline time.Time
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

// Worker runs aggregation sessions. Run drives it from a dispatch channel;
// Session runs one dispatch on the calling goroutine, so a caller that owns
// scheduling (a deterministic simulation) can order sessions itself.
type Worker struct {
	store       *store.ConsensusStore
	scheme      crypto.Scheme
	publisher   Publisher
	gate        *proving.Gate
	shadowRates shadow.Rates
	// now is the consensus clock that session deadlines are measured against.
	now     func() time.Time
	metrics *metrics.Metrics
	// One estimator lives across sessions so it keeps calibrating to this
	// node's real per-unit prover cost. Sessions never overlap, so it needs no
	// locking.
	estimator *unitCostEstimator
}

func NewWorker(
	consensusStore *store.ConsensusStore,
	scheme crypto.Scheme,
	publisher Publisher,
	gate *proving.Gate,
	shadowRates shadow.Rates,
	now func() time.Time,
	m *metrics.Metrics,
) *Worker {
	return &Worker{
		store:       consensusStore,
		scheme:      scheme,
		publisher:   publisher,
		gate:        gate,
		shadowRates: shadowRates,
		now:         now,
		metrics:     m,
		estimator:   newUnitCostEstimator(),
	}
}

// Run runs a session for each dispatch until ctx ends or dispatches closes.
func (w *Worker) Run(ctx context.Context, dispatches <-chan Dispatch) {
	for {
		select {
		case <-ctx.Done():
			return
		case dispatch, ok := <-dispatches:
			if !ok {
				return
			}
			w.metrics.SetProvingQueueDepth("aggregation", len(dispatches))
			w.Session(ctx, dispatch)
		}
	}
}

// Session acquires the prover, aggregates the dispatched snapshot within its
// deadline, applies the results to the store and publishes them.
func (w *Worker) Session(ctx context.Context, dispatch Dispatch) {
	if dispatch.Snapshot == nil {
		return
	}
	acquireCtx, cancelAcquire := context.WithTimeout(ctx, AcquirePatience)
	if w.gate != nil && !w.gate.Acquire(acquireCtx, false) {
		cancelAcquire()
		w.metrics.IncProofOperation("aggregation", "canceled")
		// A lost prover costs this slot its aggregate and slows justification;
		// without this line the loss shows only in metrics, a silent gap in the logs.
		logger.Warn(logger.Signature, "aggregation skipped: prover unavailable slot=%d", dispatch.Slot)
		return
	}
	cancelAcquire()

	// The session budget bounds how long the gate is held, not which
	// results survive: groups are proven frontier-first (ascending
	// target slot, see orderedGroups) and every completed aggregate is
	// applied and published even when the budget cuts the session
	// short. Discarding finished aggregates
	// (and their signature deletes) regrows the next snapshot until
	// no session can ever finish inside a slot.
	workerStart := time.Now()
	sessionStart := w.now()
	deadline := dispatch.Deadline
	if deadline.IsZero() {
		deadline = sessionStart.Add(SessionBudget)
	}
	// The deadline is the slot's promotion boundary, so waiting on the
	// gate or behind a previous session can consume it entirely. An
	// aggregate finished after that boundary misses the promotion it was
	// produced for, so there is nothing to gain by proving one. Report it
	// as its own case: running the session anyway would produce no output
	// and raise the starvation warning, which is meant for a session that
	// had time and still produced nothing.
	if !sessionStart.Before(deadline) {
		w.metrics.IncProofOperation("aggregation", "expired")
		logger.Warn(logger.Signature, "aggregation skipped: past the promotion boundary slot=%d late_by=%v", dispatch.Slot, sessionStart.Sub(deadline))
		if w.gate != nil {
			w.gate.Release(false)
		}
		return
	}
	// The window is what the dispatcher actually allowed, which is less
	// than SessionBudget whenever the gate was held for a while.
	budget := deadline.Sub(sessionStart)
	aggs, payloads, deletes, truncated, skips := aggregateFromSnapshot(w.gate.ProposalPending, dispatch.Snapshot, deadline, w.now, dispatch.MaxGroups, w.shadowRates, w.estimator, w.scheme.Aggregate, w.metrics)
	workerElapsed := time.Since(workerStart)
	if w.gate != nil {
		w.gate.Release(false)
	}
	if truncated {
		w.metrics.IncProofOperation("aggregation", "truncated")
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
	if len(aggs) > 0 {
		w.metrics.IncProofOperation("aggregation", "success")
	} else {
		w.metrics.IncProofOperation("aggregation", "empty")
	}
	w.metrics.ObserveProvingDuration("aggregation", workerElapsed.Seconds())
	w.metrics.ObserveAggregationWorkerTotalTime(workerElapsed.Seconds())
	for reason, n := range skips {
		w.metrics.IncAggregationGroupSkipped(reason, n)
	}
	// Report why a session produced little or nothing. produced=0 alone
	// cannot distinguish an idle aggregator from one dropping every group.
	skipSummary := ""
	if s := skips.summary(); s != "" {
		skipSummary = " skipped=" + s
	}
	logger.Info(logger.Signature, "aggregation worker: slot=%d produced=%d duration=%v%s",
		dispatch.Slot, len(aggs), workerElapsed, skipSummary)
}
