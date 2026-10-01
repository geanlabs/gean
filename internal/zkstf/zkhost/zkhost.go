// Package zkhost runs, proves and verifies the state-transition guest through
// a zkVM host binary built from zk/<zkvm>host. Every host speaks the same
// command line:
//
//	<host> execute <elf> <input>          prints cycles=<n> pv=<hex>
//	<host> prove   <elf> <input> <proof>  prints vk=<hex> pv=<hex>
//	<host> verify  <elf> <proof>          prints vk=<hex> pv=<hex>
//
// and exits 3 when the guest fails and 4 when a proof is invalid.
package zkhost

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"github.com/geanlabs/gean/internal/zkstf"
)

// Prover runs ELF on VM through the host binary at Bin. When Exec is set,
// Execute uses it instead of the host, for a zkVM with a faster executor.
type Prover struct {
	VM   zkstf.ZKVM
	Bin  string
	ELF  string
	Exec func(ctx context.Context, input []byte) (*zkstf.ExecResult, error)
}

// A host's exit statuses for a guest that aborted or halted with a non-zero
// code, and for a proof that does not verify.
const (
	exitGuestFailed  = 3
	exitInvalidProof = 4
)

var (
	executeLine = regexp.MustCompile(`(?m)^cycles=(\d+) pv=([0-9a-f]*)$`)
	proofLine   = regexp.MustCompile(`(?m)^vk=([0-9a-f]{64}) pv=([0-9a-f]*)$`)
)

func (p Prover) ZKVM() zkstf.ZKVM { return p.VM }

func (p Prover) Execute(ctx context.Context, input []byte) (*zkstf.ExecResult, error) {
	if p.Exec != nil {
		return p.Exec(ctx, input)
	}
	dir, err := os.MkdirTemp("", "zkhost-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	inPath := filepath.Join(dir, "in.bin")
	if err := os.WriteFile(inPath, input, 0o600); err != nil {
		return nil, err
	}
	start := time.Now()
	out, err := p.run(ctx, "execute", p.ELF, inPath)
	elapsed := time.Since(start)
	if err != nil {
		return nil, err
	}
	m := executeLine.FindSubmatch(out)
	if m == nil {
		return nil, fmt.Errorf("zkhost: no result line in output: %s", bytes.TrimSpace(out))
	}
	cycles, err := strconv.ParseUint(string(m[1]), 10, 64)
	if err != nil {
		return nil, err
	}
	pv, err := parsePV(m[2])
	if err != nil {
		return nil, err
	}
	return &zkstf.ExecResult{PublicValues: pv, Cycles: cycles, Duration: elapsed}, nil
}

// Prove proves input. The host verifies the proof before writing it.
func (p Prover) Prove(ctx context.Context, input []byte) (*zkstf.Proof, error) {
	dir, err := os.MkdirTemp("", "zkhost-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	inPath, proofPath := filepath.Join(dir, "in.bin"), filepath.Join(dir, "proof.bin")
	if err := os.WriteFile(inPath, input, 0o600); err != nil {
		return nil, err
	}
	out, err := p.run(ctx, "prove", p.ELF, inPath, proofPath)
	if err != nil {
		return nil, err
	}
	vk, pv, err := parseProofLine(out)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(proofPath)
	if err != nil {
		return nil, err
	}
	return &zkstf.Proof{ZKVM: p.VM, VKHash: vk, PublicValues: pv, Data: data}, nil
}

// Verify checks proof against the verifying key of ELF, requiring exit code
// 0, and that the proof's metadata names this program and its public values.
func (p Prover) Verify(ctx context.Context, proof *zkstf.Proof) (zkstf.PublicValues, error) {
	if proof.ZKVM != p.VM {
		return zkstf.PublicValues{}, fmt.Errorf("%w: a %s proof, not %s", zkstf.ErrInvalidProof, proof.ZKVM, p.VM)
	}
	dir, err := os.MkdirTemp("", "zkhost-")
	if err != nil {
		return zkstf.PublicValues{}, err
	}
	defer os.RemoveAll(dir)
	proofPath := filepath.Join(dir, "proof.bin")
	if err := os.WriteFile(proofPath, proof.Data, 0o600); err != nil {
		return zkstf.PublicValues{}, err
	}
	out, err := p.run(ctx, "verify", p.ELF, proofPath)
	if err != nil {
		return zkstf.PublicValues{}, err
	}
	vk, pv, err := parseProofLine(out)
	if err != nil {
		return zkstf.PublicValues{}, err
	}
	if vk != proof.VKHash {
		return zkstf.PublicValues{}, fmt.Errorf("%w: proof names verifying key %x, program has %x", zkstf.ErrInvalidProof, proof.VKHash, vk)
	}
	if pv != proof.PublicValues {
		return zkstf.PublicValues{}, fmt.Errorf("%w: proof file public values differ from the proven ones", zkstf.ErrInvalidProof)
	}
	return pv, nil
}

// run runs the host and returns its stdout, mapping its guest-failure and
// invalid-proof exit statuses to the zkstf errors.
func (p Prover) run(ctx context.Context, args ...string) ([]byte, error) {
	if p.Bin == "" {
		return nil, fmt.Errorf("%w: no %s host binary (run make zk-host ZKVM=%s)", zkstf.ErrUnsupported, p.VM, p.VM)
	}
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, p.Bin, args...)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		switch exitErr.ExitCode() {
		case exitGuestFailed:
			return nil, fmt.Errorf("%w: %s", zkstf.ErrGuestFailed, bytes.TrimSpace(stderr.Bytes()))
		case exitInvalidProof:
			return nil, fmt.Errorf("%w: %s", zkstf.ErrInvalidProof, bytes.TrimSpace(stderr.Bytes()))
		}
	}
	if err != nil {
		return nil, fmt.Errorf("%s host %s: %w: %s", p.VM, args[0], err, bytes.TrimSpace(stderr.Bytes()))
	}
	return stdout.Bytes(), nil
}

func parseProofLine(out []byte) ([32]byte, zkstf.PublicValues, error) {
	var vk [32]byte
	m := proofLine.FindSubmatch(out)
	if m == nil {
		return vk, zkstf.PublicValues{}, fmt.Errorf("zkhost: no result line in output: %s", bytes.TrimSpace(out))
	}
	if _, err := hex.Decode(vk[:], m[1]); err != nil {
		return vk, zkstf.PublicValues{}, err
	}
	pv, err := parsePV(m[2])
	return vk, pv, err
}

func parsePV(h []byte) (zkstf.PublicValues, error) {
	b, err := hex.DecodeString(string(h))
	if err != nil {
		return zkstf.PublicValues{}, err
	}
	return zkstf.ParsePublicValues(b)
}
