package node

import (
	"sync"

	"github.com/geanlabs/gean/types"
)

// PendingAttestationBuffer holds gossip attestations whose referenced head
// block is not yet in the store. When that head block later arrives, the
// engine drains the bucket and replays the attestations through the normal
// gossip path so signatures get verified against real state.
//
// Keyed by head.Root so draining on block arrival is O(1). Per-root and total
// caps bound memory under adversarial load; per-root overflow drops the
// oldest entry in the bucket (FIFO).
type PendingAttestationBuffer struct {
	mu         sync.Mutex
	byHead     map[[32]byte][]*types.SignedAttestation
	perRootCap int
	totalCap   int
	total      int
	evicted    int // cumulative per-root FIFO evictions
	rejected   int // cumulative total-cap rejections
}

// NewPendingAttestationBuffer constructs a buffer with the given caps.
// perRootCap bounds the depth of any single head-root bucket; totalCap bounds
// the sum across all buckets.
func NewPendingAttestationBuffer(perRootCap, totalCap int) *PendingAttestationBuffer {
	return &PendingAttestationBuffer{
		byHead:     make(map[[32]byte][]*types.SignedAttestation),
		perRootCap: perRootCap,
		totalCap:   totalCap,
	}
}

// Add buffers att under headRoot.
//
// Returns added=true when the attestation landed in the buffer. Returns
// dropped>0 when adding this entry evicted an older one in the same bucket
// (per-root FIFO overflow). Returns added=false when the total cap is hit
// and the attestation was rejected outright.
func (b *PendingAttestationBuffer) Add(headRoot [32]byte, att *types.SignedAttestation) (added bool, dropped int) {
	b.mu.Lock()
	defer b.mu.Unlock()

	bucket := b.byHead[headRoot]

	// Per-root overflow: drop oldest, append new. Total count is unchanged
	// (one in, one out), so the total-cap branch below does not apply.
	if len(bucket) >= b.perRootCap {
		bucket = append(bucket[1:], att)
		b.byHead[headRoot] = bucket
		b.evicted++
		return true, 1
	}

	// Total cap: reject new entries once the buffer is full.
	if b.total >= b.totalCap {
		b.rejected++
		return false, 0
	}

	b.byHead[headRoot] = append(bucket, att)
	b.total++
	return true, 0
}

// Drain atomically removes and returns the bucket for headRoot. Returns nil
// when no bucket exists.
func (b *PendingAttestationBuffer) Drain(headRoot [32]byte) []*types.SignedAttestation {
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

// PruneBelow drops every buffered attestation whose Data.Slot <= finalizedSlot.
// Empty buckets are removed. Returns the number of attestations removed.
func (b *PendingAttestationBuffer) PruneBelow(finalizedSlot uint64) int {
	b.mu.Lock()
	defer b.mu.Unlock()

	removed := 0
	for headRoot, bucket := range b.byHead {
		kept := bucket[:0]
		for _, att := range bucket {
			if att.Data.Slot <= finalizedSlot {
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

// Total returns the current total number of buffered attestations across all
// head-root buckets.
func (b *PendingAttestationBuffer) Total() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.total
}

// Len returns the current number of distinct head-root buckets.
func (b *PendingAttestationBuffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.byHead)
}

// Stats returns cumulative eviction (per-root FIFO overflow) and rejection
// (total cap exceeded) counters since construction.
func (b *PendingAttestationBuffer) Stats() (evicted, rejected int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.evicted, b.rejected
}
