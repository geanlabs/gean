// Package sim runs lean nodes in one process on an in-memory p2p network, for
// multi-node tests. Each node is a full lean.Run with its own data directory,
// node key and validator keys, made by gean's keygen; the nodes reach each
// other without p2p sockets. Their HTTP API is served on loopback, so tests
// observe a node the way an operator would.
package sim

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/geanlabs/gean/internal/core/clock"
	"github.com/geanlabs/gean/internal/core/p2p/memnet"
	"github.com/geanlabs/gean/internal/lean"
	"github.com/geanlabs/gean/internal/lean/keygen"
)

// Config describes a simulated network.
type Config struct {
	Nodes      int
	Validators int
	// GenesisDelay is how far after Start genesis falls, leaving the nodes
	// time to start and connect.
	GenesisDelay time.Duration
	// Aggregators are the indices of the nodes that aggregate attestations.
	Aggregators []int
	// Clock drives every node. Nil means the system clock.
	Clock clock.Clock
}

// Sim is a running simulated network.
type Sim struct {
	Nodes   []*Node
	network *memnet.Network
}

// Node is one lean node in a Sim.
type Node struct {
	ID string
	// API is the base URL of the node's HTTP API.
	API string

	cfg    lean.Config
	cancel context.CancelFunc
	done   chan error
}

// Start generates a network for cfg and starts every node on it. The nodes
// are stopped when the test ends.
func Start(t testing.TB, cfg Config) *Sim {
	t.Helper()
	clk := cfg.Clock
	if clk == nil {
		clk = clock.Real{}
	}
	dir := t.TempDir()
	testnet := filepath.Join(dir, "testnet")
	generate := func(genesis time.Time) {
		if err := keygen.Run([]string{
			"--validators", strconv.Itoa(cfg.Validators),
			"--nodes", strconv.Itoa(cfg.Nodes),
			"--genesis-time", strconv.FormatInt(genesis.Unix(), 10),
			"--output", testnet,
		}, io.Discard); err != nil {
			t.Fatalf("keygen: %v", err)
		}
	}
	// Generating XMSS keys takes a while, so genesis is set once they exist:
	// the second run reuses the keys and rewrites only the network config.
	generate(clk.Now())
	generate(clk.Now().Add(cfg.GenesisDelay))

	s := &Sim{network: memnet.New()}
	t.Cleanup(func() { s.network.Close() })
	aggregators := make(map[int]bool, len(cfg.Aggregators))
	for _, i := range cfg.Aggregators {
		aggregators[i] = true
	}
	for i := range cfg.Nodes {
		id := fmt.Sprintf("node%d", i)
		apiPort, metricsPort := freePort(t), freePort(t)
		n := &Node{
			ID:  id,
			API: fmt.Sprintf("http://127.0.0.1:%d", apiPort),
			cfg: lean.Config{
				ConfigDir:    testnet,
				NodeKey:      filepath.Join(testnet, id+".key"),
				NodeID:       id,
				DataDir:      filepath.Join(dir, "data", id),
				HTTPAddr:     "127.0.0.1",
				APIPort:      apiPort,
				MetricsPort:  metricsPort,
				IsAggregator: aggregators[i],
				Clock:        clk,
				Network:      s.network,
			},
		}
		n.Start()
		t.Cleanup(func() {
			if err := n.Stop(); err != nil {
				t.Errorf("%s: %v", n.ID, err)
			}
		})
		s.Nodes = append(s.Nodes, n)
	}
	return s
}

// Start runs the node until Stop. A node that has stopped can start again on
// the same data directory.
func (n *Node) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	n.cancel = cancel
	n.done = make(chan error, 1)
	go func() { n.done <- lean.Run(ctx, n.cfg) }()
}

// Stop shuts the node down and returns what lean.Run returned. Stopping a
// stopped node does nothing.
func (n *Node) Stop() error {
	if n.cancel == nil {
		return nil
	}
	n.cancel()
	n.cancel = nil
	return <-n.done
}

// Get decodes the JSON the node's API serves at path into v.
func (n *Node) Get(path string, v any) error {
	resp, err := http.Get(n.API + path)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", path, resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

// ForkChoice is the node's fork-choice view, as /lean/v0/fork_choice serves it.
type ForkChoice struct {
	Head  string `json:"head"`
	Nodes []struct {
		Root          string `json:"root"`
		Slot          uint64 `json:"slot"`
		ProposerIndex uint64 `json:"proposer_index"`
	} `json:"nodes"`
	Justified Checkpoint `json:"justified"`
	Finalized Checkpoint `json:"finalized"`
}

// Checkpoint is a checkpoint as the API serves it.
type Checkpoint struct {
	Root string `json:"root"`
	Slot uint64 `json:"slot"`
}

// ForkChoice returns the node's fork-choice view.
func (n *Node) ForkChoice() (ForkChoice, error) {
	var fc ForkChoice
	err := n.Get("/lean/v0/fork_choice", &fc)
	return fc, err
}

// Eventually polls cond until it holds, failing the test after timeout.
func Eventually(t testing.TB, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out after %s waiting for %s", timeout, what)
		}
		time.Sleep(250 * time.Millisecond)
	}
}

// freePort returns a loopback TCP port free at the time of the call.
func freePort(t testing.TB) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}
