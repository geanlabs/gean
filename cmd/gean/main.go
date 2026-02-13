package main

import (
	"context"
	"flag"
	"io"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/geanlabs/gean/config"
	"github.com/geanlabs/gean/observability/logging"
	"github.com/geanlabs/gean/node"
)

const version = "v0.1.0"

func main() {
	genesisPath := flag.String("genesis", "", "Path to config.yaml")
	bootnodesPath := flag.String("bootnodes", "", "Path to nodes.yaml")
	validatorsPath := flag.String("validator-registry-path", "", "Path to validators.yaml")
	nodeID := flag.String("node-id", "", "Node name (index into validators.yaml)")
	nodeKey := flag.String("node-key", "", "Path to secp256k1 private key file")
	listenAddr := flag.String("listen-addr", "/ip4/0.0.0.0/udp/9000/quic-v1", "QUIC listen address")
	metricsPort := flag.Int("metrics-port", 0, "Prometheus metrics port (0 = disabled)")
	devnetID := flag.String("devnet-id", "devnet0", "Devnet identifier for gossip topics")
	logLevel := flag.String("log-level", "info", "Log level (debug, info, warn, error)")
	flag.Parse()

	// Initialize structured logger and suppress noisy stdlib log output (quic-go, etc.).
	logging.Init(parseLevel(*logLevel))
	log.SetOutput(io.Discard)

	logger := logging.NewComponentLogger(logging.CompNode)

	if *genesisPath == "" {
		logger.Error("--genesis flag is required")
		os.Exit(1)
	}

	// Print banner first.
	logging.Banner(version)

	// Load genesis config.
	genCfg, err := config.LoadGenesisConfig(*genesisPath)
	if err != nil {
		logger.Error("failed to load genesis config", "err", err)
		os.Exit(1)
	}
	logger.Info("genesis config loaded",
		"genesis_time", genCfg.GenesisTime,
		"validators", genCfg.ValidatorCount,
	)

	// Load bootnodes.
	var bootnodes []string
	if *bootnodesPath != "" {
		nodes, err := config.LoadBootnodes(*bootnodesPath)
		if err != nil {
			logger.Error("failed to load bootnodes", "err", err)
			os.Exit(1)
		}
		for _, n := range nodes {
			bootnodes = append(bootnodes, n.Multiaddr)
		}
		if len(bootnodes) > 0 {
			logger.Info("bootnodes loaded", "count", len(bootnodes))
		}
	}

	// Load validator assignments.
	var validatorIDs []uint64
	if *validatorsPath != "" && *nodeID != "" {
		reg, err := config.LoadValidators(*validatorsPath)
		if err != nil {
			logger.Error("failed to load validators", "err", err)
			os.Exit(1)
		}
		validatorIDs = reg.GetValidatorIndices(*nodeID)
		if len(validatorIDs) == 0 {
			logger.Warn("no validators found for node", "node_id", *nodeID)
		} else {
			logger.Info("validator duties loaded",
				"node_id", *nodeID,
				"validators", strconv.Itoa(len(validatorIDs)),
			)
		}
	}

	nodeCfg := node.Config{
		GenesisTime:   genCfg.GenesisTime,
		NumValidators: genCfg.ValidatorCount,
		ListenAddr:    *listenAddr,
		NodeKeyPath:   *nodeKey,
		Bootnodes:     bootnodes,
		ValidatorIDs:  validatorIDs,
		MetricsPort:   *metricsPort,
		DevnetID:      *devnetID,
	}

	n, err := node.New(nodeCfg)
	if err != nil {
		logger.Error("failed to initialize node", "err", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle signals.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	if err := n.Run(ctx); err != nil {
		logger.Error("node exited with error", "err", err)
		os.Exit(1)
	}
}

func parseLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
