package pending

import "sync"

// AttestationBuffer holds attestations whose head block is not imported yet,
// grouped by that head root and bounded per root and in total. T is the
// chain's attestation type.
type AttestationBuffer[T any] struct {
	mu sync.Mutex
	// slotOf returns an attestation's slot, or false for one too malformed to
	// hold.
	slotOf     func(T) (uint64, bool)
	byHead     map[[32]byte][]T
	perRootCap int
	totalCap   int
	total      int
	evicted    int
	rejected   int
}

// NewAttestationBuffer returns a buffer holding at most perRootCap
// attestations per head root and totalCap in all. slotOf returns an
// attestation's slot, or false for one too malformed to hold.
func NewAttestationBuffer[T any](perRootCap, totalCap int, slotOf func(T) (uint64, bool)) *AttestationBuffer[T] {
	return &AttestationBuffer[T]{
		slotOf:     slotOf,
		byHead:     make(map[[32]byte][]T),
		perRootCap: perRootCap,
		totalCap:   totalCap,
	}
}

func (b *AttestationBuffer[T]) Add(headRoot [32]byte, att T) (added bool, dropped int) {
	if b == nil {
		return false, 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	if _, ok := b.slotOf(att); b.perRootCap <= 0 || b.totalCap <= 0 || !ok {
		b.rejected++
		return false, 0
	}

	bucket := b.byHead[headRoot]

	if len(bucket) >= b.perRootCap {
		bucket = append(bucket[1:], att)
		b.byHead[headRoot] = bucket
		b.evicted++
		return true, 1
	}

	if b.total >= b.totalCap {
		b.rejected++
		return false, 0
	}

	b.byHead[headRoot] = append(bucket, att)
	b.total++
	return true, 0
}

func (b *AttestationBuffer[T]) Drain(headRoot [32]byte) []T {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	bucket, ok := b.byHead[headRoot]
	if !ok {
		return nil
	}
	delete(b.byHead, headRoot)
	b.total -= len(bucket)
	return bucket
}

func (b *AttestationBuffer[T]) PruneBelow(finalizedSlot uint64) int {
	if b == nil {
		return 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	removed := 0
	for headRoot, bucket := range b.byHead {
		kept := bucket[:0]
		for _, att := range bucket {
			if slot, ok := b.slotOf(att); !ok || slot <= finalizedSlot {
				removed++
				continue
			}
			kept = append(kept, att)
		}
		if len(kept) == 0 {
			delete(b.byHead, headRoot)
		} else {
			b.byHead[headRoot] = kept
		}
	}
	b.total -= removed
	return removed
}

func (b *AttestationBuffer[T]) Total() int {
	if b == nil {
		return 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.total
}

func (b *AttestationBuffer[T]) Len() int {
	if b == nil {
		return 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.byHead)
}

func (b *AttestationBuffer[T]) Stats() (evicted, rejected int) {
	if b == nil {
		return 0, 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.evicted, b.rejected
}
