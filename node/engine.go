package node

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/geanlabs/gean/aggregation"
	"github.com/geanlabs/gean/crypto"
	"github.com/geanlabs/gean/dutygate"
	"github.com/geanlabs/gean/forkchoice"
	"github.com/geanlabs/gean/logger"
	"github.com/geanlabs/gean/pending"
	"github.com/geanlabs/gean/proving"
	"github.com/geanlabs/gean/role"
	"github.com/geanlabs/gean/shadow"
	"github.com/geanlabs/gean/store"
	"github.com/geanlabs/gean/tasks"
	"github.com/geanlabs/gean/types"
)

const (
	MaxBlockFetchDepth = 512
	MaxPendingBlocks   = 1024
)

const (
	PendingAttestationsPerRootCap = 8
	PendingAttestationsTotalCap   = 512
)

// Network is what the engine needs from the peer-to-peer layer: publishing its
// own messages, fetching blocks it is missing, and peer counts for sync status
// and metrics. Inbound gossip reaches the engine through its On* methods.
// p2p.Host is the libp2p implementation; sim.Network delivers in-process.
type Network interface {
	PublishBlock(ctx context.Context, block *types.SignedBlock) error
	PublishAttestation(ctx context.Context, att *types.SignedAttestation, committeeCount uint64) error
	PublishAggregatedAttestation(ctx context.Context, agg *types.SignedAggregatedAttestation) error
	// FetchBlocksByRootBatchWithRetry returns the blocks it found and the roots
	// it could not, for at most types.MaxBlocksPerRootFetch roots.
	FetchBlocksByRootBatchWithRetry(ctx context.Context, roots [][32]byte) ([]*types.SignedBlock, [][32]byte, error)
	ConnectedPeers() int
	MeshPeerCount() int
	TopicMeshSizes() map[string]int
}

// Components are the parts an Engine is assembled from. The caller builds and
// owns each one; the engine only uses them.
type Components struct {
	Store      *store.ConsensusStore
	ForkChoice *forkchoice.ForkChoice
	// Network is nil for an engine that never publishes or fetches.
	Network Network
	// Keys signs this node's duties; nil runs the engine without validators.
	Keys crypto.Signer
	// Crypto verifies and proves signatures.
	Crypto     crypto.Scheme
	Aggregator *role.Controller
	Clock      Clock
}

// Config holds the engine's network parameters.
type Config struct {
	CommitteeCount uint64
	// AggregateSubnetIDs are the attestation subnets this node subscribes to as
	// an aggregator. Empty means every subnet.
	AggregateSubnetIDs []uint64
	Shadow             shadow.Rates
}

type Engine struct {
	store              *store.ConsensusStore
	forkChoice         *forkchoice.ForkChoice
	network            Network
	keys               crypto.Signer
	scheme             crypto.Scheme
	aggregator         *role.Controller
	dutyGate           *dutygate.Gate
	committeeCount     uint64
	aggregateSubnetIDs []uint64
	// expectedVoters caches how many validators this node can hear from in a
	// slot. Its inputs are fixed once the registry is known, and it is read on
	// every attestation arrival.
	expectedVoters      uint64
	shadowRates         shadow.Rates
	pendingBlocks       *pending.BlockBuffer
	pendingAttestations *pending.AttestationBuffer

	blockCh       chan *types.SignedBlock
	attestationCh chan *types.SignedAttestation
	aggregationCh chan *types.SignedAggregatedAttestation
	failedRootCh  chan [32]byte
	fetchRootCh   chan [32]byte

	// earlyAggregateCh is a coalescing (capacity-1) wake-up: an attestation-verify
	// goroutine pokes it after inserting a signature, and the dispatch loop reacts
	// by considering an early aggregation session. It carries no data — the loop
	// re-reads live store state — so a full channel is simply dropped.
	earlyAggregateCh chan struct{}

	aggregationDispatchCh chan aggregation.Dispatch
	proposalCh            chan proposalDuty
	proposalResultCh      chan *proposalResult
	recoveryCh            chan *types.SignedBlock
	provingGate           *proving.Gate
	aggregationWorker     *aggregation.Worker
	clock                 Clock

	// Dispatch-owned reservation survives dequeue and result acceptance.
	lastProposalDuty proposalDuty
	proposalReserved bool

	// workers owns every goroutine the engine starts: the long-running workers
	// and the per-message verification goroutines. Run waits for all of them
	// before returning, so the caller may release storage and keys afterwards.
	workers tasks.Group

	lastTick time.Time

	// lastTickMs mirrors lastTick for the stall sampler, which runs on its own
	// goroutine precisely so it still reports while the dispatch loop is blocked.
	lastTickMs atomic.Int64

	warnedMissingJustified [32]byte

	// forkChoiceView is the latest consistent picture of fork choice for
	// readers off the dispatch loop; see ForkChoiceView.
	forkChoiceView atomic.Pointer[forkchoice.View]

	// maxSeenGossipSlot is the highest plausible slot heard on gossip, whether
	// or not the block was admitted. Written from the p2p goroutine, read on
	// the tick loop by the duty gate.
	maxSeenGossipSlot atomic.Uint64

	// fetchInFlight tracks block roots already queued for by-root fetch so a single
	// missing parent cannot flood fetchRootCh with duplicate requests. Accessed only
	// on the dispatch loop (queue on onBlock, clear on receive/exhaustion), so no lock.
	fetchInFlight  map[[32]byte]bool
	topicMeshSizes atomic.Pointer[map[string]int]

	// coveragePreMerge holds the new-payload participants captured before the
	// tick promoted them, keyed by the slot each vote is for. Read only on the
	// dispatch loop, which is also the only writer.
	coveragePreMerge map[uint64][][]byte

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
}

// New assembles an engine from its components.
func New(c Components, cfg Config) *Engine {
	e := &Engine{
		store:               c.Store,
		forkChoice:          c.ForkChoice,
		network:             c.Network,
		keys:                c.Keys,
		scheme:              c.Crypto,
		aggregator:          c.Aggregator,
		dutyGate:            dutygate.New(logDutyGateEvent),
		committeeCount:      cfg.CommitteeCount,
		aggregateSubnetIDs:  cfg.AggregateSubnetIDs,
		shadowRates:         cfg.Shadow,
		pendingBlocks:       pending.NewBlockBuffer(),
		pendingAttestations: pending.NewAttestationBuffer(PendingAttestationsPerRootCap, PendingAttestationsTotalCap),
		// Sized so gossip keeps flowing while the dispatch loop chews through a
		// fetched batch: sync delivery blocks when full, but gossip drops, and a
		// large import burst can take minutes of XMSS verification.
		blockCh:               make(chan *types.SignedBlock, 256),
		attestationCh:         make(chan *types.SignedAttestation, 256),
		aggregationCh:         make(chan *types.SignedAggregatedAttestation, 64),
		failedRootCh:          make(chan [32]byte, 64),
		fetchRootCh:           make(chan [32]byte, 256),
		earlyAggregateCh:      make(chan struct{}, 1),
		aggregationDispatchCh: make(chan aggregation.Dispatch, 1),
		proposalCh:            make(chan proposalDuty, 1),
		proposalResultCh:      make(chan *proposalResult, 1),
		recoveryCh:            make(chan *types.SignedBlock, 8),
		provingGate:           proving.NewGate(),
		fetchInFlight:         make(map[[32]byte]bool),
		clock:                 c.Clock,
	}
	e.publishForkChoiceView()
	e.aggregationWorker = aggregation.NewWorker(e.store, e.scheme, e.network, e.provingGate, e.shadowRates, e.clock.Now)
	return e
}

// Run drives the engine until ctx is cancelled. It returns only after every
// goroutine the engine started has finished, including in-flight proving and
// verification, so the caller can then close storage and release keys.
func (e *Engine) Run(ctx context.Context) {
	e.initMetrics()

	ticker := time.NewTicker(types.MillisecondsPerInterval * time.Millisecond)
	defer ticker.Stop()

	e.startWorkers(ctx)

	logger.Info(logger.Node, "started")
	e.onTick(ctx)
	e.dispatch(ctx, ticker.C)
	e.workers.Wait()
}
