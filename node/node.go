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
)

var Version = "v0.1.0"

// Queue sizes for async gossip message dispatch. Decouples gossip readers
// from fork-choice processing to prevent libp2p send queue backpressure.
const (
	blockQueueSize       = 64
	attestationQueueSize = 256
	aggregationQueueSize = 128
)

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

	// Async gossip message queues. Gossip reader goroutines enqueue here
	// instead of processing synchronously, preventing slow fork-choice
	// operations from blocking message consumption and causing peer
	// send queue backpressure.
	blockCh       chan *types.SignedBlockWithAttestation
	attestationCh chan *types.SignedAttestation
	aggregationCh chan *types.SignedAggregatedAttestation

	// recoveryCooldown prevents recoverMissingParentSync from firing
	// for every gossip block with a missing parent, which causes
	// excessive blocks_by_root request flooding to peers.
	recoveryMu       sync.Mutex
	lastRecoveryTime time.Time

	Clock *Clock
	dbCloser io.Closer
	log      *slog.Logger

	ctx    context.Context
	cancel context.CancelFunc
}

func (n *Node) Close() {
	n.cancel()
	if n.API != nil {
		n.API.Stop()
	}
	// Free Rust-allocated XMSS keypairs.
	if n.Validator != nil {
		for _, key := range n.Validator.Keys {
			if f, ok := key.(interface{ Free() }); ok {
				f.Free()
			}
		}
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
