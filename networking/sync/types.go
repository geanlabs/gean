// types.go contains shared sync types, constants, and constructor wiring.
//
// Package sync implements the chain synchronization protocol for the Lean consensus client.
//
// When a node discovers a peer with a higher head slot (via the Status handshake),
// it requests missing blocks via the BlocksByRoot req/resp protocol and processes
// them in parent-first order. Missing parents are fetched recursively.
//
// Sync requests use exponential backoff retry (1s, 2s, 4s, max 3 retries) to
// handle transient stream failures gracefully.
package sync

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/devylongs/gean/networking/reqresp"
	"github.com/devylongs/gean/types"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
)

// ChainStore provides access to the block store for chain synchronization.
// Satisfied by forkchoice.Store without modification.
type ChainStore interface {
	HasBlock(root types.Root) bool
	ProcessBlock(block *types.Block) error
	AdvanceTime(unixTime uint64, hasProposal bool)
}

const (
	reqrespTimeout = 30 * time.Second
	maxSyncRetries = 3
	baseRetryDelay = 1 * time.Second
)

type SyncState int

const (
	SyncStateIdle SyncState = iota
	SyncStateSyncing
)

type Syncer struct {
	host           host.Host
	store          ChainStore
	streamHandler  *reqresp.StreamHandler
	reqrespHandler *reqresp.Handler
	logger         *slog.Logger

	mu         sync.RWMutex
	peerStatus map[peer.ID]*reqresp.Status
	state      SyncState

	pendingParents   map[types.Root]struct{}
	pendingParentsMu sync.Mutex

	ctx    context.Context
	cancel context.CancelFunc
}

// Config holds syncer configuration.
type Config struct {
	Host           host.Host
	Store          ChainStore
	StreamHandler  *reqresp.StreamHandler
	ReqRespHandler *reqresp.Handler
	Logger         *slog.Logger
}

// NewSyncer creates a new syncer.
func NewSyncer(ctx context.Context, cfg Config) *Syncer {
	ctx, cancel := context.WithCancel(ctx)

	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return &Syncer{
		host:           cfg.Host,
		store:          cfg.Store,
		streamHandler:  cfg.StreamHandler,
		reqrespHandler: cfg.ReqRespHandler,
		logger:         logger,
		peerStatus:     make(map[peer.ID]*reqresp.Status),
		pendingParents: make(map[types.Root]struct{}),
		state:          SyncStateIdle,
		ctx:            ctx,
		cancel:         cancel,
	}
}
