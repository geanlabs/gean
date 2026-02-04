package types

import "fmt"

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
	// Check perfect square
	sqrt := int(float64(delta) + 0.5)
	for sqrt*sqrt > delta {
		sqrt--
	}
	if sqrt*sqrt == delta {
		return true
	}
	// Check pronic (x^2 + x = x*(x+1))
	for x := 1; x*(x+1) <= delta; x++ {
		if x*(x+1) == delta {
			return true
		}
	}
	return false
}

const (
	SecondsPerSlot             uint64 = 4
	IntervalsPerSlot           uint64 = 4
	SecondsPerInterval         uint64 = SecondsPerSlot / IntervalsPerSlot
	JustificationLookbackSlots uint64 = 3
)
