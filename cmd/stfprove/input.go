package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/geanlabs/gean/internal/types"
	"github.com/geanlabs/gean/internal/zkstf"
)

func runInput(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("input", flag.ContinueOnError)
	fs.SetOutput(stderr)
	statePath := fs.String("state", "", "pre-state SSZ file")
	blockPath := fs.String("block", "", "block SSZ file (Block or SignedBlock, see --signed)")
	signed := fs.Bool("signed", false, "the block file holds a SignedBlock")
	out := fs.String("o", "in.bin", "output file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *statePath == "" || *blockPath == "" {
		return fmt.Errorf("%w: --state and --block are required", errUsage)
	}

	stateSSZ, err := os.ReadFile(*statePath)
	if err != nil {
		return err
	}
	blockSSZ, err := os.ReadFile(*blockPath)
	if err != nil {
		return err
	}
	if *signed {
		var sb types.SignedBlock
		if err := sb.UnmarshalSSZ(blockSSZ); err != nil {
			return fmt.Errorf("decode signed block: %w", err)
		}
		if blockSSZ, err = sb.Block.MarshalSSZ(); err != nil {
			return err
		}
	}
	input, err := zkstf.EncodeInput(stateSSZ, blockSSZ)
	if err != nil {
		return err
	}
	return os.WriteFile(*out, input, 0o644)
}
