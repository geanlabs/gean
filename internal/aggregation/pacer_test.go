package aggregation

import (
	"testing"
	"time"
)

// A budget that grew on idle slots would hand its full size to the first
// backlog that arrives, before anything about this host has been read. It grows
// only after a slot the budget actually constrained.
func TestPacerGrowsOnlyAfterALimitedSlot(t *testing.T) {
	p := NewPacer()
	p.beginSlot(1)
	p.beginSlot(2)
	if p.budget != 0 {
		t.Fatalf("budget = %v after an idle slot, want 0", p.budget)
	}

	p.refuse(2)
	p.beginSlot(3)
	if p.budget != backlogBudgetStep {
		t.Fatalf("budget = %v after a limited slot, want %v", p.budget, backlogBudgetStep)
	}
}

func TestPacerNeverExceedsMax(t *testing.T) {
	p := NewPacer()
	for slot := uint64(1); slot < 100; slot++ {
		p.beginSlot(slot)
		p.refuse(slot)
	}
	if p.budget != backlogBudgetMax {
		t.Fatalf("budget = %v, want the max %v", p.budget, backlogBudgetMax)
	}
}

// A late tick means the node is behind on its own consensus work, the first
// sign of the saturation that once left a node 129 slots behind. It must cut
// the budget even in a slot that also wanted more.
func TestPacerHalvesOnLateTick(t *testing.T) {
	for _, tc := range []struct {
		name string
		lag  time.Duration
		want time.Duration
	}{
		{"on_time", tickLateLimit, backlogBudgetMax},
		{"late", tickLateLimit + time.Millisecond, backlogBudgetMax / 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := NewPacer()
			p.budget = backlogBudgetMax
			p.beginSlot(1)
			p.refuse(1)
			p.ReportTickLag(tc.lag)
			p.beginSlot(2)
			if p.budget != tc.want {
				t.Fatalf("budget = %v, want %v", p.budget, tc.want)
			}
			if p.tickLag.Load() != 0 {
				t.Fatal("tick lag was not reset at the slot boundary")
			}
		})
	}
}

func TestPacerKeepsTheWorstTickLag(t *testing.T) {
	p := NewPacer()
	p.ReportTickLag(300 * time.Millisecond)
	p.ReportTickLag(10 * time.Millisecond)
	if got := time.Duration(p.tickLag.Load()); got != 300*time.Millisecond {
		t.Fatalf("tick lag = %v, want the worst report", got)
	}
	var nilPacer *Pacer
	nilPacer.ReportTickLag(time.Second)
}

// Proofs slowing well past this node's own recent best mean the cores are
// contended, which shows before ticks run late on a shared host.
func TestPacerHalvesWhenProofsRunSlow(t *testing.T) {
	p := NewPacer()
	p.budget = backlogBudgetMax
	p.beginSlot(1)
	p.observeCost(800 * time.Millisecond)
	p.observeCost(1500 * time.Millisecond) // under twice the best: normal spread
	p.refuse(1)
	p.beginSlot(2)
	if p.budget != backlogBudgetMax {
		t.Fatalf("budget = %v after normal spread, want %v", p.budget, backlogBudgetMax)
	}

	p.observeCost(1700 * time.Millisecond) // over twice the best
	p.beginSlot(3)
	if p.budget != backlogBudgetMax/2 {
		t.Fatalf("budget = %v after slow proofs, want %v", p.budget, backlogBudgetMax/2)
	}
}

// The baseline follows a faster reading at once. A slower one moves it only
// at a slot boundary, by a fraction, and not at all while the slot is flagged
// slow: drifting on every session, often fed the same smoothed value, wrote
// lasting contention off as normal within a few slots.
func TestPacerBaselineFollowsTheBestAndDriftsOncePerSlot(t *testing.T) {
	p := NewPacer()
	p.beginSlot(1)
	p.observeCost(time.Second)
	p.observeCost(600 * time.Millisecond)
	if p.baseline != 600*time.Millisecond {
		t.Fatalf("baseline = %v, want the faster reading", p.baseline)
	}
	for range 10 {
		p.observeCost(1100 * time.Millisecond)
	}
	if p.baseline != 600*time.Millisecond {
		t.Fatalf("baseline = %v moved within the slot", p.baseline)
	}
	p.beginSlot(2)
	if want := 600*time.Millisecond + 500*time.Millisecond/baselineDrift; p.baseline != want {
		t.Fatalf("baseline = %v after one slot, want %v", p.baseline, want)
	}

	before := p.baseline
	p.observeCost(3 * before)
	p.beginSlot(3)
	if p.baseline != before {
		t.Fatalf("baseline = %v drifted toward a slow slot, want %v", p.baseline, before)
	}
}

func TestPacerAdjustsOncePerSlot(t *testing.T) {
	p := NewPacer()
	p.beginSlot(5)
	p.refuse(5)
	p.beginSlot(5)
	p.beginSlot(4)
	if p.budget != 0 || !p.limited {
		t.Fatalf("budget = %v limited = %v, want no adjustment within the slot", p.budget, p.limited)
	}
}

func TestPacerBacklogOpen(t *testing.T) {
	p := NewPacer()
	if !p.BacklogOpen(7, true) || !p.BacklogOpen(7, false) {
		t.Fatal("an idle pacer with budget left should be open")
	}
	p.finish(7, true)
	if p.BacklogOpen(7, true) || !p.BacklogOpen(7, false) {
		t.Fatal("finishing a window should close only that window")
	}
	p.refuse(7)
	if p.BacklogOpen(7, false) {
		t.Fatal("a spent budget should close the whole slot")
	}
	if !p.BacklogOpen(8, true) || !p.BacklogOpen(8, false) {
		t.Fatal("the next slot should open again")
	}
	p.idle.Store(false)
	if p.BacklogOpen(8, true) {
		t.Fatal("a busy worker should not be offered work")
	}
	var nilPacer *Pacer
	if nilPacer.BacklogOpen(8, true) {
		t.Fatal("a nil pacer should never be open")
	}
}

func TestPacerChargesSpentTime(t *testing.T) {
	p := NewPacer()
	p.budget = time.Second
	p.beginSlot(1)
	p.charge(700 * time.Millisecond)
	if got := p.remaining(); got != 300*time.Millisecond {
		t.Fatalf("remaining = %v, want 300ms", got)
	}
	p.charge(time.Second)
	if got := p.remaining(); got != 0 {
		t.Fatalf("remaining = %v after overspend, want 0", got)
	}
	p.beginSlot(2)
	if got := p.remaining(); got != time.Second {
		t.Fatalf("remaining = %v in a new slot, want the full budget", got)
	}
}
