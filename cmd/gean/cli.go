package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"

	"github.com/geanlabs/gean/internal/core/logger"
	"github.com/geanlabs/gean/internal/lean"
	"github.com/geanlabs/gean/internal/lean/keygen"
)

const usage = `Usage:
  gean lean [flags]          Run a Lean Ethereum consensus node
  gean lean keygen [flags]   Generate a local network's genesis config and keys

Run "gean lean --help" or "gean lean keygen --help" to see their flags.
`

// runCommand picks the subcommand from args and returns the process exit
// code.
func runCommand(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "help", "-h", "-help", "--help":
		fmt.Fprint(stdout, usage)
		return 0
	case "lean":
		if len(args) > 1 && args[1] == "keygen" {
			return runKeygen(args[2:], stderr)
		}
		return runLeanNode(args[1:], stderr)
	}
	fmt.Fprintf(stderr, "unknown command %q\n\n%s", args[0], usage)
	return 2
}

// runLeanNode runs the lean consensus node until it shuts down.
func runLeanNode(args []string, stderr io.Writer) int {
	cfg, err := parseConfig(args, stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stop := notifyShutdown()
	go func() {
		<-stop
		cancel()
	}()
	if err := lean.Run(ctx, cfg); err != nil {
		logger.Error(logger.Node, "fatal: %v", err)
		return 1
	}
	return 0
}

// runKeygen generates a local network's genesis config and keys.
func runKeygen(args []string, stderr io.Writer) int {
	if err := keygen.Run(args, stderr); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		log.Print(err)
		return 1
	}
	return 0
}
