package node

import (
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/geanlabs/gean/chain/forkchoice"
	"github.com/geanlabs/gean/chain/statetransition"
	"github.com/geanlabs/gean/network"
	"github.com/geanlabs/gean/network/gossipsub"
	"github.com/geanlabs/gean/network/p2p"
	"github.com/geanlabs/gean/observability/logging"
	"github.com/geanlabs/gean/observability/metrics"
	"github.com/geanlabs/gean/storage/memory"
	"github.com/geanlabs/gean/types"
	"github.com/geanlabs/gean/xmss/leansig"
)

// New creates and wires up a new Node.
func New(cfg Config) (*Node, error) {
	log := logging.NewComponentLogger(logging.CompNode)

	fc := initGenesis(log, cfg)

	host, topics, err := initP2P(cfg)
	if err != nil {
		return nil, err
	}

	p2pManager, p2pDiscovery, err2 := initDiscovery(log, cfg)
	if err2 != nil {
		host.Close()
		return nil, err2
	}

	validatorKeys, err := loadValidatorKeys(log, cfg)
	if err != nil {
		if p2pDiscovery != nil {
			p2pDiscovery.Close()
		}
		if p2pManager != nil {
			p2pManager.Close()
		}
		host.Close()
		return nil, err
	}

	validator := &ValidatorDuties{
		Indices:                      cfg.ValidatorIDs,
		Keys:                         validatorKeys,
		FC:                           fc,
		Topics:                       topics,
		PublishBlock:                 gossipsub.PublishBlock,
		PublishAttestation:           gossipsub.PublishAttestation,
		PublishAggregatedAttestation: gossipsub.PublishAggregatedAttestation,
		Log:                          logging.NewComponentLogger(logging.CompValidator),
	}

	n := &Node{
		FC:           fc,
		Host:         host,
		Topics:       topics,
		Clock:        NewClock(cfg.GenesisTime),
		Validator:    validator,
		P2PManager:   p2pManager,
		P2PDiscovery: p2pDiscovery,
		log:          log,
	}

	if err := registerHandlers(n, fc); err != nil {
		if p2pDiscovery != nil {
			p2pDiscovery.Close()
		}
		if p2pManager != nil {
			p2pManager.Close()
		}
		host.Close()
		return nil, err
	}

	if len(cfg.Bootnodes) > 0 {
		network.ConnectBootnodes(host.Ctx, host.P2P, cfg.Bootnodes)
	}

	startMetrics(log, cfg)

	return n, nil
}

func initGenesis(log *slog.Logger, cfg Config) *forkchoice.Store {
	genesisState := statetransition.GenerateGenesis(cfg.GenesisTime, cfg.Validators)

	genesisBlock := &types.Block{
		Slot:          0,
		ProposerIndex: 0,
		ParentRoot:    types.ZeroHash,
		StateRoot:     types.ZeroHash,
		Body:          &types.BlockBody{Attestations: []*types.Attestation{}},
	}

	stateRoot, _ := genesisState.HashTreeRoot()
	genesisBlock.StateRoot = stateRoot

	genesisRoot, _ := genesisBlock.HashTreeRoot()
	log.Info("genesis state initialized",
		"state_root", logging.ShortHash(stateRoot),
		"block_root", logging.ShortHash(genesisRoot),
	)

	fc := forkchoice.NewStore(genesisState, genesisBlock, memory.New())
	fc.NowFn = func() uint64 { return uint64(time.Now().Unix()) }
	return fc
}

func initP2P(cfg Config) (*network.Host, *gossipsub.Topics, error) {
	host, err := network.NewHost(cfg.ListenAddr, cfg.NodeKeyPath, cfg.Bootnodes)
	if err != nil {
		return nil, nil, fmt.Errorf("create host: %w", err)
	}

	netLog := logging.NewComponentLogger(logging.CompNetwork)
	netLog.Info("libp2p host started",
		"peer_id", host.P2P.ID().String()[:16]+"...",
		"addr", cfg.ListenAddr,
	)

	devnetID := cfg.DevnetID
	if devnetID == "" {
		devnetID = "devnet0"
	}
	topics, err := gossipsub.JoinTopics(host.PubSub, devnetID)
	if err != nil {
		host.Close()
		return nil, nil, fmt.Errorf("join topics: %w", err)
	}

	gossipLog := logging.NewComponentLogger(logging.CompGossip)
	gossipLog.Info("gossipsub topics joined", "devnet", devnetID)

	return host, topics, nil
}

func initDiscovery(log *slog.Logger, cfg Config) (*p2p.LocalNodeManager, *p2p.DiscoveryService, error) {
	discPort := cfg.DiscoveryPort
	if discPort == 0 {
		discPort = 9000
	}

	p2pDBPath := filepath.Join(cfg.DataDir, "p2p")
	if err := os.MkdirAll(p2pDBPath, 0700); err != nil {
		return nil, nil, fmt.Errorf("failed to create p2p db dir: %w", err)
	}

	p2pManager, err := p2p.NewLocalNodeManager(p2pDBPath, cfg.NodeKeyPath, net.IPv4(0, 0, 0, 0), discPort, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to init p2p manager: %w", err)
	}

	p2pDiscovery, err := p2p.NewDiscoveryService(p2pManager, discPort, cfg.Bootnodes)
	if err != nil {
		log.Warn("p2p discovery unavailable", "err", err)
	}

	return p2pManager, p2pDiscovery, nil
}

func loadValidatorKeys(log *slog.Logger, cfg Config) (map[uint64]forkchoice.Signer, error) {
	keys := make(map[uint64]forkchoice.Signer)
	if cfg.ValidatorKeysDir == "" {
		if len(cfg.ValidatorIDs) > 0 {
			log.Warn("no validator keys directory specified; validator duties will fail signing")
		}
		return keys, nil
	}

	for _, idx := range cfg.ValidatorIDs {
		pkPath := filepath.Join(cfg.ValidatorKeysDir, fmt.Sprintf("validator_%d_pk.ssz", idx))
		skPath := filepath.Join(cfg.ValidatorKeysDir, fmt.Sprintf("validator_%d_sk.ssz", idx))

		kp, err := leansig.LoadKeypair(pkPath, skPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load keypair for validator %d: %w", idx, err)
		}
		keys[idx] = kp
		log.Info("loaded validator keypair", "validator_index", idx)
	}
	return keys, nil
}

func startMetrics(log *slog.Logger, cfg Config) {
	if cfg.MetricsPort <= 0 {
		return
	}
	metrics.NodeInfo.WithLabelValues("gean", Version).Set(1)
	metrics.NodeStartTime.Set(float64(time.Now().Unix()))
	metrics.ValidatorsCount.Set(float64(len(cfg.ValidatorIDs)))
	metrics.Serve(cfg.MetricsPort)
	log.Info("metrics server started", "port", cfg.MetricsPort)
}
