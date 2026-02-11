// Package types defines the primitive and composite types for Lean Ethereum consensus.
package types

import (
	"fmt"
	"math"
)

// Primitive types.
type Slot uint64
type ValidatorIndex uint64
type Root [32]byte

// Pubkey is a 52-byte XMSS public key.
type Pubkey [52]byte

// Signature is a 3112-byte XMSS signature container (devnet1 interop wire target).
// Pinned leanSpec raw XMSS payload sizes are 412 bytes (test) / 3100 bytes (prod).
// Note: leanSpec's container type is Bytes3116, so fixture adapters must normalize.
type Signature [3112]byte

func (r Root) IsZero() bool { return r == Root{} }

// Short returns a short hex representation of the root (first 4 bytes).
func (r Root) Short() string {
	return fmt.Sprintf("%x", r[:4])
}

// Compare compares two roots lexicographically.
// Returns 1 if r > other, -1 if r < other, 0 if equal.
func (r Root) Compare(other Root) int {
	for i := 0; i < 32; i++ {
		if r[i] > other[i] {
			return 1
		}
		if r[i] < other[i] {
			return -1
		}
	}
	return 0
}

// isqrt returns the integer square root of n.
func isqrt(n int) int {
	if n <= 0 {
		return 0
	}
	x := int(math.Sqrt(float64(n)))
	// Adjust for float imprecision
	if (x+1)*(x+1) <= n {
		x++
	}
	if x*x > n {
		x--
	}
	return x
}

// IsJustifiableAfter checks if this slot is a valid justification candidate
// after the given finalized slot. A slot is justifiable if its distance (delta)
// from the finalized slot is <= 5, a perfect square, or a pronic number (x*(x+1)).
// Unjustifiable slots funnel votes toward fewer targets to help reach finalization.
func (s Slot) IsJustifiableAfter(finalizedSlot Slot) bool {
	if s < finalizedSlot {
		return false
	}
	delta := int(s - finalizedSlot)
	if delta <= 5 {
		return true
	}
	// Rule 2: perfect square check
	sq := isqrt(delta)
	if sq*sq == delta {
		return true
	}
	// Rule 3: pronic number check — 4*delta+1 must be an odd perfect square
	v := 4*delta + 1
	sqv := isqrt(v)
	return sqv*sqv == v && sqv%2 == 1
}

// Protocol constants.
const (
	SecondsPerSlot             uint64 = 4 // SECONDS_PER_SLOT: 4-second block times
	IntervalsPerSlot           uint64 = 4 // INTERVALS_PER_SLOT: 4 intervals per slot (propose, vote, safe target, accept)
	SecondsPerInterval         uint64 = SecondsPerSlot / IntervalsPerSlot
	JustificationLookbackSlots uint64 = 3 // JUSTIFICATION_LOOKBACK_SLOTS: used for seen_ttl calculation
)
