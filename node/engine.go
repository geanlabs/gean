package node

import (
	"context"
	"time"
	"unsafe"

	"github.com/geanlabs/gean/xmss"
	"github.com/geanlabs/gean/forkchoice"
	"github.com/geanlabs/gean/logger"
	"github.com/geanlabs/gean/p2p"
	"github.com/geanlabs/gean/types"
)

// Engine is the consensus coordination loop.
// It owns Store, ForkChoice, and KeyManager as siblings,
// matching ethlambda's BlockChainServer (lib.rs L78-95).
type Engine struct {
	Store               *ConsensusStore
	FC                  *forkchoice.ForkChoice
	P2P                 *p2p.Host
	Keys                *xmss.KeyManager
	IsAggregator        bool
	CommitteeCount      uint64
	PendingBlocks       map[[32]byte]map[[32]byte]bool // parent_root -> {child_roots}
	PendingBlockParents map[[32]byte][32]byte           // block_root -> missing_ancestor

	// Channels for receiving messages from P2P goroutine.
	BlockCh       chan *types.SignedBlockWithAttestation
	AttestationCh chan *types.SignedAttestation
	AggregationCh chan *types.SignedAggregatedAttestation
}

// New creates a new Engine.
func New(
	s *ConsensusStore,
	fc *forkchoice.ForkChoice,
	p2pHost *p2p.Host,
	keys *xmss.KeyManager,
	isAggregator bool,
	committeeCount uint64,
) *Engine {
	return &Engine{
		Store:               s,
		FC:                  fc,
		P2P:                 p2pHost,
		Keys:                keys,
		IsAggregator:        isAggregator,
		CommitteeCount:      committeeCount,
		PendingBlocks:       make(map[[32]byte]map[[32]byte]bool),
		PendingBlockParents: make(map[[32]byte][32]byte),
		BlockCh:             make(chan *types.SignedBlockWithAttestation, 64),
		AttestationCh:       make(chan *types.SignedAttestation, 256),
		AggregationCh:       make(chan *types.SignedAggregatedAttestation, 64),
	}
}

// Run starts the engine's main loop.
// This is the single-writer goroutine — all state mutations happen here.
// Matches ethlambda BlockChainServer's message handling loop.
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

	// Initialize static metrics.
	SetNodeInfo("geany", "dev")
	SetNodeStartTime(float64(time.Now().Unix()))
	SetIsAggregator(e.IsAggregator)
	SetAttestationCommitteeCount(e.CommitteeCount)
	if e.Keys != nil {
		SetValidatorsCount(len(e.Keys.ValidatorIDs()))
	}

	ticker := time.NewTicker(types.MillisecondsPerInterval * time.Millisecond)
	defer ticker.Stop()

	logger.Info(logger.Node, "started")

	for {
		select {
		case <-ctx.Done():
			logger.Info(logger.Node, "shutting down")
			return

		case <-ticker.C:
			e.onTick()

		case block := <-e.BlockCh:
			e.onBlock(block)

		case att := <-e.AttestationCh:
			e.onGossipAttestation(att)

		case agg := <-e.AggregationCh:
			e.onGossipAggregatedAttestation(agg)
		}
	}
}

// --- MessageHandler interface for P2P ---

func (e *Engine) OnBlock(block *types.SignedBlockWithAttestation) {
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
