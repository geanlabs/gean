package xmss

import (
	"fmt"
	"math"
)

// Slot is the 32-bit slot an XMSS signature is bound to. A slot of 2^32 or more
// has no signature, and truncating it would bind a different slot.
func Slot(slot uint64) (uint32, error) {
	if slot > math.MaxUint32 {
		return 0, fmt.Errorf("slot %d overflows uint32", slot)
	}
	return uint32(slot), nil
}

// Wire sizes of the XMSS scheme gean signs and verifies with (leanVM). Code
// that stores keys or signatures in fixed-size fields, such as lean's SSZ
// types, must use the same sizes; lean checks this at compile time.
const (
	// PubkeySize is the length of a serialized XMSS public key, in bytes.
	PubkeySize = 32
	// SignatureSize is the length of a serialized XMSS signature, in bytes.
	SignatureSize = 1208
)
