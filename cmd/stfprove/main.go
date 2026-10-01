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
	"strings"

	"github.com/geanlabs/gean/internal/zkstf"
	"github.com/geanlabs/gean/internal/zkstf/ziskemu"
	"github.com/geanlabs/gean/internal/zkstf/zkhost"
)

const usage = `usage: stfprove <command> [flags]

commands:
  vectors   write the generated test vectors as framed inputs plus a manifest
  execute   run inputs on a backend and check them against the manifest or native
  prove     prove one framed input and write a proof file
  verify    verify a proof file and print its public values
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
	case "execute":
		return runExecute(ctx, args, stdout, stderr)
	case "prove":
		return runProve(ctx, args, stdout, stderr)
	case "verify":
		return runVerify(ctx, args, stdout, stderr)
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
	elf     string
	ziskemu string
	host    string
}

func (b *backendFlags) register(fs *flag.FlagSet) {
	fs.StringVar(&b.zkvm, "zkvm", "", "backend: native, zisk, sp1 or openvm (required)")
	fs.StringVar(&b.elf, "elf", "", "guest ELF (default zk/out/stf-<zkvm>.elf)")
	fs.StringVar(&b.ziskemu, "ziskemu", envOr("ZISKEMU", "ziskemu"), "ziskemu binary, which executes for --zkvm zisk (env ZISKEMU)")
	fs.StringVar(&b.host, "host", "", "host binary built from zk/<zkvm>host (default env <ZKVM>HOST, e.g. SP1HOST, else <zkvm>host on PATH)")
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
	if _, err := os.Stat(elf); err != nil {
		return nil, fmt.Errorf("%s guest: %w (run make zk-guest ZKVM=%s)", z, err, z)
	}
	p := zkhost.Prover{VM: z, ELF: elf}
	// A missing host leaves Bin empty: execution may still work, and
	// proving reports how to build the host.
	p.Bin, _ = exec.LookPath(b.hostName(z))
	if z == zkstf.ZisK {
		bin, err := exec.LookPath(b.ziskemu)
		if err != nil {
			return nil, err
		}
		p.Exec = ziskemu.Executor{Bin: bin, ELF: elf}.Execute
	} else if p.Bin == "" {
		return nil, fmt.Errorf("no %s host: %s not found (run make zk-host ZKVM=%s)", z, b.hostName(z), z)
	}
	return p, nil
}

// hostName is the host binary for z: --host, else env <ZKVM>HOST, else
// <zkvm>host on PATH.
func (b *backendFlags) hostName(z zkstf.ZKVM) string {
	if b.host != "" {
		return b.host
	}
	return envOr(strings.ToUpper(string(z))+"HOST", string(z)+"host")
}

func runZKVMs(stdout io.Writer) error {
	b := backendFlags{ziskemu: envOr("ZISKEMU", "ziskemu")}
	for _, z := range zkstf.ZKVMs {
		status := "available"
		p, err := b.newProver(z)
		if err != nil {
			status = err.Error()
		} else if h, ok := p.(zkhost.Prover); ok && h.Bin == "" {
			status = "execute only: no " + b.hostName(z) + " to prove (run make zk-host ZKVM=" + string(z) + ")"
		}
		fmt.Fprintf(stdout, "%-8s %s\n", z, status)
	}
	return nil
}
