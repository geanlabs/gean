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
)

func main() {
	var (
		genesisTime    uint64
		validatorCount uint64
		validatorIndex int64
		listenAddr     string
		bootnodes      string
		logLevel       string
	)

	flag.Uint64Var(&genesisTime, "genesis-time", uint64(time.Now().Unix()), "Genesis time (unix timestamp)")
	flag.Uint64Var(&validatorCount, "validator-count", 4, "Number of validators")
	flag.Int64Var(&validatorIndex, "validator-index", -1, "Validator index (-1 for non-validator)")
	flag.StringVar(&listenAddr, "listen", "/ip4/0.0.0.0/tcp/9000", "Listen address")
	flag.StringVar(&bootnodes, "bootnodes", "", "Comma-separated bootnode multiaddrs")
	flag.StringVar(&logLevel, "log-level", "info", "Log level (debug, info, warn, error)")
	flag.Parse()

	// Setup logger
	var level slog.Level
	switch strings.ToLower(logLevel) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))

	// Parse bootnodes
	var bootnodeList []string
	if bootnodes != "" {
		bootnodeList = strings.Split(bootnodes, ",")
	}

	// Build config
	cfg := &node.Config{
		GenesisTime:    genesisTime,
		ValidatorCount: validatorCount,
		ListenAddrs:    []string{listenAddr},
		Bootnodes:      bootnodeList,
		Logger:         logger,
	}

	if validatorIndex >= 0 {
		idx := uint64(validatorIndex)
		cfg.ValidatorIndex = &idx
	}

	// Create node
	ctx := context.Background()
	n, err := node.New(ctx, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create node: %v\n", err)
		os.Exit(1)
	}

	// Start node
	n.Start()

	logger.Info("gean consensus client running",
		"slot", n.CurrentSlot(),
		"peers", n.PeerCount(),
	)

	// Wait for interrupt
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	logger.Info("shutting down...")
	n.Stop()
}
