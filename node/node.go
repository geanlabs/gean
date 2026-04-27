package node

import (
	"context"
	"time"
	"unsafe"

	"github.com/geanlabs/gean/forkchoice"
	"github.com/geanlabs/gean/logger"
	"github.com/geanlabs/gean/p2p"
	"github.com/geanlabs/gean/statetransition"
	"github.com/geanlabs/gean/types"
	"github.com/geanlabs/gean/xmss"
)

// Engine is the consensus coordination loop.
// It owns Store, ForkChoice, and KeyManager as siblings,
// rs L78-95).
// Pending block limits to prevent stuck-forever scenarios.
const (
	MaxBlockFetchDepth = 512  // Max ancestor chain depth before discarding
	MaxPendingBlocks   = 1024 // Max pending blocks before rejecting new ones
)

// Pending-attestation buffer caps. perRoot bounds the depth of any single
// head-root bucket; total bounds the sum across all buckets. Sized for
// devnet-4 expectations (low validator counts, short propagation windows);
// promote to flags if a future deployment needs different ceilings.
const (
	PendingAttestationsPerRootCap = 8
	PendingAttestationsTotalCap   = 512
)

type Engine struct {
	Store               *ConsensusStore
	FC                  *forkchoice.ForkChoice
	P2P                 *p2p.Host
	Keys                *xmss.KeyManager
	AggCtl              *AggregatorController
	CommitteeCount      uint64
	PendingBlocks       map[[32]byte]map[[32]byte]bool // parent_root -> {child_roots}
	PendingBlockParents map[[32]byte][32]byte          // block_root -> missing_ancestor
	PendingBlockDepths  map[[32]byte]int               // block_root -> fetch depth
	PendingAttestations *PendingAttestationBuffer      // gossip atts buffered by unknown head root

	// Channels for receiving messages from P2P goroutine.
	BlockCh       chan *types.SignedBlock
	AttestationCh chan *types.SignedAttestation
	AggregationCh chan *types.SignedAggregatedAttestation
	FailedRootCh  chan [32]byte // roots that exhausted all fetch retries — triggers subtree cleanup
	FetchRootCh   chan [32]byte // roots to fetch — coalesced into batches by the fetch batcher
}

// New creates a new Engine.
func New(
	s *ConsensusStore,
	fc *forkchoice.ForkChoice,
	p2pHost *p2p.Host,
	keys *xmss.KeyManager,
	aggCtl *AggregatorController,
	committeeCount uint64,
) *Engine {
	return &Engine{
		Store:               s,
		FC:                  fc,
		P2P:                 p2pHost,
		Keys:                keys,
		AggCtl:              aggCtl,
		CommitteeCount:      committeeCount,
		PendingBlocks:       make(map[[32]byte]map[[32]byte]bool),
		PendingBlockParents: make(map[[32]byte][32]byte),
		PendingBlockDepths:  make(map[[32]byte]int),
		PendingAttestations: NewPendingAttestationBuffer(PendingAttestationsPerRootCap, PendingAttestationsTotalCap),
		BlockCh:             make(chan *types.SignedBlock, 64),
		AttestationCh:       make(chan *types.SignedAttestation, 256),
		AggregationCh:       make(chan *types.SignedAggregatedAttestation, 64),
		FailedRootCh:        make(chan [32]byte, 64),
		FetchRootCh:         make(chan [32]byte, 256),
	}
}

// Run starts the engine's main loop.
// This is the single-writer goroutine — all state mutations happen here.
func (e *Engine) Run(ctx context.Context) {
	// Set up callbacks for gossip store (avoids circular deps).
	FreeSignatureFunc = func(ptr unsafe.Pointer) {
		xmss.FreeSignature(ptr)
	}
	AggregateMetricsFunc = func(durationSeconds float64, numAttestations int) {
		ObservePqSigAggBuildingTime(durationSeconds)
		IncPqSigAggregatedTotal()
		IncPqSigAttestationsInAggregated(numAttestations)
	}

	// Wire gossip-size hooks into the p2p layer.
	p2p.GossipBlockSizeHook = ObserveGossipBlockSize
	p2p.GossipAttestationSizeHook = ObserveGossipAttestationSize
	p2p.GossipAggregationSizeHook = ObserveGossipAggregationSize

	// Wire state-transition observability hooks.
	statetransition.ObserveTotalTimeHook = ObserveSTFTime
	statetransition.ObserveSlotsTimeHook = ObserveSTFSlotsTime
	statetransition.ObserveBlockTimeHook = ObserveSTFBlockTime
	statetransition.ObserveAttestationsTimeHook = ObserveSTFAttestationsTime
	statetransition.IncSlotsProcessedHook = IncSTFSlotsProcessed
	statetransition.IncAttestationsProcessedHook = IncSTFAttestationsProcessed

	// Wire peer event hooks. Client label is "unknown" until libp2p
	// identify-based client detection is added (follow-up); spec result
	// label is "success" for the accepted-connection path.
	p2p.PeerConnectedHook = func(direction string) {
		IncPeerConnection(direction, "success")
	}
	p2p.PeerDisconnectedHook = func(direction, reason string) {
		IncPeerDisconnection(direction, reason)
	}
	p2p.PeerCountHook = func(count int) {
		SetConnectedPeers("unknown", count)
	}

	// Initial sync status is "idle" until peers connect.
	SetSyncStatus("idle")

	// Initialize static metrics.
	// lean_is_aggregator is kept in sync via AggregatorController.Set on
	// every transition; NewAggregatorController already seeded it at boot.
	SetNodeInfo("gean", "dev")
	SetNodeStartTime(float64(time.Now().Unix()))
	SetAttestationCommitteeCount(e.CommitteeCount)
	if e.Keys != nil {
		vids := e.Keys.ValidatorIDs()
		SetValidatorsCount(len(vids))
		if len(vids) > 0 && e.CommitteeCount > 0 {
			SetAttestationCommitteeSubnet(vids[0] % e.CommitteeCount)
		}
	}

	ticker := time.NewTicker(types.MillisecondsPerInterval * time.Millisecond)
	defer ticker.Stop()

	// Start the fetch batcher: coalesces individual fetch requests into
	// batches of up to MaxBlocksPerRequest roots per peer request.
	go e.runFetchBatcher(ctx)

	logger.Info(logger.Node, "started")

	// Process gossip attestations concurrently — each gets its own goroutine
	// for XMSS verification (~500ms each). This matches zeam's inline model
	// where attestations are verified as they arrive, not queued.
	// AttestationSignatureMap is mutex-protected for safe concurrent writes.
	go func() {
		for att := range e.AttestationCh {
			att := att
			go e.onGossipAttestation(att)
		}
	}()

	for {
		select {
		case <-ctx.Done():
			logger.Info(logger.Node, "shutting down")
			return

		case <-ticker.C:
			e.onTick()

		case block := <-e.BlockCh:
			e.onBlock(block)

		case agg := <-e.AggregationCh:
			e.onGossipAggregatedAttestation(agg)

		case root := <-e.FailedRootCh:
			e.onFailedRoot(root)
		}
	}
}

// --- MessageHandler interface for P2P ---

func (e *Engine) OnBlock(block *types.SignedBlock) {
	select {
	case e.BlockCh <- block:
	default:
		logger.Warn(logger.Chain, "block channel full, dropping")
	}
}

func (e *Engine) OnGossipAttestation(att *types.SignedAttestation) {
	select {
	case e.AttestationCh <- att:
	default:
		logger.Warn(logger.Gossip, "attestation channel full, dropping")
	}
}

func (e *Engine) OnGossipAggregatedAttestation(agg *types.SignedAggregatedAttestation) {
	select {
	case e.AggregationCh <- agg:
	default:
		logger.Warn(logger.Signature, "aggregation channel full, dropping")
	}
}
