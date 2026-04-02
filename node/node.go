package node

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"time"

	apiserver "github.com/geanlabs/gean/api/server"
	"github.com/geanlabs/gean/chain/forkchoice"
	"github.com/geanlabs/gean/network"
	"github.com/geanlabs/gean/network/gossipsub"
	"github.com/geanlabs/gean/network/p2p"
	"github.com/geanlabs/gean/types"
	"github.com/libp2p/go-libp2p/core/peer"
)

var Version = "v0.1.0"

// Node is the main gean node orchestrator.
type Node struct {
	FC        *forkchoice.Store
	Host      *network.Host
	Topics    *gossipsub.Topics
	API       *apiserver.Server
	Validator *ValidatorDuties

	// P2P Services
	P2PManager   *p2p.LocalNodeManager
	P2PDiscovery *p2p.DiscoveryService

	// PendingBlocks caches blocks awaiting parent availability.
	PendingBlocks *PendingBlockCache

	// Sync deduplication: tracks roots currently being fetched to avoid
	// duplicate requests across peers and recovery attempts.
	// Matches leanSpec BackfillSync._pending pattern.
	pendingRoots   map[[32]byte]struct{}
	pendingRootsMu sync.Mutex

	// Per-peer backoff tracking for sync requests.
	// Tracks consecutive failures and last attempt time per peer.
	peerBackoff   map[peer.ID]*peerSyncState
	peerBackoffMu sync.Mutex

	// Recovery cooldown prevents recoverMissingParentSync from flooding
	// peers when multiple gossip blocks arrive with missing parents.
	recoveryMu       sync.Mutex
	lastRecoveryTime time.Time

	Clock    *Clock
	dbCloser io.Closer
	log      *slog.Logger

	ctx    context.Context
	cancel context.CancelFunc
}

// peerSyncState tracks backoff state for a single peer during sync.
// Modeled after ethlambda's exponential backoff pattern.
type peerSyncState struct {
	failures  int
	lastTried time.Time
}

// Sync constants aligned with leanSpec (subspecs/sync/config.py).
const (
	// maxBlocksPerRequest is the maximum number of block roots to request
	// in a single BlocksByRoot RPC call. Matches leanSpec MAX_BLOCKS_PER_REQUEST.
	maxBlocksPerRequest = 10

	// maxBackfillDepth is the maximum depth for backward chain walks.
	// Matches leanSpec MAX_BACKFILL_DEPTH and zeam MAX_BLOCK_FETCH_DEPTH.
	maxBackfillDepth = 512

	// recoveryCooldown prevents rapid-fire recovery attempts when multiple
	// gossip blocks arrive with missing parents in quick succession.
	recoveryCooldown = 2 * time.Second

	// maxSyncRetries is the maximum number of retry attempts per peer
	// before giving up. Matches ethlambda MAX_FETCH_RETRIES.
	maxSyncRetries = 10

	// initialBackoff is the starting backoff duration for failed sync requests.
	initialBackoff = 5 * time.Millisecond

	// backoffMultiplier doubles the backoff on each consecutive failure.
	backoffMultiplier = 2
)

func (n *Node) Close() {
	n.cancel()
	if n.API != nil {
		n.API.Stop()
	}
	if n.dbCloser != nil {
		n.dbCloser.Close()
	}
	if n.P2PDiscovery != nil {
		n.P2PDiscovery.Close()
	}
	if n.P2PManager != nil {
		n.P2PManager.Close()
	}
	if n.Host != nil {
		n.Host.Close()
	}
}

// Config holds node configuration.
type Config struct {
	GenesisTime       uint64
	Validators        []*types.Validator
	ListenAddr        string
	NodeKeyPath       string
	Bootnodes         []string
	DiscoveryPort     int
	DataDir           string
	CheckpointSyncURL string
	ValidatorIDs      []uint64
	ValidatorKeysDir  string
	MetricsPort       int
	DevnetID          string
	IsAggregator      bool
	APIHost           string
	APIPort           int
	APIEnabled        bool
}
