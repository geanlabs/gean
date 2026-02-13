package node

import (
	"log/slog"

	"github.com/geanlabs/gean/chain/forkchoice"
	"github.com/geanlabs/gean/network"
	"github.com/geanlabs/gean/network/gossipsub"
)

const version = "v0.1.0"

// Node is the main gean node orchestrator.
type Node struct {
	FC        *forkchoice.Store
	Host      *network.Host
	Topics    *gossipsub.Topics
	Clock     *Clock
	Validator *ValidatorDuties
	log       *slog.Logger
}

// Config holds node configuration.
type Config struct {
	GenesisTime   uint64
	NumValidators uint64
	ListenAddr    string
	NodeKeyPath   string
	Bootnodes     []string
	ValidatorIDs  []uint64
	MetricsPort   int
	DevnetID      string
}
