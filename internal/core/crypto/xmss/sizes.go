package xmss

import (
	"errors"
	"fmt"
	"math"
)

// ErrSlotOverflow is a slot past the 32 bits an XMSS signature binds.
var ErrSlotOverflow = errors.New("slot overflows uint32")

// slot32 narrows a slot to the 32 bits an XMSS signature binds. A slot of 2^32
// or more has no signature, and truncating it would bind a different slot.
func slot32(slot uint64) (uint32, error) {
	if slot > math.MaxUint32 {
		return 0, fmt.Errorf("%w: slot %d", ErrSlotOverflow, slot)
	}
	return uint32(slot), nil
}

// Wire sizes of the XMSS scheme gean signs and verifies with (leanVM).
const (
	// PubkeySize is the length of a serialized XMSS public key, in bytes.
	PubkeySize = 32
	// SignatureSize is the length of a serialized XMSS signature, in bytes.
	SignatureSize = 1208
)
