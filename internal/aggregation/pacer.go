package aggregation

import (
	"sync/atomic"
	"time"

	"github.com/geanlabs/gean/internal/metrics"
	"github.com/geanlabs/gean/internal/types"
)

// backlogBudgetMax is the most proving time per slot that backlog sessions may
// take on top of the interval-2 session. Every backlog window ends inside the
// slot it is charged to, so with that session's two intervals the prover stays
// free for at least one interval of every slot.
const backlogBudgetMax = 2 * types.MillisecondsPerInterval * time.Millisecond

// backlogBudgetStep is how much the budget grows after a slot that wanted more
// and showed no strain. Cuts halve it, so the budget climbs over many slots and
// falls within a few.
const backlogBudgetStep = types.MillisecondsPerInterval / 8 * time.Millisecond

// tickLateLimit is how long after its boundary a tick may be handled before the
// node counts as behind on its own consensus work. Falling behind is how a
// saturated host showed up before: four aggregators on one 16-core host left a
// node 129 slots behind.
const tickLateLimit = types.MillisecondsPerInterval / 4 * time.Millisecond

// proofSlowFactor marks CPU contention: the smoothed proof cost at this multiple
// of the node's own recent best means something else is competing for the cores.
const proofSlowFactor = 2

// baselineDrift is the share (1/n) of a slower reading the baseline absorbs per
// unflagged slot, so a lasting change in proof cost eventually becomes the new
// normal without contention being written off within a few slots.
const baselineDrift = 64

// Pacer sizes how much proving time backlog aggregation may use per slot. No
// fixed value suits every machine gean runs on, so the budget tunes itself from
// two signals the node measures against itself: ticks handled late, and proofs
// running slower than this node's recent best. It grows slowly while a slot
// wants more and neither shows, and halves when either does. At zero, aggregation
// is the interval-2 session alone.
//
// The dispatch loop reads idle, closed and done and writes tickLag; everything
// else is owned by the worker goroutine.
type Pacer struct {
	idle    atomic.Bool
	closed  atomic.Uint64 // slot+1 of the slot whose budget is spent; 0 when none
	done    atomic.Uint64 // windowKey of the last window out of work; 0 when none
	tickLag atomic.Int64  // worst tick lateness since the last slot boundary, ns

	started  bool
	slot     uint64
	budget   time.Duration
	spent    time.Duration
	limited  bool // the budget turned work away this slot
	slow     bool // proofs ran slow this slot
	cost     time.Duration
	baseline time.Duration
}

// windowKey names one of a slot's two backlog windows: the one before interval
// 2, and the one after. Never zero, so zero can mean none.
func windowKey(slot uint64, early bool) uint64 {
	if early {
		return 2*slot + 1
	}
	return 2*slot + 2
}

func NewPacer() *Pacer {
	p := &Pacer{}
	p.idle.Store(true)
	return p
}

// ReportTickLag records how late the dispatch loop handled a tick.
func (p *Pacer) ReportTickLag(lag time.Duration) {
	if p == nil {
		return
	}
	for {
		cur := p.tickLag.Load()
		if int64(lag) <= cur || p.tickLag.CompareAndSwap(cur, int64(lag)) {
			return
		}
	}
}

// BacklogOpen reports whether a backlog session dispatched in slot's early
// (before interval 2) or late window would run: the worker is waiting, the
// slot's budget is not spent, and the window still has work. Checked before
// the snapshot is built, so a busy worker costs the dispatch loop nothing.
func (p *Pacer) BacklogOpen(slot uint64, early bool) bool {
	return p != nil && p.idle.Load() && p.closed.Load() != slot+1 && p.done.Load() != windowKey(slot, early)
}

// beginSlot adjusts the budget once per slot, from what the previous slot showed.
func (p *Pacer) beginSlot(slot uint64) {
	if p.started && slot <= p.slot {
		return
	}
	if p.started {
		p.adjust()
	}
	p.started = true
	p.slot = slot
	p.spent = 0
	p.limited = false
	p.slow = false
}

func (p *Pacer) adjust() {
	if !p.slow && p.cost > p.baseline {
		p.baseline += (p.cost - p.baseline) / baselineDrift
	}
	lag := time.Duration(p.tickLag.Swap(0))
	switch {
	case lag > tickLateLimit:
		p.budget /= 2
		metrics.IncAggregationBacklogBudgetChange(metrics.BacklogBudgetTickLate)
	case p.slow:
		p.budget /= 2
		metrics.IncAggregationBacklogBudgetChange(metrics.BacklogBudgetProofSlow)
	case p.limited && p.budget < backlogBudgetMax:
		// Only a slot the budget actually constrained says more would be used;
		// growing on idle slots would hand a large budget to the first backlog
		// that arrives, before any signal about this host has been read.
		p.budget = min(p.budget+backlogBudgetStep, backlogBudgetMax)
		metrics.IncAggregationBacklogBudgetChange(metrics.BacklogBudgetGrow)
	}
	metrics.SetAggregationBacklogBudget(p.budget.Seconds())
}

func (p *Pacer) remaining() time.Duration {
	return max(p.budget-p.spent, 0)
}

func (p *Pacer) charge(d time.Duration) {
	p.spent += d
}

// refuse marks the slot's budget as spent: the dispatch loop stops offering
// backlog work until the next slot, and the slot counts toward growing it.
func (p *Pacer) refuse(slot uint64) {
	p.limited = true
	p.closed.Store(slot + 1)
}

// finish closes one window without counting it as demand: a backlog session
// went through every group it had, or the window is too short for one proof,
// so another snapshot in it would find the same outcome.
func (p *Pacer) finish(slot uint64, early bool) {
	p.done.Store(windowKey(slot, early))
}

// observeCost takes the smoothed per-proof cost after a session and flags the
// slot when it runs well above this node's recent best. The baseline follows a
// faster reading at once and a slower one only at the slot boundary.
func (p *Pacer) observeCost(cost time.Duration) {
	if cost <= 0 {
		return
	}
	p.cost = cost
	if p.baseline == 0 || cost < p.baseline {
		p.baseline = cost
		return
	}
	if cost > proofSlowFactor*p.baseline {
		p.slow = true
	}
}
