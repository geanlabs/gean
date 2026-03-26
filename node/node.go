package node

import (
	"context"
	"io"
	"log/slog"

	apiserver "github.com/geanlabs/gean/api/server"
	"github.com/geanlabs/gean/chain/forkchoice"
	"github.com/geanlabs/gean/network"
	"github.com/geanlabs/gean/network/gossipsub"
	"github.com/geanlabs/gean/network/p2p"
	"github.com/geanlabs/gean/types"
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

	Clock    *Clock
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
	GenesisTime               uint64
	Validators                []*types.Validator
	ListenAddr                string
	NodeKeyPath               string
	Bootnodes                 []string
	DiscoveryPort             int
	DataDir                   string
	CheckpointSyncURL         string
	ValidatorIDs              []uint64
	ValidatorKeysDir          string
	MetricsPort               int
	DevnetID                  string
	IsAggregator              bool
	ImportSubnetIDs           []uint64
	AttestationCommitteeCount uint64
	APIHost                   string
	APIPort                   int
	APIEnabled                bool
}
