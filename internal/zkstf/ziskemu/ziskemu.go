// Package ziskemu runs the state-transition guest on the ZisK emulator. It
// executes and counts steps but cannot prove.
package ziskemu

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"github.com/geanlabs/gean/internal/zkstf"
)

// Prover runs ELF under the ziskemu binary at Bin.
type Prover struct {
	Bin string
	ELF string
}

// ziskemu exits 0 even when the guest halts with an error; the run's outcome
// is only in its report.
var (
	errorLine = regexp.MustCompile(`finished with error at step=\d+ pc=(0x[0-9a-f]+)`)
	stepsLine = regexp.MustCompile(`process_rom\(\) steps=(\d+)`)
)

func (Prover) ZKVM() zkstf.ZKVM { return zkstf.ZisK }

func (p Prover) Execute(ctx context.Context, input []byte) (*zkstf.ExecResult, error) {
	dir, err := os.MkdirTemp("", "ziskemu-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	inPath, outPath := filepath.Join(dir, "in.bin"), filepath.Join(dir, "out.bin")
	if err := os.WriteFile(inPath, Frame(input), 0o600); err != nil {
		return nil, err
	}

	start := time.Now()
	cmd := exec.CommandContext(ctx, p.Bin, "-e", p.ELF, "-i", inPath, "-o", outPath, "-m")
	report, err := cmd.CombinedOutput()
	elapsed := time.Since(start)
	if err != nil {
		return nil, fmt.Errorf("ziskemu: %w: %s", err, bytes.TrimSpace(report))
	}
	if m := errorLine.FindSubmatch(report); m != nil {
		return nil, fmt.Errorf("%w: guest halted with an error at pc %s", zkstf.ErrGuestFailed, m[1])
	}
	m := stepsLine.FindSubmatch(report)
	if m == nil {
		return nil, fmt.Errorf("ziskemu: no step count in report: %s", bytes.TrimSpace(report))
	}
	steps, err := strconv.ParseUint(string(m[1]), 10, 64)
	if err != nil {
		return nil, err
	}
	out, err := os.ReadFile(outPath)
	if err != nil {
		return nil, fmt.Errorf("ziskemu output: %w", err)
	}
	pv, err := zkstf.ParsePublicValues(out)
	if err != nil {
		return nil, err
	}
	return &zkstf.ExecResult{PublicValues: pv, Cycles: steps, Duration: elapsed}, nil
}

func (Prover) Prove(context.Context, []byte) (*zkstf.Proof, error) {
	return nil, fmt.Errorf("%w: the ZisK emulator executes but does not prove", zkstf.ErrUnsupported)
}

func (Prover) Verify(context.Context, *zkstf.Proof) (zkstf.PublicValues, error) {
	return zkstf.PublicValues{}, fmt.Errorf("%w: the ZisK emulator executes but does not verify", zkstf.ErrUnsupported)
}

// Frame wraps an input in ZisK's stdin record: a u64 little-endian length
// followed by the bytes, zero-padded to a multiple of eight.
func Frame(input []byte) []byte {
	out := make([]byte, 8, 8+len(input)+7)
	binary.LittleEndian.PutUint64(out, uint64(len(input)))
	out = append(out, input...)
	for len(out)%8 != 0 {
		out = append(out, 0)
	}
	return out
}
