package transition

import "math"

// SlotIsJustifiableAfter reports whether slot is a justification candidate after
// finalizedSlot: its distance is at most 5, a perfect square or a pronic number
// n(n+1). Integer roots keep it exact over all of uint64, as leanSpec's isqrt is.
func SlotIsJustifiableAfter(slot, finalizedSlot uint64) bool {
	if slot < finalizedSlot {
		return false
	}
	delta := slot - finalizedSlot
	if delta <= 5 {
		return true
	}
	// A pronic n(n+1) lies between n^2 and (n+1)^2, so its integer root is n too.
	root := isqrt(delta)
	return root*root == delta || root*(root+1) == delta
}

// isqrt returns the largest r with r*r <= n, for n > 0. Comparing by division
// keeps the correction from overflowing.
func isqrt(n uint64) uint64 {
	r := uint64(math.Sqrt(float64(n)))
	for r > n/r {
		r--
	}
	for r+1 <= n/(r+1) {
		r++
	}
	return r
}
