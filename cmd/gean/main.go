package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/devylongs/gean/node"
	"github.com/devylongs/gean/types"
)

func main() {
	genesisTime := flag.Uint64("genesis-time", 0, "Genesis time (Unix timestamp). Defaults to 10 seconds from now.")
	validators := flag.Uint64("validators", 8, "Number of validators in the network")
	validatorIndex := flag.Int64("validator-index", -1, "Validator index to run as (-1 for non-validator)")
	listen := flag.String("listen", "/ip4/0.0.0.0/udp/9000/quic-v1", "Listen multiaddr (QUIC)")
	bootnodes := flag.String("bootnodes", "", "Comma-separated bootnode multiaddrs")
	logLevel := flag.String("log-level", "info", "Log level (debug, info, warn, error)")
	flag.Parse()

	fmt.Println("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ gean ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	level := slog.LevelInfo
	switch *logLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))

	genesis := *genesisTime
	if genesis == 0 {
		genesis = uint64(time.Now().Unix()) + 10
		logger.Info("genesis time not set, using now + 10 seconds", "genesis_time", genesis)
	}

	valIdx := types.NoValidator
	if *validatorIndex >= 0 {
		valIdx = uint64(*validatorIndex)
		logger.Info("running as validator", "index", valIdx)
	}

	var bootnodesSlice []string
	if *bootnodes != "" {
		bootnodesSlice = strings.Split(*bootnodes, ",")
	}

	nodeCfg := &node.Config{
		GenesisTime:    genesis,
		ValidatorCount: *validators,
		ValidatorIndex: valIdx,
		ListenAddrs:    []string{*listen},
		Bootnodes:      bootnodesSlice,
		Logger:         logger,
	}

	logger.Info("config",
		"genesis_time", genesis,
		"validators", *validators,
		"bootnodes", len(bootnodesSlice),
	)

	ctx, cancel := context.WithCancel(context.Background())
	n, err := node.New(ctx, nodeCfg)
	if err != nil {
		logger.Error("failed to create node", "error", err)
		os.Exit(1)
	}

	n.Start()
	logger.Info("gean running", "slot", n.CurrentSlot(), "peers", n.PeerCount())

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	logger.Info("shutting down...")
	n.Stop()
	cancel()
}
