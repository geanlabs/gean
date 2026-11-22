package transition

import "math"

// SlotIsJustifiableAfter reports whether slot is a justification candidate after
// finalizedSlot: its distance is at most 5, a perfect square or a pronic number
// n(n+1). The checks are exact integer arithmetic, as leanSpec's isqrt is; a
// float square root misses squares past 2^52, and 4*delta+1 overflows.
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

// isqrt returns the largest r with r*r <= n.
func isqrt(n uint64) uint64 {
	r := min(uint64(math.Sqrt(float64(n))), math.MaxUint32)
	for r*r > n {
		r--
	}
	for r < math.MaxUint32 && (r+1)*(r+1) <= n {
		r++
	}
	return r
}
