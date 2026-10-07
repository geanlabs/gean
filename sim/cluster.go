// Package sim runs gean nodes in one process, on a simulated network and a
// manual clock. Every node is assembled from the same library components the
// gean binary uses; only the network, the clock and, by default, the
// signature scheme are replaced. Time moves only when the simulation advances
// it, and all work runs on the caller's goroutine in a fixed order, so a
// scenario produces the same chain every run.
package sim

import (
	"context"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/geanlabs/gean/crypto"
	"github.com/geanlabs/gean/db"
	"github.com/geanlabs/gean/genesis"
	"github.com/geanlabs/gean/metrics"
	"github.com/geanlabs/gean/node"
	"github.com/geanlabs/gean/role"
	"github.com/geanlabs/gean/store"
	"github.com/geanlabs/gean/types"
	"github.com/prometheus/client_golang/prometheus"
)

// Crypto is the signature scheme and validator keys a cluster runs on.
type Crypto interface {
	Scheme() crypto.Scheme
	// PublicKeys returns validator i's attestation and proposal keys.
	PublicKeys(i uint64) (attestation, proposal crypto.PublicKey)
	// Signer signs as the given validators. It stays owned by the Crypto.
	Signer(validatorIDs []uint64) crypto.Signer
	// Close releases the scheme and keys once every node has stopped.
	Close()
}

// Config describes a simulated network.
type Config struct {
	// GenesisTime is the genesis time in seconds; the clock starts there.
	GenesisTime uint64
	// Validators is the validator count.
	Validators int
	// Nodes lists, for each node, the validators whose keys it holds.
	Nodes [][]uint64
	// Aggregators lists the nodes that aggregate attestations.
	Aggregators []int
	// Crypto is the scheme the nodes sign and verify with. Nil uses Insecure,
	// which is fast and needs no native code; sim/xmsssim provides XMSS.
	Crypto Crypto
}

// Node is one simulated node.
type Node struct {
	Engine *node.Engine
	Store  *store.ConsensusStore
	// Aggregator is the node's aggregator role; setting it switches
	// aggregation on or off at runtime, as the admin API does.
	Aggregator *role.Controller
	// Metrics is the node's own metrics registry.
	Metrics *prometheus.Registry
}

// Cluster is a set of nodes sharing a simulated network and clock.
type Cluster struct {
	clock  Clock
	crypto Crypto
	nodes  []*Node
}

// New builds every node from one genesis and runs their first tick at the
// genesis time. It takes ownership of cfg.Crypto: Close releases it, and New
// releases it itself if it fails.
func New(ctx context.Context, cfg Config) (*Cluster, error) {
	cryptoProvider := cfg.Crypto
	if cryptoProvider == nil {
		cryptoProvider = Insecure()
	}
	if err := cfg.validate(); err != nil {
		cryptoProvider.Close()
		return nil, err
	}
	entries := make([]genesis.GenesisValidatorEntry, cfg.Validators)
	for i := range entries {
		attestation, proposal := cryptoProvider.PublicKeys(uint64(i))
		entries[i] = genesis.GenesisValidatorEntry{
			AttestationPubkey: hex.EncodeToString(attestation[:]),
			ProposalPubkey:    hex.EncodeToString(proposal[:]),
		}
	}
	aggregators := make(map[int]bool, len(cfg.Aggregators))
	for _, i := range cfg.Aggregators {
		aggregators[i] = true
	}

	c := &Cluster{clock: Clock{now: time.Unix(int64(cfg.GenesisTime), 0)}, crypto: cryptoProvider}
	for i, validators := range cfg.Nodes {
		n, err := c.newNode(i, cfg, entries, validators, aggregators[i])
		if err != nil {
			c.Close()
			return nil, fmt.Errorf("node %d: %w", i, err)
		}
		c.nodes = append(c.nodes, n)
	}

	for _, n := range c.nodes {
		n.Engine.Tick(ctx)
	}
	c.settle(ctx)
	return c, nil
}

func (cfg Config) validate() error {
	if len(cfg.Nodes) == 0 {
		return fmt.Errorf("simulation needs at least one node")
	}
	held := make(map[uint64]bool)
	for i, validators := range cfg.Nodes {
		for _, id := range validators {
			if id >= uint64(cfg.Validators) || held[id] {
				return fmt.Errorf("node %d: validator %d is out of range or held by another node", i, id)
			}
			held[id] = true
		}
	}
	for _, i := range cfg.Aggregators {
		if i < 0 || i >= len(cfg.Nodes) {
			return fmt.Errorf("aggregator %d is not a node", i)
		}
	}
	return nil
}

func (c *Cluster) newNode(index int, cfg Config, entries []genesis.GenesisValidatorEntry, validators []uint64, aggregator bool) (*Node, error) {
	gc := &genesis.GenesisConfig{GenesisTime: cfg.GenesisTime, GenesisValidators: entries}
	genesisState, err := gc.GenesisState()
	if err != nil {
		return nil, err
	}
	s := store.NewConsensusStore(db.NewInMemoryBackend())
	if _, err := s.InitFromGenesis(genesisState); err != nil {
		return nil, err
	}
	if err := s.RecoverTime(cfg.GenesisTime, c.clock.Now()); err != nil {
		return nil, err
	}
	if err := s.SeedMaxStoredBlockSlot(); err != nil {
		return nil, err
	}
	fc, err := node.ForkChoiceFromStore(s)
	if err != nil {
		return nil, err
	}

	n := &Node{Store: s, Aggregator: role.New(aggregator), Metrics: prometheus.NewRegistry()}
	components := node.Components{
		Store:      s,
		ForkChoice: fc,
		Network:    &network{cluster: c, self: index},
		Crypto:     c.crypto.Scheme(),
		Aggregator: n.Aggregator,
		Clock:      &c.clock,
		Metrics:    metrics.New(n.Metrics),
	}
	if len(validators) > 0 {
		components.Keys = c.crypto.Signer(validators)
	}
	n.Engine = node.New(components, node.Config{CommitteeCount: types.AttestationCommitteeCount})
	return n, nil
}

// Nodes returns the nodes in index order.
func (c *Cluster) Nodes() []*Node { return c.nodes }

// Slot is the clock's current slot.
func (c *Cluster) Slot() uint64 {
	return types.CurrentSlot(c.nodes[0].Store.Config().GenesisTime, uint64(c.clock.Now().UnixMilli()))
}

// AdvanceInterval moves the clock one interval, ticks every node in order,
// then lets the network settle.
func (c *Cluster) AdvanceInterval(ctx context.Context) {
	c.clock.advance(types.MillisecondsPerInterval * time.Millisecond)
	for _, n := range c.nodes {
		n.Engine.Tick(ctx)
	}
	c.settle(ctx)
}

// AdvanceSlots advances the clock by whole slots, interval by interval.
func (c *Cluster) AdvanceSlots(ctx context.Context, slots int) {
	for range slots * types.IntervalsPerSlot {
		c.AdvanceInterval(ctx)
	}
}

// settle runs every node's queued work, in node order, until a full pass
// finds none: one node's output is the next node's input, so a single pass is
// not enough.
func (c *Cluster) settle(ctx context.Context) {
	for {
		handled := false
		for _, n := range c.nodes {
			if n.Engine.ProcessPending(ctx) {
				handled = true
			}
		}
		if !handled {
			return
		}
	}
}

// Close releases the cluster's scheme and keys.
func (c *Cluster) Close() {
	c.crypto.Close()
}
