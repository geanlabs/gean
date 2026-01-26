package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alecthomas/kong"
	"github.com/devylongs/gean/node"
)

var cli struct {
	GenesisTime    uint64   `help:"Genesis time (Unix timestamp). Defaults to 10 seconds from now."`
	Validators     uint64   `default:"8" help:"Number of validators in the network"`
	ValidatorIndex *uint64  `help:"Validator index to run as (optional, omit for non-validator)"`
	Listen         string   `default:"/ip4/0.0.0.0/udp/9000/quic-v1" help:"Listen multiaddr (QUIC)"`
	Bootnodes      []string `help:"Bootnode multiaddrs"`
	LogLevel       string   `default:"info" enum:"debug,info,warn,error" help:"Log level"`
}

func main() {
	kong.Parse(&cli,
		kong.Name("gean"),
		kong.Description("Lean Ethereum consensus client (Devnet 0)"),
	)

	fmt.Println("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ gean ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Setup logger
	level := slog.LevelInfo
	switch cli.LogLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))

	// Set genesis time
	genesisTime := cli.GenesisTime
	if genesisTime == 0 {
		genesisTime = uint64(time.Now().Unix()) + 10
		logger.Info("genesis time not set, using now + 10 seconds", "genesis_time", genesisTime)
	}

	// Build node config
	nodeCfg := &node.Config{
		GenesisTime:    genesisTime,
		ValidatorCount: cli.Validators,
		ValidatorIndex: cli.ValidatorIndex,
		ListenAddrs:    []string{cli.Listen},
		Bootnodes:      cli.Bootnodes,
		Logger:         logger,
	}

	if cli.ValidatorIndex != nil {
		logger.Info("running as validator", "index", *cli.ValidatorIndex)
	}

	logger.Info("config",
		"genesis_time", genesisTime,
		"validators", cli.Validators,
		"bootnodes", len(cli.Bootnodes),
	)

	// Create and start node
	ctx, cancel := context.WithCancel(context.Background())
	n, err := node.New(ctx, nodeCfg)
	if err != nil {
		logger.Error("failed to create node", "error", err)
		os.Exit(1)
	}

	n.Start()
	logger.Info("gean running", "slot", n.CurrentSlot(), "peers", n.PeerCount())

	// Wait for shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	logger.Info("shutting down...")
	n.Stop()
	cancel()
}
