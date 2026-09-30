package main

import (
	"context"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/geanlabs/gean/internal/zkstf"
)

func runExecute(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("execute", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var backend backendFlags
	backend.register(fs)
	inputPath := fs.String("i", "", "framed input file")
	manifestPath := fs.String("manifest", "", "run every case in a vectors manifest instead of one input")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if (*inputPath == "") == (*manifestPath == "") {
		return fmt.Errorf("%w: give exactly one of -i or --manifest", errUsage)
	}
	prover, err := backend.prover()
	if err != nil {
		return err
	}

	if *inputPath != "" {
		input, err := os.ReadFile(*inputPath)
		if err != nil {
			return err
		}
		res, err := prover.Execute(ctx, input)
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "%s cycles=%d time=%s\n", res.PublicValues, res.Cycles, res.Duration)
		return nil
	}

	m, err := loadManifest(*manifestPath)
	if err != nil {
		return err
	}
	dir := filepath.Dir(*manifestPath)
	var failed int
	for _, c := range m.Cases {
		input, err := os.ReadFile(filepath.Join(dir, c.Input))
		if err != nil {
			return err
		}
		res, err := prover.Execute(ctx, input)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		verdict := checkCase(c, res, err)
		if verdict != "ok" {
			failed++
		}
		cycles := uint64(0)
		if res != nil {
			cycles = res.Cycles
		}
		fmt.Fprintf(stdout, "%-40s %-8s cycles=%d\n", c.Name, verdict, cycles)
	}
	fmt.Fprintf(stdout, "%s: %d/%d cases as expected\n", prover.ZKVM(), len(m.Cases)-failed, len(m.Cases))
	if failed > 0 {
		return fmt.Errorf("%d cases did not match", failed)
	}
	return nil
}

// checkCase compares one execution with the manifest: accept cases must
// reproduce the recorded public values, reject cases must fail in the guest.
func checkCase(c manifestCase, res *zkstf.ExecResult, err error) string {
	if c.WantErr {
		if errors.Is(err, zkstf.ErrGuestFailed) {
			return "ok"
		}
		if err != nil {
			return "ERROR: " + err.Error()
		}
		return "ACCEPTED"
	}
	if err != nil {
		return "ERROR: " + err.Error()
	}
	b := res.PublicValues.Bytes()
	if "0x"+hex.EncodeToString(b[:]) != c.PublicValues {
		return "MISMATCH"
	}
	return "ok"
}
