package types

import "math"

// IsJustifiableAfter checks if a slot is a valid candidate for justification
// after a given finalized slot according to 3SF-mini rules.
//
// A slot is justifiable if its distance (delta) from the finalized slot is:
//  1. Less than or equal to 5
//  2. A perfect square (e.g., 9, 16, 25...)
//  3. A pronic number n*(n+1) (e.g., 6, 12, 20, 30...)
func IsJustifiableAfter(slot, finalizedSlot uint64) bool {
	if slot < finalizedSlot {
		panic("candidate slot must not be before finalized slot")
	}

	delta := slot - finalizedSlot

	// Rule 1: first 5 slots always justifiable
	if delta <= 5 {
		return true
	}

	// Rule 2: perfect square
	s := isqrt(delta)
	if s*s == delta {
		return true
	}

	// Rule 3: pronic number n*(n+1)
	if s*(s+1) == delta {
		return true
	}

	return false
}

// isqrt returns the integer square root of n (floor(sqrt(n))).
func isqrt(n uint64) uint64 {
	if n == 0 {
		return 0
	}
	x := uint64(math.Sqrt(float64(n)))
	// Correct for float64 imprecision near large values.
	if (x+1)*(x+1) <= n {
		return x + 1
	}
	if x*x > n {
		return x - 1
	}
	return x
}
