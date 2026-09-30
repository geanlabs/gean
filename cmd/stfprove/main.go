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
	"os/exec"
	"os/signal"
	"path/filepath"

	"github.com/geanlabs/gean/internal/zkstf"
	"github.com/geanlabs/gean/internal/zkstf/ziskemu"
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
	zkvm    string
	backend string
	elf     string
	ziskemu string
}

func (b *backendFlags) register(fs *flag.FlagSet) {
	fs.StringVar(&b.zkvm, "zkvm", "", "backend: native, zisk, sp1 or openvm (required)")
	fs.StringVar(&b.backend, "backend", "ere", "how a zkVM runs: ere (prover server) or emu (ZisK emulator, execute only)")
	fs.StringVar(&b.elf, "elf", "", "guest ELF (default zk/out/stf-<zkvm>.elf)")
	fs.StringVar(&b.ziskemu, "ziskemu", envOr("ZISKEMU", "ziskemu"), "ziskemu binary for --backend emu (env ZISKEMU)")
}

func (b *backendFlags) prover() (zkstf.Prover, error) {
	if b.zkvm == "" {
		return nil, fmt.Errorf("%w: --zkvm is required (native, zisk, sp1 or openvm)", errUsage)
	}
	z, err := zkstf.ParseZKVM(b.zkvm)
	if err != nil {
		return nil, err
	}
	return b.newProver(z)
}

// newProver builds the backend for a zkVM.
func (b *backendFlags) newProver(z zkstf.ZKVM) (zkstf.Prover, error) {
	if z == zkstf.Native {
		return zkstf.NativeProver{}, nil
	}
	elf := b.elf
	if elf == "" {
		elf = filepath.Join("zk", "out", "stf-"+string(z)+".elf")
	}
	switch {
	case z == zkstf.ZisK && b.backend == "emu":
		if _, err := os.Stat(elf); err != nil {
			return nil, fmt.Errorf("zisk guest: %w (run make zk-guest ZKVM=zisk)", err)
		}
		bin, err := exec.LookPath(b.ziskemu)
		if err != nil {
			return nil, fmt.Errorf("ziskemu: %w", err)
		}
		return ziskemu.Prover{Bin: bin, ELF: elf}, nil
	case b.backend == "emu":
		return nil, fmt.Errorf("%w: only zisk has an emulator backend", zkstf.ErrUnsupported)
	case b.backend == "ere":
		return nil, fmt.Errorf("%w: %s proving through ere is not implemented yet", zkstf.ErrUnsupported, z)
	default:
		return nil, fmt.Errorf("%w: unknown --backend %q", errUsage, b.backend)
	}
}

func runZKVMs(stdout io.Writer) error {
	for _, z := range zkstf.ZKVMs {
		for _, backend := range []string{"emu", "ere"} {
			if z == zkstf.Native && backend == "ere" {
				continue
			}
			b := backendFlags{backend: backend, ziskemu: envOr("ZISKEMU", "ziskemu")}
			status := "available"
			if _, err := b.newProver(z); err != nil {
				status = err.Error()
			}
			name := string(z) + "/" + backend
			if z == zkstf.Native {
				name = string(z)
			}
			fmt.Fprintf(stdout, "%-12s %s\n", name, status)
		}
	}
	return nil
}
