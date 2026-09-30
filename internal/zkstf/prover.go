package zkstf

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
	"time"
)

// ZKVM names a proving backend. Native runs the Go state transition directly
// and exists so every zkVM result can be compared against it.
type ZKVM string

const (
	Native ZKVM = "native"
	ZisK   ZKVM = "zisk"
	SP1    ZKVM = "sp1"
	OpenVM ZKVM = "openvm"
)

// ZKVMs lists every backend in the order they are supported.
var ZKVMs = []ZKVM{Native, ZisK, SP1, OpenVM}

func ParseZKVM(s string) (ZKVM, error) {
	if z := ZKVM(s); slices.Contains(ZKVMs, z) {
		return z, nil
	}
	return "", fmt.Errorf("unknown zkvm %q (want one of native, zisk, sp1, openvm)", s)
}

var (
	// ErrUnsupported is returned for an operation a backend cannot perform, such
	// as proving with the native backend or using a zkVM not built yet.
	ErrUnsupported = errors.New("zkstf: not supported")
	// ErrGuestFailed means the guest halted as a failure: the transition was
	// rejected, so no proof exists for the input.
	ErrGuestFailed = errors.New("zkstf: guest reported failure")
)

// ExecResult is the outcome of running the guest without proving.
type ExecResult struct {
	PublicValues PublicValues
	Cycles       uint64 // 0 when the backend does not count cycles
	Duration     time.Duration
}

// Prover runs, proves and verifies the state transition on one backend.
type Prover interface {
	ZKVM() ZKVM
	Execute(ctx context.Context, input []byte) (*ExecResult, error)
	Prove(ctx context.Context, input []byte) (*Proof, error)
	Verify(ctx context.Context, proof *Proof) (PublicValues, error)
}

// Proof is a backend proof plus the metadata needed to refuse it in the wrong
// context: the zkVM that made it, a hash of the program verifying key, and a
// hash of the input it was produced for.
type Proof struct {
	ZKVM         ZKVM
	VKHash       [32]byte
	InputHash    [32]byte
	PublicValues PublicValues
	Data         []byte
}

const (
	proofMagic   = "GSTP"
	proofVersion = 1
	maxProofData = 1 << 30
)

// InputHash identifies an input in proof metadata.
func InputHash(input []byte) [32]byte { return sha256.Sum256(input) }

// MarshalBinary encodes a proof file:
//
//	"GSTP" | u32 version | u8 len | zkvm | vk hash | input hash | public values | u64 len | data
func (p *Proof) MarshalBinary() ([]byte, error) {
	if len(p.ZKVM) == 0 || len(p.ZKVM) > 255 {
		return nil, fmt.Errorf("proof zkvm name %q", p.ZKVM)
	}
	pv := p.PublicValues.Bytes()
	out := make([]byte, 0, 4+4+1+len(p.ZKVM)+32+32+PublicValuesSize+8+len(p.Data))
	out = append(out, proofMagic...)
	out = binary.LittleEndian.AppendUint32(out, proofVersion)
	out = append(out, byte(len(p.ZKVM)))
	out = append(out, p.ZKVM...)
	out = append(out, p.VKHash[:]...)
	out = append(out, p.InputHash[:]...)
	out = append(out, pv[:]...)
	out = binary.LittleEndian.AppendUint64(out, uint64(len(p.Data)))
	out = append(out, p.Data...)
	return out, nil
}

func (p *Proof) UnmarshalBinary(b []byte) error {
	bad := func(what string) error { return fmt.Errorf("malformed proof file: %s", what) }
	if len(b) < 9 || string(b[:4]) != proofMagic {
		return bad("magic")
	}
	if v := binary.LittleEndian.Uint32(b[4:]); v != proofVersion {
		return bad(fmt.Sprintf("version %d", v))
	}
	n := int(b[8])
	b = b[9:]
	if len(b) < n+32+32+PublicValuesSize+8 {
		return bad("truncated header")
	}
	z, err := ParseZKVM(string(b[:n]))
	if err != nil {
		return bad(err.Error())
	}
	b = b[n:]
	var out Proof
	out.ZKVM = z
	copy(out.VKHash[:], b[:32])
	copy(out.InputHash[:], b[32:64])
	if out.PublicValues, err = ParsePublicValues(b[64 : 64+PublicValuesSize]); err != nil {
		return err
	}
	b = b[64+PublicValuesSize:]
	size := binary.LittleEndian.Uint64(b)
	b = b[8:]
	if size > maxProofData || size != uint64(len(b)) {
		return bad("data length")
	}
	out.Data = slices.Clone(b)
	*p = out
	return nil
}
