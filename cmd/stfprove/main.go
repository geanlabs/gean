// Command stfprove checks the zkVM guests of gean's state transition (zk/) against
// the Go transition. It writes test vectors, runs them through a zkVM host,
// and proves and verifies single inputs.
//
// Every host built from zk/<zkvm>/host speaks one command line:
//
//	<host> execute <elf> <input>          prints cycles=<n> pv=<hex>
//	<host> prove   <elf> <input> <proof>  prints vk=<hex> pv=<hex>
//	<host> verify  <elf> <proof>          prints vk=<hex> pv=<hex>
//
// and exits 3 when the guest fails and 4 when a proof is invalid.
package main

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/geanlabs/gean/internal/statetransition"
	"github.com/geanlabs/gean/internal/zkstf"
	"github.com/geanlabs/gean/internal/zkstf/zkvectors"
)

const usage = `usage:
  stfprove vectors -o <dir>
  stfprove exec    -host <bin> -elf <elf> -vectors <dir>
  stfprove prove   -host <bin> -elf <elf> -i <input> -o <proof>
  stfprove verify  -host <bin> -elf <elf> <proof>`

// A host's exit statuses for a guest that failed and for an invalid proof.
const (
	exitGuestFailed  = 3
	exitInvalidProof = 4
)

// mutations is how many mutated inputs the vectors carry beyond the suite.
const mutations = 400

var resultLine = regexp.MustCompile(`(?m)^(cycles|vk)=([0-9a-f]+) pv=([0-9a-f]*)$`)

func main() {
	log.SetFlags(0)
	if len(os.Args) < 2 {
		log.Fatal(usage)
	}
	fs := flag.NewFlagSet(os.Args[1], flag.ExitOnError)
	host := fs.String("host", "", "zkVM host binary")
	elf := fs.String("elf", "", "guest ELF")
	out := fs.String("o", "", "output directory (vectors) or proof file (prove)")
	input := fs.String("i", "", "framed input")
	vectors := fs.String("vectors", "", "vectors directory")
	_ = fs.Parse(os.Args[2:])

	var err error
	switch os.Args[1] {
	case "vectors":
		err = writeVectors(*out)
	case "exec":
		err = execVectors(*host, *elf, *vectors)
	case "prove":
		err = prove(*host, *elf, *input, *out)
	case "verify":
		if fs.NArg() != 1 {
			log.Fatal(usage)
		}
		var pv zkstf.PublicValues
		if _, pv, err = run(*host, "verify", *elf, fs.Arg(0)); err == nil {
			fmt.Printf("valid %s\n", pv)
		}
	default:
		log.Fatal(usage)
	}
	if err != nil {
		log.Fatal(err)
	}
}

// writeVectors writes every generated input as <n>.gstf and a manifest with
// one line per input: "<file> <name> ok <public values>" or
// "<file> <name> err <kind> [<computed state root>]", as the Go transition
// decides.
func writeVectors(dir string) error {
	cases, err := zkvectors.Suite()
	if err != nil {
		return err
	}
	var inputs []zkvectors.Input
	for _, c := range cases {
		in, err := zkstf.NewInput(c.Pre, c.Block)
		if err != nil {
			return fmt.Errorf("%s: %w", c.Name, err)
		}
		inputs = append(inputs, zkvectors.Input{Name: c.Name, Bytes: in})
	}
	mutated, err := zkvectors.Mutations(cases, 1, mutations)
	if err != nil {
		return err
	}
	divergent, err := zkvectors.Divergences()
	if err != nil {
		return err
	}
	inputs = append(append(inputs, mutated...), divergent...)

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	var manifest strings.Builder
	for i, in := range inputs {
		file := fmt.Sprintf("%04d.gstf", i)
		if err := os.WriteFile(filepath.Join(dir, file), in.Bytes, 0o644); err != nil {
			return err
		}
		outcome, err := expect(in.Bytes)
		if err != nil {
			return fmt.Errorf("%s: %w", in.Name, err)
		}
		fmt.Fprintf(&manifest, "%s %s %s\n", file, in.Name, outcome)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.txt"), []byte(manifest.String()), 0o644); err != nil {
		return err
	}
	fmt.Printf("wrote %d vectors to %s\n", len(inputs), dir)
	return nil
}

// expect is the manifest outcome of one input under the Go transition.
func expect(in []byte) (string, error) {
	pv, err := zkstf.Apply(in)
	if err == nil {
		b := pv.Bytes()
		return "ok " + hex.EncodeToString(b[:]), nil
	}
	kind, kerr := zkstf.ErrorKind(err)
	if kerr != nil {
		return "", kerr
	}
	var mismatch *statetransition.StateRootMismatchError
	if errors.As(err, &mismatch) {
		return fmt.Sprintf("err %s %x", kind, mismatch.Computed), nil
	}
	return "err " + kind, nil
}

// execVectors runs every vector on the host and checks each outcome against
// the manifest: accepted inputs must reproduce the public values and
// rejected ones must fail in the guest.
func execVectors(host, elf, dir string) error {
	f, err := os.Open(filepath.Join(dir, "manifest.txt"))
	if err != nil {
		return err
	}
	defer f.Close()
	var total, failed int
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		file, name, want := fields[0], fields[1], fields[2]
		// Typed SSZ rejects the inputs on which fastssz is known to be laxer;
		// see zk/stf/tests/vectors.rs.
		if strings.HasPrefix(name, "divergence/") {
			want = "err"
		}
		_, pv, err := run(host, "execute", elf, filepath.Join(dir, file))
		verdict := "ok"
		switch {
		case want == "err" && !errors.Is(err, errGuestFailed):
			verdict = fmt.Sprintf("WRONG: want guest failure, got %v", err)
		case want == "ok" && err != nil:
			verdict = fmt.Sprintf("WRONG: %v", err)
		case want == "ok":
			b := pv.Bytes()
			if hex.EncodeToString(b[:]) != fields[3] {
				verdict = "WRONG: public values differ from Go"
			}
		}
		total++
		if verdict != "ok" {
			failed++
			fmt.Printf("%-60s %s\n", name, verdict)
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	fmt.Printf("%d/%d vectors match the Go transition\n", total-failed, total)
	if failed > 0 {
		return fmt.Errorf("%d vectors differ", failed)
	}
	return nil
}

// prove proves one input, verifies the proof and checks that the proven
// public values are the Go transition's.
func prove(host, elf, input, proof string) error {
	in, err := os.ReadFile(input)
	if err != nil {
		return err
	}
	want, err := zkstf.Apply(in)
	if err != nil {
		return fmt.Errorf("go transition rejects the input: %w", err)
	}
	start := time.Now()
	if _, _, err := run(host, "prove", elf, input, proof); err != nil {
		return err
	}
	elapsed := time.Since(start)
	vk, pv, err := run(host, "verify", elf, proof)
	if err != nil {
		return err
	}
	if pv != want {
		return fmt.Errorf("proven %s, Go computes %s", pv, want)
	}
	fmt.Printf("%s vk=%s time=%s\n", pv, vk, elapsed.Round(time.Millisecond))
	return nil
}

var (
	errGuestFailed  = errors.New("guest failed")
	errInvalidProof = errors.New("invalid proof")
)

// run runs one host command and parses its result line.
func run(host, cmd string, args ...string) (string, zkstf.PublicValues, error) {
	var stdout, stderr bytes.Buffer
	c := exec.Command(host, append([]string{cmd}, args...)...)
	c.Stdout, c.Stderr = &stdout, &stderr
	err := c.Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		switch exitErr.ExitCode() {
		case exitGuestFailed:
			return "", zkstf.PublicValues{}, fmt.Errorf("%w: %s", errGuestFailed, bytes.TrimSpace(stderr.Bytes()))
		case exitInvalidProof:
			return "", zkstf.PublicValues{}, fmt.Errorf("%w: %s", errInvalidProof, bytes.TrimSpace(stderr.Bytes()))
		}
	}
	if err != nil {
		return "", zkstf.PublicValues{}, fmt.Errorf("host %s: %w: %s", cmd, err, bytes.TrimSpace(stderr.Bytes()))
	}
	m := resultLine.FindSubmatch(stdout.Bytes())
	if m == nil {
		return "", zkstf.PublicValues{}, fmt.Errorf("host %s: no result line: %s", cmd, bytes.TrimSpace(stdout.Bytes()))
	}
	raw, err := hex.DecodeString(string(m[3]))
	if err != nil {
		return "", zkstf.PublicValues{}, err
	}
	pv, err := zkstf.ParsePublicValues(raw)
	return string(m[2]), pv, err
}
