// Package launch assembles and runs a full gean node from the library: the
// Pebble store, the libp2p network, the XMSS prover, the engine, the sync
// driver and the HTTP servers. cmd/gean only parses flags and handles
// signals; anything that wants a node the binary would run calls Launch.
package launch

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/geanlabs/gean/api"
	"github.com/geanlabs/gean/consensus/forkchoice"
	"github.com/geanlabs/gean/consensus/genesis"
	"github.com/geanlabs/gean/crypto"
	"github.com/geanlabs/gean/crypto/xmss"
	"github.com/geanlabs/gean/logger"
	"github.com/geanlabs/gean/metrics"
	"github.com/geanlabs/gean/net/p2p"
	"github.com/geanlabs/gean/net/syncer"
	"github.com/geanlabs/gean/node"
	"github.com/geanlabs/gean/role"
	"github.com/geanlabs/gean/shadow"
	"github.com/geanlabs/gean/storage/db/pebbledb"
	"github.com/geanlabs/gean/storage/store"
	"github.com/geanlabs/gean/tasks"
	"github.com/geanlabs/gean/types"
)

// ShutdownTimeout bounds how long Wait waits for every service to finish. An
// in-flight proof cannot be interrupted, so this is generous.
const ShutdownTimeout = 30 * time.Second

// ErrShutdownTimeout is returned by Wait when services did not finish within
// ShutdownTimeout. Storage and keys are then still in use and must not be
// released; the process should exit.
var ErrShutdownTimeout = errors.New("shutdown timed out")

// APIHandlerFunc builds the node API served at Config.APIAddress.
type APIHandlerFunc func(s *store.ConsensusStore, forkChoiceView func() *forkchoice.View, aggregator *role.Controller, scheme crypto.Scheme) http.Handler

// Config is everything a node needs that is not derived from its data
// directory.
type Config struct {
	DataDir       string
	Genesis       *genesis.GenesisConfig
	CheckpointURL string
	Bootnodes     []multiaddr.Multiaddr
	// Keys signs this node's duties. The caller owns them and closes them
	// after Wait returns.
	Keys               *xmss.KeyManager
	NodeKeyPath        string
	GossipPort         int
	APIAddress         string
	MetricsAddress     string
	CommitteeCount     uint64
	AggregateSubnetIDs []uint64
	IsAggregator       bool
	ProverArena        bool
	Shadow             shadow.Rates
	GitCommit          string
	// APIHandler builds the API; nil serves api.NewHandler.
	APIHandler APIHandlerFunc
}

// Node is a running node.
type Node struct {
	cancel   context.CancelFunc
	ctx      context.Context
	services tasks.Group
	closers  []func()

	failOnce sync.Once
	failure  error
}

// Launch opens the store, prepares the chain, starts the network, the engine,
// the sync driver and the HTTP servers, and returns once they are running.
// Every service is critical: if one fails, the node shuts down and Wait
// returns the failure. Cancelling ctx stops the node.
func Launch(ctx context.Context, cfg Config) (n *Node, err error) {
	ctx, cancel := context.WithCancel(ctx)
	n = &Node{ctx: ctx, cancel: cancel}
	defer func() {
		if err != nil {
			cancel()
			n.release()
		}
	}()

	p2p.SetClientGitCommit(cfg.GitCommit)
	nodeMetrics := metrics.New(prometheus.DefaultRegisterer)
	nodeMetrics.SetNodeInfo("gean", cfg.GitCommit)

	dataDir, err := filepath.Abs(cfg.DataDir)
	if err != nil {
		return nil, fmt.Errorf("resolve data dir: %w", err)
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	logger.Info(logger.Node, "storage: %s", dataDir)
	backend, err := pebbledb.Open(dataDir)
	if err != nil {
		return nil, fmt.Errorf("open pebble: %w", err)
	}
	n.closers = append(n.closers, func() { backend.Close() })
	s := store.NewConsensusStore(backend)

	if cfg.CheckpointURL != "" {
		logger.Info(logger.Node, "checkpoint sync configured: url=%s", cfg.CheckpointURL)
	} else {
		logger.Info(logger.Node, "checkpoint sync not configured (no --checkpoint-sync-url)")
	}
	fc, err := node.OpenChain(s, cfg.Genesis, cfg.CheckpointURL, time.Now())
	if err != nil {
		return nil, err
	}

	host, err := p2p.NewHost(ctx, cfg.NodeKeyPath, cfg.GossipPort, cfg.CommitteeCount, cfg.Keys.ValidatorIDs(), cfg.IsAggregator, cfg.AggregateSubnetIDs)
	if err != nil {
		return nil, fmt.Errorf("create p2p host: %w", err)
	}
	n.closers = append(n.closers, host.Close)
	logger.Info(logger.Network, "p2p: peer_id=%s listen_port=%d", host.PeerID(), cfg.GossipPort)
	instrumentP2P(host, nodeMetrics)
	// Handlers must be live before the prover pre-init: the listener is
	// already accepting connections, and peers that dial during the
	// multi-second init would fail req/resp protocol negotiation.
	registerReqRespHandlers(host, s)

	proving := len(cfg.Keys.ValidatorIDs()) > 0 || cfg.IsAggregator
	xmss.SetProverArena(cfg.ProverArena)
	warnIfMemoryLimited(proving)
	if err := preinitializeXMSS(proving); err != nil {
		return nil, err
	}

	aggregator := role.NewWithHook(cfg.IsAggregator, nodeMetrics.SetIsAggregator)
	scheme := xmss.NewScheme()
	n.closers = append(n.closers, scheme.Close)
	engine := node.New(node.Components{
		Store:      s,
		ForkChoice: fc,
		Network:    host,
		Keys:       cfg.Keys,
		Crypto:     scheme,
		Aggregator: aggregator,
		Clock:      node.SystemClock{},
		Metrics:    nodeMetrics,
	}, node.Config{
		CommitteeCount:     cfg.CommitteeCount,
		AggregateSubnetIDs: cfg.AggregateSubnetIDs,
		Shadow:             cfg.Shadow,
	})

	host.StartGossipListeners(engine)
	n.services.GoCritical("engine", func() error { engine.Run(ctx); return nil }, n.fail)
	syncDriver := syncer.NewSyncDriver(ctx, engine, s, syncNetwork{host})
	host.Hooks.PeerStatus = func(id peer.ID) { syncDriver.OnPeerConnected(syncer.PeerID(id)) }
	n.services.GoCritical("sync driver", func() error { syncDriver.Run(); return nil }, n.fail)

	apiHandler := cfg.APIHandler
	if apiHandler == nil {
		apiHandler = func(s *store.ConsensusStore, view func() *forkchoice.View, aggregator *role.Controller, _ crypto.Scheme) http.Handler {
			return api.NewHandler(s, view, aggregator)
		}
	}
	handler := apiHandler(s, engine.ForkChoiceView, aggregator, scheme)
	n.services.GoCritical("api server", func() error { return api.Serve(ctx, "api", cfg.APIAddress, handler) }, n.fail)
	n.services.GoCritical("metrics server", func() error {
		return api.Serve(ctx, "metrics", cfg.MetricsAddress, api.NewMetricsHandler())
	}, n.fail)

	host.ConnectBootnodes(ctx, cfg.Bootnodes)
	host.StartBootnodeRedial(ctx, cfg.Bootnodes)
	host.ReannounceSubscriptionsAfter(5 * time.Second)

	logger.Info(logger.Node, "gean started: api=%s metrics=%s aggregator=%v", cfg.APIAddress, cfg.MetricsAddress, cfg.IsAggregator)
	return n, nil
}

// Stop asks the node to shut down; Wait reports when it has.
func (n *Node) Stop() { n.cancel() }

// Wait blocks until the node is stopped or a service fails, then waits up to
// ShutdownTimeout for every service to finish and releases the scheme, the
// network and storage. It returns the first service failure, nil after a
// clean stop, or ErrShutdownTimeout, in which case nothing was released.
func (n *Node) Wait() error {
	<-n.ctx.Done()
	logger.Info(logger.Node, "shutting down...")
	done := make(chan struct{})
	go func() {
		n.services.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(ShutdownTimeout):
		return ErrShutdownTimeout
	}
	n.release()
	return n.failure
}

// fail records the first service failure and shuts the node down.
func (n *Node) fail(err error) {
	n.failOnce.Do(func() {
		logger.Error(logger.Node, "%v", err)
		n.failure = err
	})
	n.cancel()
}

// release closes what Launch opened, newest first.
func (n *Node) release() {
	for i := len(n.closers) - 1; i >= 0; i-- {
		n.closers[i]()
	}
	n.closers = nil
}

func instrumentP2P(p2pHost *p2p.Host, m *metrics.Metrics) {
	p2pHost.Hooks.GossipBlockSize = m.ObserveGossipBlockSize
	p2pHost.Hooks.GossipAttestationSize = m.ObserveGossipAttestationSize
	p2pHost.Hooks.GossipAggregationSize = m.ObserveGossipAggregationSize
	p2pHost.Hooks.PeerConnected = func(direction string) {
		m.IncPeerConnection(direction, "success")
	}
	p2pHost.Hooks.PeerDisconnected = func(direction, reason string) {
		m.IncPeerDisconnection(direction, reason)
	}
	p2pHost.Hooks.PeerCount = func(count int) {
		m.SetConnectedPeers("unknown", count)
	}
	p2pHost.Hooks.ReqRespTimeout = m.IncReqRespTimeout
	p2pHost.Hooks.ReqRespRequestSize = m.ObserveReqRespRequestSize
	p2pHost.Hooks.ReqRespResponseChunkSize = m.ObserveReqRespResponseChunkSize
}

// The prover owns a multi-GB resident arena; nodes that never prove (no
// validator keys, not aggregating) skip it and keep only the verifier. The
// proving FFI entry points lazily self-initialize, so an unexpected proving
// need still works — it just pays the init off the tick loop on first use.
func preinitializeXMSS(proving bool) error {
	if !proving {
		logger.Info(logger.Node, "no validator keys and not an aggregator; deferring XMSS prover init")
		return xmss.EnsureVerifierReady()
	}
	logger.Info(logger.Node, "pre-initializing XMSS prover (this takes ~45s)...")
	if err := xmss.EnsureProverReady(); err != nil {
		return err
	}
	logger.Info(logger.Node, "XMSS prover ready")
	return xmss.EnsureVerifierReady()
}

func registerReqRespHandlers(p2pHost *p2p.Host, s *store.ConsensusStore) {
	p2pHost.RegisterReqRespHandlers(
		s.Status,
		func(root [32]byte) *types.SignedBlock {
			return s.GetSignedBlock(root)
		},
		// The block-request window slides with the responder's current slot, not its
		// head: a lagging head would advertise a lower floor and offer history the
		// spec lets us prune.
		func() uint64 {
			return types.CurrentSlot(s.Config().GenesisTime, uint64(time.Now().UnixMilli()))
		},
		func(startSlot, count uint64) ([]*types.SignedBlock, bool) {
			return s.GetCanonicalBlocksInRange(startSlot, count)
		},
	)
}

// syncNetwork adapts the libp2p host to the sync driver's peer-neutral port.
type syncNetwork struct{ *p2p.Host }

func (n syncNetwork) Peers() []syncer.PeerID {
	peers := n.Host.Peers()
	ids := make([]syncer.PeerID, len(peers))
	for i, p := range peers {
		ids[i] = syncer.PeerID(p)
	}
	return ids
}

func (n syncNetwork) SendStatusRequest(ctx context.Context, id syncer.PeerID, ours *types.Status) (*types.Status, error) {
	return n.Host.SendStatusRequest(ctx, peer.ID(id), ours)
}

func (n syncNetwork) FetchBlocksByRange(ctx context.Context, id syncer.PeerID, startSlot, count uint64) ([]*types.SignedBlock, error) {
	return n.Host.FetchBlocksByRange(ctx, peer.ID(id), startSlot, count)
}
