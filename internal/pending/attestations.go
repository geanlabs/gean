package pending

import (
	"sync"

	"github.com/geanlabs/gean/internal/types"
)

type AttestationBuffer struct {
	mu         sync.Mutex
	byHead     map[[32]byte][]*types.SignedAttestation
	perRootCap int
	totalCap   int
	total      int
	evicted    int
	rejected   int
}

func NewAttestationBuffer(perRootCap, totalCap int) *AttestationBuffer {
	return &AttestationBuffer{
		byHead:     make(map[[32]byte][]*types.SignedAttestation),
		perRootCap: perRootCap,
		totalCap:   totalCap,
	}
}

func (b *AttestationBuffer) Add(headRoot [32]byte, att *types.SignedAttestation) (added bool, dropped int) {
	if b == nil {
		return false, 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.perRootCap <= 0 || b.totalCap <= 0 || att == nil || att.Data == nil {
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

func (b *AttestationBuffer) Drain(headRoot [32]byte) []*types.SignedAttestation {
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

func (b *AttestationBuffer) PruneBelow(finalizedSlot uint64) int {
	if b == nil {
		return 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	removed := 0
	for headRoot, bucket := range b.byHead {
		kept := bucket[:0]
		for _, att := range bucket {
			if att == nil || att.Data == nil || att.Data.Slot <= finalizedSlot {
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

func (b *AttestationBuffer) Total() int {
	if b == nil {
		return 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.total
}

func (b *AttestationBuffer) Len() int {
	if b == nil {
		return 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.byHead)
}

func (b *AttestationBuffer) Stats() (evicted, rejected int) {
	if b == nil {
		return 0, 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.evicted, b.rejected
}
