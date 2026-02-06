package types

import (
	"fmt"
	"math"
)

// Primitive Types

type Slot uint64
type ValidatorIndex uint64
type Root [32]byte

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

// IsJustifiableAfter checks if this slot is a valid candidate for justification
// after the given finalized slot. Per 3SF-mini spec:
// - delta <= 5 (immediate)
// - delta is a perfect square (x^2)
// - delta is a pronic number (x^2 + x)
func (s Slot) IsJustifiableAfter(finalizedSlot Slot) bool {
	if s < finalizedSlot {
		return false
	}
	delta := int(s - finalizedSlot)
	if delta <= 5 {
		return true
	}
	// Check perfect square: isqrt(delta)^2 == delta
	sq := isqrt(delta)
	if sq*sq == delta {
		return true
	}
	// Check pronic number: 4*delta+1 is an odd perfect square
	v := 4*delta + 1
	sqv := isqrt(v)
	return sqv*sqv == v && sqv%2 == 1
}

const (
	SecondsPerSlot             uint64 = 4
	IntervalsPerSlot           uint64 = 4
	SecondsPerInterval         uint64 = SecondsPerSlot / IntervalsPerSlot
	JustificationLookbackSlots uint64 = 3
)
