package dutygate

import (
	"sync"
)

const (
	SyncLagThreshold      uint64 = 4
	NetworkStallThreshold uint64 = 8
	HysteresisBand        uint64 = 2
)

const (
	ReasonLocalLag     = "local_lag"
	ReasonCaughtUp     = "caught_up"
	ReasonNetworkStall = "network_stall"
)

type Event struct {
	Reason        string
	Duty          string
	Slot          uint64
	HeadSlot      uint64
	MaxStoredSlot uint64
	Lag           uint64
	NetworkLag    uint64
}

type Gate struct {
	mu           sync.Mutex
	closed       bool
	onTransition func(Event)
}

func New(onTransition ...func(Event)) *Gate {
	g := &Gate{}
	if len(onTransition) > 0 {
		g.onTransition = onTransition[0]
	}
	return g
}

func (g *Gate) IsClosed() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.closed
}

func (g *Gate) Decide(duty string, wallSlot, headSlot, maxStoredSlot uint64) bool {
	g.mu.Lock()
	next := decide(g.closed, duty, wallSlot, headSlot, maxStoredSlot)
	g.closed = next.closed
	g.mu.Unlock()

	g.emit(next.event)
	return next.allow
}

func (g *Gate) emit(event *Event) {
	if event == nil || g.onTransition == nil {
		return
	}
	g.onTransition(*event)
}

func gateEvent(reason, duty string, wallSlot, headSlot, maxStoredSlot, lag, networkLag uint64) *Event {
	return &Event{
		Reason:        reason,
		Duty:          duty,
		Slot:          wallSlot,
		HeadSlot:      headSlot,
		MaxStoredSlot: maxStoredSlot,
		Lag:           lag,
		NetworkLag:    networkLag,
	}
}

func slotLag(wallSlot, seenSlot uint64) uint64 {
	if wallSlot <= seenSlot {
		return 0
	}
	return wallSlot - seenSlot
}

func maxSlot(a, b uint64) uint64 {
	if a > b {
		return a
	}
	return b
}

func reopenLagThreshold() uint64 {
	if HysteresisBand >= SyncLagThreshold {
		return 0
	}
	return SyncLagThreshold - HysteresisBand
}
