// Package sim runs gean nodes in one process, on a simulated network and a
// manual clock. Every node is assembled from the same library components the
// gean binary uses; only the network and the clock are replaced. Time moves
// only when the simulation advances it, and all work runs on the caller's
// goroutine in a fixed order, so a scenario produces the same chain every run.
package sim

import (
	"context"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/geanlabs/gean/crypto/xmss"
	"github.com/geanlabs/gean/db"
	"github.com/geanlabs/gean/genesis"
	"github.com/geanlabs/gean/node"
	"github.com/geanlabs/gean/role"
	"github.com/geanlabs/gean/store"
	"github.com/geanlabs/gean/types"
)

// keyLifetime is how many slots a simulated validator key can sign for. It is
// far below a real key's lifetime, so generating keys takes seconds.
const keyLifetime = 1 << 10

// Config describes a simulated network.
type Config struct {
	// GenesisTime is the genesis time in seconds; the clock starts there.
	GenesisTime uint64
	// Validators is the validator count. Validator i signs with keys derived
	// from i, so every run uses the same keys.
	Validators int
	// Nodes lists, for each node, the validators whose keys it holds.
	Nodes [][]uint64
	// Aggregators lists the nodes that aggregate attestations.
	Aggregators []int
}

// Node is one simulated node.
type Node struct {
	Engine *node.Engine
	Store  *store.ConsensusStore
	// Aggregator is the node's aggregator role; setting it switches
	// aggregation on or off at runtime, as the admin API does.
	Aggregator *role.Controller
	keys       *xmss.KeyManager
	pubKeys    *xmss.PubKeyCache
}

// Cluster is a set of nodes sharing a simulated network and clock.
type Cluster struct {
	clock Clock
	nodes []*Node
}

// New builds every node from one genesis and runs their first tick at the
// genesis time.
func New(ctx context.Context, cfg Config) (*Cluster, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	attKeys, propKeys, entries, err := validatorKeys(cfg.Validators)
	if err != nil {
		return nil, err
	}
	aggregators := make(map[int]bool, len(cfg.Aggregators))
	for _, i := range cfg.Aggregators {
		aggregators[i] = true
	}

	c := &Cluster{clock: Clock{now: time.Unix(int64(cfg.GenesisTime), 0)}}
	owned := make(map[uint64]bool)
	for i, validators := range cfg.Nodes {
		nodeAtt := make(map[uint64]*xmss.ValidatorKeyPair, len(validators))
		nodeProp := make(map[uint64]*xmss.ValidatorKeyPair, len(validators))
		for _, id := range validators {
			nodeAtt[id], nodeProp[id] = attKeys[id], propKeys[id]
		}
		n, err := c.newNode(i, cfg, entries, nodeAtt, nodeProp, aggregators[i])
		if err != nil {
			c.Close()
			closeKeys(attKeys, propKeys, owned)
			return nil, fmt.Errorf("node %d: %w", i, err)
		}
		for _, id := range validators {
			owned[id] = true
		}
		c.nodes = append(c.nodes, n)
	}
	closeKeys(attKeys, propKeys, owned)

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

func (c *Cluster) newNode(index int, cfg Config, entries []genesis.GenesisValidatorEntry, attKeys, propKeys map[uint64]*xmss.ValidatorKeyPair, aggregator bool) (*Node, error) {
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

	n := &Node{Store: s, Aggregator: role.New(aggregator), pubKeys: xmss.NewPubKeyCache()}
	if len(attKeys) > 0 {
		n.keys = xmss.NewKeyManager(attKeys, propKeys)
	}
	n.Engine = node.New(node.Components{
		Store:      s,
		ForkChoice: fc,
		Network:    &network{cluster: c, self: index},
		Keys:       n.keys,
		PubKeys:    n.pubKeys,
		Aggregator: n.Aggregator,
		Clock:      &c.clock,
	}, node.Config{CommitteeCount: types.AttestationCommitteeCount})
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

// Close releases every node's keys and native caches.
func (c *Cluster) Close() {
	for _, n := range c.nodes {
		n.keys.Close()
		n.pubKeys.Close()
	}
}

// validatorKeys derives each validator's attestation and proposal keys from
// its index and returns them with their genesis entries.
func validatorKeys(count int) (att, prop map[uint64]*xmss.ValidatorKeyPair, entries []genesis.GenesisValidatorEntry, err error) {
	att = make(map[uint64]*xmss.ValidatorKeyPair, count)
	prop = make(map[uint64]*xmss.ValidatorKeyPair, count)
	for i := range uint64(count) {
		attKey, attHex, err := keyPair(fmt.Sprintf("gean-sim-validator-%d-attestation", i), i)
		if err != nil {
			closeKeys(att, prop, nil)
			return nil, nil, nil, err
		}
		att[i] = attKey
		propKey, propHex, err := keyPair(fmt.Sprintf("gean-sim-validator-%d-proposal", i), i)
		if err != nil {
			closeKeys(att, prop, nil)
			return nil, nil, nil, err
		}
		prop[i] = propKey
		entries = append(entries, genesis.GenesisValidatorEntry{AttestationPubkey: attHex, ProposalPubkey: propHex})
	}
	return att, prop, entries, nil
}

func keyPair(seed string, index uint64) (*xmss.ValidatorKeyPair, string, error) {
	kp, err := xmss.GenerateKeyPair(seed, 0, keyLifetime)
	if err != nil {
		return nil, "", fmt.Errorf("generate key %s: %w", seed, err)
	}
	kp.Index = index
	pk, err := kp.PublicKeyBytes()
	if err != nil {
		kp.Close()
		return nil, "", fmt.Errorf("public key %s: %w", seed, err)
	}
	return kp, hex.EncodeToString(pk[:]), nil
}

// closeKeys closes the generated keys no node took ownership of.
func closeKeys(att, prop map[uint64]*xmss.ValidatorKeyPair, owned map[uint64]bool) {
	for id, kp := range att {
		if !owned[id] {
			kp.Close()
		}
	}
	for id, kp := range prop {
		if !owned[id] {
			kp.Close()
		}
	}
}
