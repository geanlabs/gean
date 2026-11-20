package node

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/geanlabs/gean/internal/lean/keys"

	"github.com/geanlabs/gean/internal/core/clock"
	"github.com/geanlabs/gean/internal/core/lifecycle"
	"github.com/geanlabs/gean/internal/core/logger"
	"github.com/geanlabs/gean/internal/core/pending"
	"github.com/geanlabs/gean/internal/lean/aggregation"
	"github.com/geanlabs/gean/internal/lean/dutygate"
	"github.com/geanlabs/gean/internal/lean/forkchoice"
	"github.com/geanlabs/gean/internal/lean/importer"
	"github.com/geanlabs/gean/internal/lean/p2p"
	"github.com/geanlabs/gean/internal/lean/proving"
	"github.com/geanlabs/gean/internal/lean/role"
	"github.com/geanlabs/gean/internal/lean/shadow"
	"github.com/geanlabs/gean/internal/lean/store"
	"github.com/geanlabs/gean/internal/lean/syncer"
	"github.com/geanlabs/gean/internal/lean/types"
)

var gitCommit = "unknown"

const (
	PendingAttestationsPerRootCap = 8
	PendingAttestationsTotalCap   = 512
)

type Engine struct {
	// Clock is the time source for every time-dependent decision: ticks, the
	// current slot, deadlines. Set by New.
	Clock clock.Clock

	Store          *store.ConsensusStore
	FC             *forkchoice.ForkChoice
	P2P            *p2p.Host
	Keys           *keys.Manager
	AggCtl         *role.Controller
	DutyGate       *dutygate.Gate
	CommitteeCount uint64
	// AggregateSubnetIDs are the attestation subnets this node subscribes to as
	// an aggregator. Empty means every subnet. Set by the caller after New.
	AggregateSubnetIDs  []uint64
	Shadow              shadow.Rates
	PendingAttestations *pending.AttestationBuffer[*types.SignedAttestation]

	// Importer imports received blocks, holds those whose parent is missing,
	// and fetches missing parents.
	Importer *importer.Importer

	// Aggregator schedules and proves aggregation sessions and recovers
	// proofs from imported blocks.
	Aggregator *aggregation.Aggregator

	AttestationCh chan *types.SignedAttestation
	AggregationCh chan *types.SignedAggregatedAttestation

	// ProvingGate is the prover token shared by proposals, aggregation and
	// proof recovery.
	ProvingGate *proving.Gate

	// proposals is the block-proposal duty's state; see proposer.
	proposals proposer

	// tasks tracks Run and every worker it starts, so shutdown can join them
	// before closing the database and freeing the keys they use: a worker
	// still running after Close calls into a closed Pebble instance, which
	// panics, and a proof in flight still borrows native key material.
	tasks lifecycle.Group

	// attestationsDropped counts gossip attestations dropped on a full verify
	// queue since the last chain-status line.
	attestationsDropped atomic.Uint64

	lastTick time.Time

	// lastTickMs mirrors lastTick for the stall sampler, which runs on its own
	// goroutine precisely so it still reports while the dispatch loop is blocked.
	lastTickMs atomic.Int64

	// lastIntervalStartMs is the start of the interval whose duties onTick
	// last ran; see claimInterval.
	lastIntervalStartMs uint64

	warnedMissingJustified [32]byte

	// maxSeenGossipSlot is the highest plausible slot heard on gossip, whether
	// or not the block was admitted. Written from the p2p goroutine, read on
	// the tick loop by the duty gate.
	maxSeenGossipSlot atomic.Uint64

	topicMeshSizes atomic.Pointer[map[string]int]

	// coveragePreMerge holds the new-payload participants captured before the
	// tick promoted them, keyed by the slot each vote is for. Read only on the
	// dispatch loop, which is also the only writer.
	coveragePreMerge map[uint64][][]byte
}

// New returns an engine. clk is the time source for every time-dependent
// decision; it is fixed for the engine's life, since the importer and
// aggregator are built with it.
func New(
	clk clock.Clock,
	s *store.ConsensusStore,
	fc *forkchoice.ForkChoice,
	p2pHost *p2p.Host,
	keyManager *keys.Manager,
	aggCtl *role.Controller,
	committeeCount uint64,
	shadowRates shadow.Rates,
) *Engine {
	p2p.SetClientGitCommit(gitCommit)
	e := &Engine{
		Clock:               clk,
		Store:               s,
		FC:                  fc,
		P2P:                 p2pHost,
		Keys:                keyManager,
		AggCtl:              aggCtl,
		DutyGate:            dutygate.New(logDutyGateEvent),
		CommitteeCount:      committeeCount,
		Shadow:              shadowRates,
		PendingAttestations: pending.NewAttestationBuffer(PendingAttestationsPerRootCap, PendingAttestationsTotalCap, types.AttestationSlot),
		AttestationCh:       make(chan *types.SignedAttestation, 256),
		AggregationCh:       make(chan *types.SignedAggregatedAttestation, 64),
		proposals:           newProposer(),
		ProvingGate:         proving.NewGate(),
	}
	e.Aggregator = e.newAggregator()
	e.Importer = e.newImporter()
	e.configureP2PHooks()
	return e
}

// newAggregator builds the aggregator. Its functions read e's fields when
// called, so changes made after New (such as AggregateSubnetIDs) are seen.
func (e *Engine) newAggregator() *aggregation.Aggregator {
	cfg := aggregation.Config{
		Store:        e.Store,
		Clock:        e.Clock,
		Gate:         e.ProvingGate,
		Shadow:       e.Shadow,
		IsAggregator: func() bool { return e.AggCtl != nil && e.AggCtl.Get() },
		ProposingAt:  e.proposingAt,
		ProposerAt: func(slot uint64) bool {
			_, ok := e.getOurProposer(slot)
			return ok
		},
		Synced:  func() bool { return e.GetSyncStatus() == syncer.SyncSynced },
		Subnets: func() (uint64, []uint64) { return e.CommitteeCount, e.AggregateSubnetIDs },
	}
	// A nil *p2p.Host in the interface would not compare equal to nil, so only
	// set the publisher when there is a host.
	if e.P2P != nil {
		cfg.Publisher = e.P2P
	}
	return aggregation.New(cfg)
}

// newImporter builds the block importer for e's store and fork choice.
func (e *Engine) newImporter() *importer.Importer {
	cfg := importer.Config{
		Store:               e.Store,
		FC:                  e.FC,
		PendingAttestations: e.PendingAttestations,
		Hooks: importer.Hooks{
			Imported:    e.Aggregator.DispatchRecovery,
			UpdateHead:  e.updateHead,
			Attestation: e.OnGossipAttestation,
		},
	}
	// A nil *p2p.Host in the interface would not compare equal to nil, so only
	// set the fetcher when there is a host.
	if e.P2P != nil {
		cfg.Fetcher = e.P2P
	}
	return importer.New(cfg)
}

// Start runs the engine on a tracked goroutine.
func (e *Engine) Start(ctx context.Context) {
	e.tasks.Go(func() { e.Run(ctx) })
}

// Wait blocks until Run and every worker have returned, including any proof in
// flight. Callers cancel the context first, and close the database and keys
// only after Wait returns.
func (e *Engine) Wait() {
	e.tasks.Close()
}

func (e *Engine) Run(ctx context.Context) {
	e.initMetrics()

	e.startWorkers(ctx)

	logger.Info(logger.Node, "started")
	e.onTick()
	e.dispatch(ctx, alignedTicks(ctx, e.Clock, e.Store.Config().GenesisTime))
}
