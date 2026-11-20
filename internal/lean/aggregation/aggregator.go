package aggregation

import (
	"context"

	"github.com/geanlabs/gean/internal/core/clock"
	"github.com/geanlabs/gean/internal/lean/proving"
	"github.com/geanlabs/gean/internal/lean/shadow"
	"github.com/geanlabs/gean/internal/lean/store"
	"github.com/geanlabs/gean/internal/lean/types"
)

// Config is what an Aggregator needs. The functions answer questions only the
// node can: its role, its validators' duties, its sync status and its subnets.
type Config struct {
	Store *store.ConsensusStore
	Clock clock.Clock
	// Gate is the prover token shared with proposals. Nil means no sharing.
	Gate *proving.Gate
	// Publisher gossips aggregates. Nil disables publishing.
	Publisher Publisher
	Shadow    shadow.Rates

	// IsAggregator reports whether this node aggregates right now.
	IsAggregator func() bool
	// ProposingAt reports whether one of this node's validators proposes at
	// slot, given the validator count.
	ProposingAt func(slot, numValidators uint64) bool
	// ProposerAt reports whether one of this node's validators proposes at
	// slot, reading the validator count from the head state.
	ProposerAt func(slot uint64) bool
	// Synced reports whether the node is synced.
	Synced func() bool
	// Subnets returns the attestation committee count and the subnets this
	// node aggregates (empty means all). It is read when needed, because the
	// node may set the subnets after New.
	Subnets func() (committeeCount uint64, subnetIDs []uint64)
}

// Aggregator decides when this node aggregates attestation signatures into
// proofs, runs the proving sessions, and recovers per-attestation proofs from
// imported blocks for re-broadcast.
//
// StartSession, MaybeEarly and DispatchRecovery run on the node's single
// dispatch goroutine, which owns the scheduling state. PokeEarly is safe from
// any goroutine; RunSessions and RunRecovery run on their own.
type Aggregator struct {
	cfg Config

	// dispatches carries sessions to the proving worker.
	dispatches chan Dispatch
	// early is a coalescing (capacity-1) wake-up: an attestation-verify
	// goroutine pokes it after inserting a signature, and the dispatch loop
	// reacts by considering an early aggregation session. It carries no data —
	// the loop re-reads live store state — so a full channel is simply dropped.
	early chan struct{}
	// recovery carries imported blocks whose proofs may be worth recovering.
	recovery chan *types.SignedBlock

	// aggregatedSlot is the last slot for which an aggregation session was
	// dispatched, so the early (attestation-arrival) path and the interval-2
	// fallback dispatch at most once per slot. Written and read only on the
	// single dispatch loop, so no lock.
	aggregatedSlot uint64
	// numValidators caches the validator-set size, fixed at genesis in lean
	// devnet, used as the denominator for the early-aggregation quorum. Filled
	// lazily on the dispatch loop to avoid SSZ-decoding the head state on every
	// attestation arrival; read/written only there, so no lock.
	numValidators uint64
	// expectedVoters caches how many validators this node can hear from in a
	// slot. Its inputs are fixed once the registry is known, and it is read on
	// every attestation arrival.
	expectedVoters uint64
}

// New returns an Aggregator for cfg.
func New(cfg Config) *Aggregator {
	return &Aggregator{
		cfg:        cfg,
		dispatches: make(chan Dispatch, 1),
		early:      make(chan struct{}, 1),
		recovery:   make(chan *types.SignedBlock, 8),
	}
}

// Early delivers the early-aggregation wake-ups to the dispatch goroutine,
// which then calls MaybeEarly.
func (a *Aggregator) Early() <-chan struct{} { return a.early }

// PokeEarly asks the dispatch loop to consider aggregating early now that
// another vote is in. The signal is best-effort and coalescing; the loop owns
// the actual timing and quorum decision.
func (a *Aggregator) PokeEarly() {
	select {
	case a.early <- struct{}{}:
	default:
	}
}

// RunSessions proves the dispatched aggregation sessions until ctx ends.
func (a *Aggregator) RunSessions(ctx context.Context) {
	RunWorker(ctx, a.cfg.Clock, a.dispatches, a.cfg.Store, a.cfg.Store.PubKeyCache, a.cfg.Publisher, a.cfg.Gate, a.cfg.Shadow)
}

func (a *Aggregator) slotAt(timestampMs uint64) uint64 {
	if a == nil || a.cfg.Store == nil {
		return 0
	}
	return types.CurrentSlot(a.cfg.Store.Config().GenesisTime, timestampMs)
}

func (a *Aggregator) intervalAt(timestampMs uint64) uint64 {
	if a == nil || a.cfg.Store == nil {
		return 0
	}
	return types.CurrentInterval(a.cfg.Store.Config().GenesisTime, timestampMs)
}

func (a *Aggregator) msIntoSlot(timestampMs uint64) uint64 {
	if a == nil || a.cfg.Store == nil {
		return 0
	}
	return types.MillisIntoSlot(a.cfg.Store.Config().GenesisTime, timestampMs)
}
