package zkstf

import (
	"errors"
	"fmt"
)

// PublicValuesSize is the canonical size of the committed public values.
const PublicValuesSize = 96

// maxPaddedPublicValues is the largest zero-padded form a zkVM reports: ZisK
// and OpenVM expose a fixed 256-byte output region.
const maxPaddedPublicValues = 256

var ErrMalformedPublicValues = errors.New("zkstf: malformed public values")

// PublicValues is what a proof commits to. PreStateRoot and BlockRoot are
// recomputed by the guest, never read from the input, and PostStateRoot is the
// block's state root after the transition has checked it. A chain of proofs
// links because each PreStateRoot equals the parent's PostStateRoot.
type PublicValues struct {
	PreStateRoot  [32]byte
	BlockRoot     [32]byte
	PostStateRoot [32]byte
}

func (pv PublicValues) Bytes() [PublicValuesSize]byte {
	var out [PublicValuesSize]byte
	copy(out[0:32], pv.PreStateRoot[:])
	copy(out[32:64], pv.BlockRoot[:])
	copy(out[64:96], pv.PostStateRoot[:])
	return out
}

func (pv PublicValues) String() string {
	return fmt.Sprintf("pre=%x block=%x post=%x", pv.PreStateRoot, pv.BlockRoot, pv.PostStateRoot)
}

// ParsePublicValues accepts exactly 96 bytes, or a zkVM's zero-padded output
// region whose tail beyond 96 bytes is entirely zero. Anything else is
// rejected so a guest cannot smuggle extra committed data past the host.
func ParsePublicValues(b []byte) (PublicValues, error) {
	var pv PublicValues
	if len(b) < PublicValuesSize || len(b) > maxPaddedPublicValues {
		return pv, fmt.Errorf("%w: %d bytes", ErrMalformedPublicValues, len(b))
	}
	for i, c := range b[PublicValuesSize:] {
		if c != 0 {
			return pv, fmt.Errorf("%w: non-zero padding at byte %d", ErrMalformedPublicValues, PublicValuesSize+i)
		}
	}
	copy(pv.PreStateRoot[:], b[0:32])
	copy(pv.BlockRoot[:], b[32:64])
	copy(pv.PostStateRoot[:], b[64:96])
	return pv, nil
}
