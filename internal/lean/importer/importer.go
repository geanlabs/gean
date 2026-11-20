package importer

import (
	"context"

	"github.com/geanlabs/gean/internal/core/logger"
	"github.com/geanlabs/gean/internal/lean/forkchoice"
	"github.com/geanlabs/gean/internal/lean/pending"
	"github.com/geanlabs/gean/internal/lean/store"
	"github.com/geanlabs/gean/internal/lean/types"
)

const (
	// MaxBlockFetchDepth bounds how far behind a missing parent the importer
	// chases: deeper gaps are range sync's to close.
	MaxBlockFetchDepth = 512
	// MaxPendingBlocks bounds the blocks held while their parent is missing.
	MaxPendingBlocks = 1024
)

// Fetcher requests blocks by root from peers.
type Fetcher interface {
	FetchBlocksByRootBatchWithRetry(ctx context.Context, roots [][32]byte) ([]*types.SignedBlock, [][32]byte, error)
}

// Hooks are the node's reactions to an import. Each may be nil.
type Hooks struct {
	// Imported runs after a block's state transition and persistence
	// succeeded, before fork choice learns the block.
	Imported func(*types.SignedBlock)
	// UpdateHead recomputes the head after fork choice learns a block.
	UpdateHead func()
	// Attestation re-submits a buffered attestation whose head block has just
	// been imported.
	Attestation func(*types.SignedAttestation)
}

// Config is what an Importer needs.
type Config struct {
	Store *store.ConsensusStore
	FC    *forkchoice.ForkChoice
	// Fetcher fetches missing parents. Nil disables fetching.
	Fetcher Fetcher
	// PendingAttestations holds attestations whose head block is not imported
	// yet; the importer replays them when it arrives and prunes them at
	// finality.
	PendingAttestations *pending.AttestationBuffer
	Hooks               Hooks
}

// Importer turns received blocks into imported ones: it imports blocks whose
// parent is known, holds the rest until their parent arrives, and fetches
// missing parents from peers.
//
// Import, Drain, OnFailedRoot and DiscardFinalized run on the node's single
// dispatch goroutine, which owns the importer's state. SubmitGossip, SubmitSync
// and RequestFetch are safe from any goroutine; RunFetcher runs on its own.
type Importer struct {
	store               *store.ConsensusStore
	fc                  *forkchoice.ForkChoice
	fetcher             Fetcher
	pendingAttestations *pending.AttestationBuffer
	hooks               Hooks

	// Pending holds blocks whose parent is missing.
	Pending *pending.BlockBuffer

	blocks      chan *types.SignedBlock
	failedRoots chan [32]byte
	fetchRoots  chan [32]byte

	// fetchInFlight tracks block roots already queued for by-root fetch so a single
	// missing parent cannot flood fetchRoots with duplicate requests. Accessed only
	// on the dispatch goroutine (queued on import, cleared on receive/exhaustion),
	// so no lock.
	fetchInFlight map[[32]byte]bool

	// importAttempts counts blocks handed to Import. Dispatch goroutine only.
	importAttempts uint64
}

// New returns an Importer for cfg.
func New(cfg Config) *Importer {
	return &Importer{
		store:               cfg.Store,
		fc:                  cfg.FC,
		fetcher:             cfg.Fetcher,
		pendingAttestations: cfg.PendingAttestations,
		hooks:               cfg.Hooks,
		Pending:             pending.NewBlockBuffer(),
		// Sized so gossip keeps flowing while the dispatch loop chews through a
		// fetched batch: sync delivery blocks when full, but gossip drops, and a
		// large import burst can take minutes of XMSS verification.
		blocks:        make(chan *types.SignedBlock, 256),
		failedRoots:   make(chan [32]byte, 64),
		fetchRoots:    make(chan [32]byte, 256),
		fetchInFlight: make(map[[32]byte]bool),
	}
}

// Blocks delivers received blocks to the dispatch goroutine, which passes each
// to Import.
func (i *Importer) Blocks() <-chan *types.SignedBlock { return i.blocks }

// FailedRoots delivers roots whose fetch was exhausted to the dispatch
// goroutine, which passes each to OnFailedRoot.
func (i *Importer) FailedRoots() <-chan [32]byte { return i.failedRoots }

// SubmitGossip queues a block received on gossip, dropping it if the queue is
// full.
func (i *Importer) SubmitGossip(block *types.SignedBlock) {
	select {
	case i.blocks <- block:
	default:
		logger.Warn(logger.Chain, "block channel full, dropping")
	}
}

// SubmitSync delivers a block this node itself requested (range backfill or
// by-root parent fetch). Unlike gossip delivery it blocks until the dispatch
// loop accepts the block: a requested block dropped on the floor is a gap the
// requester believes it already covered, so it is never re-requested and the
// chain can no longer connect. Returns false only if ctx ends first.
func (i *Importer) SubmitSync(ctx context.Context, block *types.SignedBlock) bool {
	select {
	case i.blocks <- block:
		return true
	case <-ctx.Done():
		return false
	}
}

// RequestFetch asks the fetcher for root without the in-flight bookkeeping the
// dispatch goroutine does, so any goroutine may call it. A full queue drops the
// request.
func (i *Importer) RequestFetch(root [32]byte) {
	select {
	case i.fetchRoots <- root:
	default:
	}
}

// ImportAttempts reports how many blocks have been handed to Import.
func (i *Importer) ImportAttempts() uint64 { return i.importAttempts }
