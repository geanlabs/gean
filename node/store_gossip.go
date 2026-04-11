package node

import (
	"unsafe"

	"github.com/geanlabs/gean/types"
)

// FreeSignatureFunc is set by the engine to provide C handle cleanup.
// AggregateMetricsFunc is set by the engine to record aggregation metrics.
// These avoid importing engine/crypto from store (no circular deps).
var FreeSignatureFunc func(unsafe.Pointer)
var AggregateMetricsFunc func(durationSeconds float64, numAttestations int)

// GossipSignatureEntry holds one validator's signature for aggregation.
type GossipSignatureEntry struct {
	ValidatorID uint64
	Signature   [types.SignatureSize]byte
	// SigHandle is an opaque C pointer to the parsed leansig Signature.
	// Kept alive to avoid SSZ round-trip corruption during aggregation.
	SigHandle unsafe.Pointer
}

// GossipDataEntry groups signatures by attestation data.
type GossipDataEntry struct {
	Data       *types.AttestationData
	Signatures []GossipSignatureEntry
}

// GossipSignatureMap maps data_root -> signatures.
type GossipSignatureMap map[[32]byte]*GossipDataEntry

// Insert adds a gossip signature for a validator (without C handle).
func (m GossipSignatureMap) Insert(dataRoot [32]byte, data *types.AttestationData, validatorID uint64, sig [types.SignatureSize]byte) {
	m.InsertWithHandle(dataRoot, data, validatorID, sig, nil, nil)
}

// InsertWithHandle adds a gossip signature with an optional opaque C handle.
// Deduplicates by validator ID per data root to prevent duplicate entries
// when the same attestation arrives from multiple gossip paths.
func (m GossipSignatureMap) InsertWithHandle(dataRoot [32]byte, data *types.AttestationData, validatorID uint64, sig [types.SignatureSize]byte, handle unsafe.Pointer, parseErr error) {
	entry, ok := m[dataRoot]
	if !ok {
		entry = &GossipDataEntry{Data: data}
		m[dataRoot] = entry
	}

	// Skip if this validator already has a signature for this data root.
	for _, existing := range entry.Signatures {
		if existing.ValidatorID == validatorID {
			return
		}
	}

	var h unsafe.Pointer
	if parseErr == nil {
		h = handle
	}
	entry.Signatures = append(entry.Signatures, GossipSignatureEntry{
		ValidatorID: validatorID,
		Signature:   sig,
		SigHandle:   h,
	})
}

// Delete removes specific (validatorID, dataRoot) entries, freeing C handles.
func (m GossipSignatureMap) Delete(keys []GossipDeleteKey) {
	for _, key := range keys {
		entry, ok := m[key.DataRoot]
		if !ok {
			continue
		}
		filtered := entry.Signatures[:0]
		for _, sig := range entry.Signatures {
			if sig.ValidatorID == key.ValidatorID {
				// Free C handle if present.
				if sig.SigHandle != nil && FreeSignatureFunc != nil {
					FreeSignatureFunc(sig.SigHandle)
				}
			} else {
				filtered = append(filtered, sig)
			}
		}
		entry.Signatures = filtered
		if len(entry.Signatures) == 0 {
			delete(m, key.DataRoot)
		}
	}
}

// PruneBelow removes entries with slot <= finalizedSlot, freeing C handles.
func (m GossipSignatureMap) PruneBelow(finalizedSlot uint64) int {
	pruned := 0
	for root, entry := range m {
		if entry.Data.Slot <= finalizedSlot {
			for _, sig := range entry.Signatures {
				if sig.SigHandle != nil && FreeSignatureFunc != nil {
					FreeSignatureFunc(sig.SigHandle)
				}
			}
			delete(m, root)
			pruned++
		}
	}
	return pruned
}

// PruneStaleSigs removes entries older than the given slot cutoff.
// Prevents unbounded growth on non-aggregator nodes.
func (m GossipSignatureMap) PruneStaleSigs(currentSlot uint64, maxAge uint64) int {
	if currentSlot < maxAge {
		return 0
	}
	cutoff := currentSlot - maxAge
	pruned := 0
	for root, entry := range m {
		if entry.Data.Slot < cutoff {
			for _, sig := range entry.Signatures {
				if sig.SigHandle != nil && FreeSignatureFunc != nil {
					FreeSignatureFunc(sig.SigHandle)
				}
			}
			delete(m, root)
			pruned++
		}
	}
	return pruned
}

// Len returns the number of data entries.
func (m GossipSignatureMap) Len() int {
	return len(m)
}

// GossipDeleteKey identifies a specific signature to delete.
type GossipDeleteKey struct {
	ValidatorID uint64
	DataRoot    [32]byte
}
