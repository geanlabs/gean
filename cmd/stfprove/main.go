// Command stfprove runs, and on zkVM backends proves, gean's state transition
// outside the node: over generated vectors, over framed input files, or over
// blocks replayed from a running node.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"

	"github.com/geanlabs/gean/internal/zkstf"
)

const usage = `usage: stfprove <command> [flags]

commands:
  vectors   write the generated test vectors as framed inputs plus a manifest
  input     frame a pre-state and block (SSZ files) into a guest input
  execute   run inputs on a backend and check them against the manifest or native
  replay    fetch blocks from a node and run them, checking every root
  zkvms     list backends and whether each is available

Every run names its backend with --zkvm native|zisk|sp1|openvm.
Run "stfprove <command> -h" for a command's flags.
`

var errUsage = errors.New("usage")

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(2)
		}
		if errors.Is(err, errUsage) {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		log.Fatal(err)
	}
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return fmt.Errorf("%w: no command", errUsage)
	}
	cmd, args := args[0], args[1:]
	switch cmd {
	case "vectors":
		return runVectors(args, stdout, stderr)
	case "input":
		return runInput(args, stderr)
	case "execute":
		return runExecute(ctx, args, stdout, stderr)
	case "replay":
		return runReplay(ctx, args, stdout, stderr)
	case "zkvms":
		return runZKVMs(stdout)
	default:
		fmt.Fprint(stderr, usage)
		return fmt.Errorf("%w: unknown command %q", errUsage, cmd)
	}
}

// backendFlags selects the proving backend. The zkVM is always explicit so a
// run can never silently use a different backend than intended.
type backendFlags struct {
	zkvm string
}

func (b *backendFlags) register(fs *flag.FlagSet) {
	fs.StringVar(&b.zkvm, "zkvm", "", "backend: native, zisk, sp1 or openvm (required)")
}

func (b *backendFlags) prover() (zkstf.Prover, error) {
	if b.zkvm == "" {
		return nil, fmt.Errorf("%w: --zkvm is required (native, zisk, sp1 or openvm)", errUsage)
	}
	z, err := zkstf.ParseZKVM(b.zkvm)
	if err != nil {
		return nil, err
	}
	return newProver(z)
}

// newProver builds the backend for a zkVM.
func newProver(z zkstf.ZKVM) (zkstf.Prover, error) {
	switch z {
	case zkstf.Native:
		return zkstf.NativeProver{}, nil
	default:
		return nil, fmt.Errorf("%w: %s guest is not built yet", zkstf.ErrUnsupported, z)
	}
}

func runZKVMs(stdout io.Writer) error {
	for _, z := range zkstf.ZKVMs {
		status := "available"
		if _, err := newProver(z); err != nil {
			status = err.Error()
		}
		fmt.Fprintf(stdout, "%-7s %s\n", z, status)
	}
	return nil
}
