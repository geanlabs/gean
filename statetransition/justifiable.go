package statetransition

import "math"

// SlotIsJustifiableAfter returns true if slot can be justified given the finalized slot.
// 3SF-mini rules: distance must be <= 5, a perfect square, or a pronic number.
func SlotIsJustifiableAfter(slot, finalizedSlot uint64) bool {
	if slot <= finalizedSlot {
		return false
	}
	delta := slot - finalizedSlot

	// Rule 1: first 5 slots after finalization are always justifiable
	if delta <= 5 {
		return true
	}

	// Rule 2: perfect square (1, 4, 9, 16, 25, ...)
	sqrt := uint64(math.Sqrt(float64(delta)))
	if sqrt*sqrt == delta {
		return true
	}

	// Rule 3: pronic number n*(n+1) (2, 6, 12, 20, 30, ...)
	// Check: 4*delta + 1 is an odd perfect square
	val := 4*delta + 1
	sqrtVal := uint64(math.Sqrt(float64(val)))
	if sqrtVal*sqrtVal == val && sqrtVal%2 == 1 {
		return true
	}

	return false
}
