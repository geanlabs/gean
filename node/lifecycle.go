package node

import (
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/multiformats/go-multiaddr"

	apiserver "github.com/geanlabs/gean/api/server"
	"github.com/geanlabs/gean/chain/forkchoice"
	"github.com/geanlabs/gean/chain/statetransition"
	"github.com/geanlabs/gean/network"
	"github.com/geanlabs/gean/network/gossipsub"
	"github.com/geanlabs/gean/network/p2p"
	"github.com/geanlabs/gean/observability/logging"
	"github.com/geanlabs/gean/observability/metrics"
	boltstore "github.com/geanlabs/gean/storage/bolt"
	"github.com/geanlabs/gean/types"
	"github.com/geanlabs/gean/xmss/leansig"
)

// New creates and wires up a new Node.
func New(cfg Config) (*Node, error) {
	log := logging.NewComponentLogger(logging.CompNode)

	fc, db, err := initChain(log, cfg)
	if err != nil {
		return nil, err
	}

	host, topics, err := initP2P(cfg)
	if err != nil {
		db.Close()
		return nil, err
	}

	p2pManager, p2pDiscovery, err2 := initDiscovery(log, cfg)
	if err2 != nil {
		host.Close()
		db.Close()
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
		db.Close()
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
		IsAggregator:                 cfg.IsAggregator,
		Log:                          logging.NewComponentLogger(logging.CompValidator),
	}

	n := &Node{
		FC:            fc,
		Host:          host,
		Topics:        topics,
		Clock:         NewClock(cfg.GenesisTime),
		Validator:     validator,
		P2PManager:    p2pManager,
		P2PDiscovery:  p2pDiscovery,
		PendingBlocks: NewPendingBlockCache(),
		dbCloser:      db,
		log:           log,
	}

	// Register req/resp handlers for sync. Gossip handlers are registered
	// before initial sync in ticker.go so blocks arriving during sync are
	// not silently dropped.
	registerReqRespHandlers(n, fc)

	if len(cfg.Bootnodes) > 0 {
		network.ConnectBootnodes(host.Ctx, host.P2P, cfg.Bootnodes)
	}

	startMetrics(log, cfg)
	apiServer, err := startAPI(cfg, fc)
	if err != nil {
		if p2pDiscovery != nil {
			p2pDiscovery.Close()
		}
		if p2pManager != nil {
			p2pManager.Close()
		}
		host.Close()
		db.Close()
		return nil, err
	}
	n.API = apiServer

	return n, nil
}

func initChain(log *slog.Logger, cfg Config) (*forkchoice.Store, *boltstore.Store, error) {
	if err := os.MkdirAll(cfg.DataDir, 0700); err != nil {
		return nil, nil, fmt.Errorf("create data dir: %w", err)
	}
	dbPath := filepath.Join(cfg.DataDir, "gean.db")
	db, err := boltstore.New(dbPath)
	if err != nil {
		return nil, nil, fmt.Errorf("open database: %w", err)
	}

	var (
		fc                      *forkchoice.Store
		checkpointSyncSucceeded bool
	)

	if cfg.CheckpointSyncURL != "" {
		log.Info("checkpoint sync enabled, downloading state from", "url", cfg.CheckpointSyncURL)

		state, err := downloadCheckpointState(cfg.CheckpointSyncURL)
		if err != nil {
			log.Warn("checkpoint sync failed, falling back to database/genesis", "err", err)
		} else {
			preparedState, stateRoot, blockRoot, err := verifyCheckpointState(state, cfg.GenesisTime, cfg.Validators)
			if err != nil {
				log.Warn("checkpoint state verification failed, falling back to database/genesis", "err", err)
			} else {
				log.Info("checkpoint state verified",
					"slot", preparedState.Slot,
					"state_root", logging.ShortHash(stateRoot),
					"block_root", logging.ShortHash(blockRoot),
				)
				fc = forkchoice.NewStoreFromCheckpointState(preparedState, blockRoot, db)
				checkpointSyncSucceeded = true
				log.Info("checkpoint sync completed successfully, using state as anchor", "slot", preparedState.Slot)
			}
		}
	}

	if fc == nil {
		fc = forkchoice.RestoreFromDB(db)
	}

	if fc != nil && !checkpointSyncSucceeded {
		status := fc.GetStatus()
		log.Info("chain restored from database",
			"head", logging.ShortHash(status.Head),
			"head_slot", status.HeadSlot,
			"justified_slot", status.JustifiedSlot,
			"finalized_slot", status.FinalizedSlot,
		)
	} else if fc != nil {
		status := fc.GetStatus()
		log.Info("chain initialized from checkpoint state",
			"head", logging.ShortHash(status.Head),
			"head_slot", status.HeadSlot,
			"justified_slot", status.JustifiedSlot,
			"finalized_slot", status.FinalizedSlot,
		)
	} else {
		genesisState := statetransition.GenerateGenesis(cfg.GenesisTime, cfg.Validators)

		genesisBlock := &types.Block{
			Slot:          0,
			ProposerIndex: 0,
			ParentRoot:    types.ZeroHash,
			StateRoot:     types.ZeroHash,
			Body:          &types.BlockBody{Attestations: []*types.AggregatedAttestation{}},
		}

		stateRoot, _ := genesisState.HashTreeRoot()
		genesisBlock.StateRoot = stateRoot

		genesisRoot, _ := genesisBlock.HashTreeRoot()
		log.Info("genesis state initialized",
			"state_root", logging.ShortHash(stateRoot),
			"block_root", logging.ShortHash(genesisRoot),
		)

		fc = forkchoice.NewStore(genesisState, genesisBlock, db)
	}

	fc.NowFn = func() uint64 { return uint64(time.Now().UnixMilli()) }
	fc.SetIsAggregator(cfg.IsAggregator)

	return fc, db, nil
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
	topics, err := gossipsub.JoinTopics(host.PubSub, devnetID, 0)
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

	// Parse QUIC port from listen address for ENR advertisement
	quicPort := parseQUICPort(cfg.ListenAddr)

	p2pDBPath := filepath.Join(cfg.DataDir, "p2p")
	if err := os.MkdirAll(p2pDBPath, 0700); err != nil {
		return nil, nil, fmt.Errorf("failed to create p2p db dir: %w", err)
	}

	p2pManager, err := p2p.NewLocalNodeManager(p2pDBPath, cfg.NodeKeyPath, net.IPv4(0, 0, 0, 0), discPort, 0, quicPort)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to init p2p manager: %w", err)
	}

	if local := p2pManager.LocalNode(); local != nil {
		local.Set(p2p.AggregatorEntry(cfg.IsAggregator))
	}

	p2pDiscovery, err := p2p.NewDiscoveryService(p2pManager, discPort, cfg.Bootnodes)
	if err != nil {
		log.Warn("p2p discovery unavailable", "err", err)
	}

	return p2pManager, p2pDiscovery, nil
}

func startAPI(cfg Config, fc *forkchoice.Store) (*apiserver.Server, error) {
	apiCfg := apiserver.Config{
		Host:    cfg.APIHost,
		Port:    cfg.APIPort,
		Enabled: cfg.APIEnabled,
	}
	apiServer := apiserver.New(apiCfg, func() *forkchoice.Store { return fc })
	if err := apiServer.Start(); err != nil {
		return nil, err
	}
	if !cfg.APIEnabled {
		return nil, nil
	}
	return apiServer, nil
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

	// Devnet-3 aggregator metrics.
	if cfg.IsAggregator {
		metrics.IsAggregator.Set(1)
	} else {
		metrics.IsAggregator.Set(0)
	}
	metrics.AttestationCommitteeCount.Set(1)  // Always 1 for devnet-3.
	metrics.AttestationCommitteeSubnet.Set(0) // Always subnet 0 for devnet-3.

	metrics.Serve(cfg.MetricsPort)
	log.Info("metrics server started", "port", cfg.MetricsPort)
}

// parseQUICPort extracts the UDP port from a QUIC multiaddr like /ip4/0.0.0.0/udp/9008/quic-v1.
func parseQUICPort(listenAddr string) int {
	if listenAddr == "" {
		return 0
	}
	ma, err := multiaddr.NewMultiaddr(listenAddr)
	if err != nil {
		return 0
	}
	// Extract the UDP port component (QUIC runs over UDP)
	val, err := ma.ValueForProtocol(multiaddr.P_UDP)
	if err != nil {
		return 0
	}
	port, err := strconv.Atoi(val)
	if err != nil {
		return 0
	}
	return port
}
