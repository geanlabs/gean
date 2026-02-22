package node

import (
	"context"
	"log/slog"

	"github.com/geanlabs/gean/chain/forkchoice"
	"github.com/geanlabs/gean/network"
	"github.com/geanlabs/gean/network/gossipsub"
	"github.com/geanlabs/gean/network/p2p"
	"github.com/geanlabs/gean/types"
)

var Version = "v0.1.0"

// Node is the main gean node orchestrator.
type Node struct {
	FC     *forkchoice.Store
	Host   *network.Host
	Topics *gossipsub.Topics
	// API       *api.Service // Temporary disable until found
	Validator *ValidatorDuties

	// P2P Services
	P2PManager   *p2p.LocalNodeManager
	P2PDiscovery *p2p.DiscoveryService

	Clock *Clock
	log   *slog.Logger

	ctx    context.Context
	cancel context.CancelFunc
}

func (n *Node) Close() {
	n.cancel()
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
	GenesisTime      uint64
	Validators       []*types.Validator
	ListenAddr       string
	NodeKeyPath      string
	Bootnodes        []string
	DiscoveryPort    int
	DataDir          string
	ValidatorIDs     []uint64
	ValidatorKeysDir string
	MetricsPort      int
	DevnetID         string
}
