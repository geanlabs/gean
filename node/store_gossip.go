package node

import (
	"sync"
	"unsafe"

	"github.com/geanlabs/gean/types"
)

// FreeSignatureFunc is set by the engine to provide C handle cleanup.
// AggregateMetricsFunc is set by the engine to record aggregation metrics.
// These avoid importing engine/crypto from store (no circular deps).
var FreeSignatureFunc func(unsafe.Pointer)
var AggregateMetricsFunc func(durationSeconds float64, numAttestations int)

// AttestationSignatureEntry holds one validator's signature for aggregation.
type AttestationSignatureEntry struct {
	ValidatorID uint64
	Signature   [types.SignatureSize]byte
	// SigHandle is an opaque C pointer to the parsed leansig Signature.
	// Kept alive to avoid SSZ round-trip corruption during aggregation.
	SigHandle unsafe.Pointer
}

// AttestationDataEntry groups signatures by attestation data.
type AttestationDataEntry struct {
	Data       *types.AttestationData
	Signatures []AttestationSignatureEntry
}

// AttestationSignatureMap is a thread-safe map of data_root -> signatures.
// Gossip attestations are verified and inserted concurrently from goroutines
// (matching zeam's inline processing model), while the aggregator reads
// from the main event loop at interval 2.
type AttestationSignatureMap struct {
	mu   sync.Mutex
	data map[[32]byte]*AttestationDataEntry
}

func NewAttestationSignatureMap() AttestationSignatureMap {
	return AttestationSignatureMap{data: make(map[[32]byte]*AttestationDataEntry)}
}

// Insert adds an attestation signature for a validator (without C handle).
func (m *AttestationSignatureMap) Insert(dataRoot [32]byte, data *types.AttestationData, validatorID uint64, sig [types.SignatureSize]byte) {
	m.InsertWithHandle(dataRoot, data, validatorID, sig, nil, nil)
}

// InsertWithHandle adds an attestation signature with an optional opaque C handle.
func (m *AttestationSignatureMap) InsertWithHandle(dataRoot [32]byte, data *types.AttestationData, validatorID uint64, sig [types.SignatureSize]byte, handle unsafe.Pointer, parseErr error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry, ok := m.data[dataRoot]
	if !ok {
		entry = &AttestationDataEntry{Data: data}
		m.data[dataRoot] = entry
	}
	var h unsafe.Pointer
	if parseErr == nil {
		h = handle
	}
	entry.Signatures = append(entry.Signatures, AttestationSignatureEntry{
		ValidatorID: validatorID,
		Signature:   sig,
		SigHandle:   h,
	})
}

// Delete removes specific (validatorID, dataRoot) entries, freeing C handles.
func (m *AttestationSignatureMap) Delete(keys []AttestationDeleteKey) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, key := range keys {
		entry, ok := m.data[key.DataRoot]
		if !ok {
			continue
		}
		filtered := entry.Signatures[:0]
		for _, sig := range entry.Signatures {
			if sig.ValidatorID == key.ValidatorID {
				if sig.SigHandle != nil && FreeSignatureFunc != nil {
					FreeSignatureFunc(sig.SigHandle)
				}
			} else {
				filtered = append(filtered, sig)
			}
		}
		entry.Signatures = filtered
		if len(entry.Signatures) == 0 {
			delete(m.data, key.DataRoot)
		}
	}
}

// PruneBelow removes entries with slot <= finalizedSlot, freeing C handles.
func (m *AttestationSignatureMap) PruneBelow(finalizedSlot uint64) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	pruned := 0
	for root, entry := range m.data {
		if entry.Data.Slot <= finalizedSlot {
			for _, sig := range entry.Signatures {
				if sig.SigHandle != nil && FreeSignatureFunc != nil {
					FreeSignatureFunc(sig.SigHandle)
				}
			}
			delete(m.data, root)
			pruned++
		}
	}
	return pruned
}

// Len returns the number of data entries.
func (m *AttestationSignatureMap) Len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.data)
}

// Snapshot returns a copy of the internal map for iteration.
// Used by the aggregator to read without holding the lock during slow ZK proving.
func (m *AttestationSignatureMap) Snapshot() map[[32]byte]*AttestationDataEntry {
	m.mu.Lock()
	defer m.mu.Unlock()
	snap := make(map[[32]byte]*AttestationDataEntry, len(m.data))
	for k, v := range m.data {
		snap[k] = v
	}
	return snap
}

// AttestationDeleteKey identifies a specific signature to delete.
type AttestationDeleteKey struct {
	ValidatorID uint64
	DataRoot    [32]byte
}
