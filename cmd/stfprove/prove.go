package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/geanlabs/gean/internal/zkstf"
)

func runProve(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("prove", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var backend backendFlags
	backend.register(fs)
	inputPath := fs.String("i", "", "framed input file (required)")
	outPath := fs.String("o", "", "proof file to write (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *inputPath == "" || *outPath == "" {
		return fmt.Errorf("%w: -i and -o are required", errUsage)
	}
	prover, err := backend.prover()
	if err != nil {
		return err
	}
	input, err := os.ReadFile(*inputPath)
	if err != nil {
		return err
	}
	start := time.Now()
	proof, err := prover.Prove(ctx, input)
	if err != nil {
		return err
	}
	elapsed := time.Since(start)
	b, err := proof.MarshalBinary()
	if err != nil {
		return err
	}
	if err := os.WriteFile(*outPath, b, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "%s vk=%x time=%s size=%d\n", proof.PublicValues, proof.VKHash, elapsed.Round(time.Millisecond), len(b))
	return nil
}

func runVerify(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var backend backendFlags
	backend.register(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("%w: usage: stfprove verify --zkvm <zkvm> [flags] <proof>", errUsage)
	}
	prover, err := backend.prover()
	if err != nil {
		return err
	}
	b, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		return err
	}
	var proof zkstf.Proof
	if err := proof.UnmarshalBinary(b); err != nil {
		return err
	}
	pv, err := prover.Verify(ctx, &proof)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "valid %s\n", pv)
	return nil
}
