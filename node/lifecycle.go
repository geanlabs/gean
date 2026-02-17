package node

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/geanlabs/gean/chain/forkchoice"
	"github.com/geanlabs/gean/chain/statetransition"
	"github.com/geanlabs/gean/xmss/leansig"
	"github.com/geanlabs/gean/network"
	"github.com/geanlabs/gean/network/gossipsub"
	"github.com/geanlabs/gean/observability/logging"
	"github.com/geanlabs/gean/observability/metrics"
	"github.com/geanlabs/gean/storage/memory"
	"github.com/geanlabs/gean/types"
)

// New creates and wires up a new Node.
func New(cfg Config) (*Node, error) {
	log := logging.NewComponentLogger(logging.CompNode)

	// Generate genesis.
	genesisState := statetransition.GenerateGenesis(cfg.GenesisTime, cfg.Validators)
	emptyBody := &types.BlockBody{Attestations: []*types.Attestation{}}

	genesisBlock := &types.Block{
		Slot:          0,
		ProposerIndex: 0,
		ParentRoot:    types.ZeroHash,
		StateRoot:     types.ZeroHash,
		Body:          emptyBody,
	}

	// Compute genesis state root and set it on the block.
	stateRoot, _ := genesisState.HashTreeRoot()
	genesisBlock.StateRoot = stateRoot

	genesisRoot, _ := genesisBlock.HashTreeRoot()
	log.Info("genesis state initialized",
		"state_root", logging.ShortHash(stateRoot),
		"block_root", logging.ShortHash(genesisRoot),
	)

	// Initialize storage and fork choice.
	store := memory.New()
	fc := forkchoice.NewStore(genesisState, genesisBlock, store)

	// Create network host.
	host, err := network.NewHost(cfg.ListenAddr, cfg.NodeKeyPath, cfg.Bootnodes)
	if err != nil {
		return nil, fmt.Errorf("create host: %w", err)
	}

	netLog := logging.NewComponentLogger(logging.CompNetwork)
	netLog.Info("libp2p host started",
		"peer_id", host.P2P.ID().String()[:16]+"...",
		"addr", cfg.ListenAddr,
	)

	// Join gossip topics.
	devnetID := cfg.DevnetID
	if devnetID == "" {
		devnetID = "devnet0"
	}
	topics, err := gossipsub.JoinTopics(host.PubSub, devnetID)
	if err != nil {
		host.Close()
		return nil, fmt.Errorf("join topics: %w", err)
	}

	gossipLog := logging.NewComponentLogger(logging.CompGossip)
	gossipLog.Info("gossipsub topics joined", "devnet", devnetID)

	clock := NewClock(cfg.GenesisTime)

	validatorKeys := make(map[uint64]forkchoice.Signer)
	if cfg.ValidatorKeysDir != "" {
		for _, idx := range cfg.ValidatorIDs {
			pkPath := filepath.Join(cfg.ValidatorKeysDir, fmt.Sprintf("validator_%d.pk", idx))
			skPath := filepath.Join(cfg.ValidatorKeysDir, fmt.Sprintf("validator_%d.sk", idx))

			kp, err := leansig.LoadKeypair(pkPath, skPath)
			if err != nil {
				return nil, fmt.Errorf("failed to load keypair for validator %d: %w", idx, err)
			}
			validatorKeys[idx] = kp
			log.Info("loaded validator keypair", "validator_index", idx)
		}
	} else if len(cfg.ValidatorIDs) > 0 {
		log.Warn("no validator keys directory specified; validator duties will fail signing")
	}

	validator := &ValidatorDuties{
		Indices:            cfg.ValidatorIDs,
		Keys:               validatorKeys,
		FC:                 fc,
		Topics:             topics,
		PublishBlock:       gossipsub.PublishBlock,
		PublishAttestation: gossipsub.PublishAttestation,
		Log:                logging.NewComponentLogger(logging.CompValidator),
	}

	n := &Node{
		FC:        fc,
		Host:      host,
		Topics:    topics,
		Clock:     clock,
		Validator: validator,
		log:       log,
	}

	// Register gossip and req/resp handlers.
	if err := registerHandlers(n, fc); err != nil {
		host.Close()
		return nil, err
	}

	// Connect to bootnodes.
	if len(cfg.Bootnodes) > 0 {
		network.ConnectBootnodes(host.Ctx, host.P2P, cfg.Bootnodes)
	}

	// Start metrics.
	if cfg.MetricsPort > 0 {
		metrics.NodeInfo.WithLabelValues("gean", version).Set(1)
		metrics.NodeStartTime.Set(float64(time.Now().Unix()))
		metrics.ValidatorsCount.Set(float64(len(cfg.ValidatorIDs)))
		metrics.Serve(cfg.MetricsPort)
		log.Info("metrics server started", "port", cfg.MetricsPort)
	}

	return n, nil
}
