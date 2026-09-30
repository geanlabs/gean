package zkstf

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/geanlabs/gean/internal/types"
)

// Input layout (little endian):
//
//	"GSTF" | u32 version | u64 len | state SSZ | u64 len | block SSZ
//
// Each zkVM adds its own transport framing around this blob; the guest board
// strips it before handing the bytes to Apply.
const (
	inputMagic   = "GSTF"
	InputVersion = 1

	// MaxStateSSZ and MaxBlockSSZ bound what a guest will decode. Both sit well
	// above realistic sizes (a 4096-validator state with a full block-hash
	// history is ~9 MiB) while keeping a hostile input from exhausting memory.
	MaxStateSSZ = 64 << 20
	MaxBlockSSZ = 8 << 20

	inputHeaderLen = len(inputMagic) + 4
)

var ErrMalformedInput = errors.New("zkstf: malformed input")

// NewInput encodes a pre-state and an unsigned block.
func NewInput(state *types.State, block *types.Block) ([]byte, error) {
	if state == nil || block == nil {
		return nil, fmt.Errorf("%w: nil state or block", ErrMalformedInput)
	}
	stateSSZ, err := state.MarshalSSZ()
	if err != nil {
		return nil, fmt.Errorf("marshal state: %w", err)
	}
	blockSSZ, err := block.MarshalSSZ()
	if err != nil {
		return nil, fmt.Errorf("marshal block: %w", err)
	}
	return EncodeInput(stateSSZ, blockSSZ)
}

// EncodeInput frames already-encoded state and block SSZ.
func EncodeInput(stateSSZ, blockSSZ []byte) ([]byte, error) {
	if len(stateSSZ) > MaxStateSSZ {
		return nil, fmt.Errorf("%w: state is %d bytes, max %d", ErrMalformedInput, len(stateSSZ), MaxStateSSZ)
	}
	if len(blockSSZ) > MaxBlockSSZ {
		return nil, fmt.Errorf("%w: block is %d bytes, max %d", ErrMalformedInput, len(blockSSZ), MaxBlockSSZ)
	}
	out := make([]byte, 0, inputHeaderLen+16+len(stateSSZ)+len(blockSSZ))
	out = append(out, inputMagic...)
	out = binary.LittleEndian.AppendUint32(out, InputVersion)
	out = binary.LittleEndian.AppendUint64(out, uint64(len(stateSSZ)))
	out = append(out, stateSSZ...)
	out = binary.LittleEndian.AppendUint64(out, uint64(len(blockSSZ)))
	out = append(out, blockSSZ...)
	return out, nil
}

// DecodeInput splits a framed input into state and block SSZ. It rejects
// unknown versions, oversize sections and trailing bytes. The returned slices
// alias in.
func DecodeInput(in []byte) (stateSSZ, blockSSZ []byte, err error) {
	if len(in) < inputHeaderLen || string(in[:len(inputMagic)]) != inputMagic {
		return nil, nil, fmt.Errorf("%w: bad magic", ErrMalformedInput)
	}
	if v := binary.LittleEndian.Uint32(in[len(inputMagic):]); v != InputVersion {
		return nil, nil, fmt.Errorf("%w: version %d, want %d", ErrMalformedInput, v, InputVersion)
	}
	rest := in[inputHeaderLen:]
	if stateSSZ, rest, err = readSection(rest, MaxStateSSZ, "state"); err != nil {
		return nil, nil, err
	}
	if blockSSZ, rest, err = readSection(rest, MaxBlockSSZ, "block"); err != nil {
		return nil, nil, err
	}
	if len(rest) != 0 {
		return nil, nil, fmt.Errorf("%w: %d trailing bytes", ErrMalformedInput, len(rest))
	}
	return stateSSZ, blockSSZ, nil
}

func readSection(in []byte, limit uint64, name string) (section, rest []byte, err error) {
	if len(in) < 8 {
		return nil, nil, fmt.Errorf("%w: truncated %s length", ErrMalformedInput, name)
	}
	n := binary.LittleEndian.Uint64(in)
	in = in[8:]
	if n > limit {
		return nil, nil, fmt.Errorf("%w: %s is %d bytes, max %d", ErrMalformedInput, name, n, limit)
	}
	if n > uint64(len(in)) {
		return nil, nil, fmt.Errorf("%w: truncated %s", ErrMalformedInput, name)
	}
	return in[:n], in[n:], nil
}
