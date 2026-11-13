package main

import (
	"errors"
	"flag"
	"os"

	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/lean/node"
)

func main() {
	cfg, err := parseConfig(os.Args[1:], os.Stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		os.Exit(1)
	}
	if err := node.Run(cfg, notifyShutdown()); err != nil {
		logger.Error(logger.Node, "fatal: %v", err)
		os.Exit(1)
	}
}
