package node

import (
	"github.com/geanlabs/gean/types"
)

// PayloadEntry stores attestation data + proofs for a single data_root.
type PayloadEntry struct {
	Data   *types.AttestationData
	Proofs []*types.AggregatedSignatureProof
}

// PayloadBuffer is a capped FIFO buffer for aggregated payloads.
type PayloadBuffer struct {
	data        map[[32]byte]*PayloadEntry // data_root -> entry
	order       [][32]byte                 // insertion order for FIFO eviction
	capacity    int
	totalProofs int
}

// NewPayloadBuffer creates a new buffer with the given capacity.
func NewPayloadBuffer(capacity int) *PayloadBuffer {
	return &PayloadBuffer{
		data:     make(map[[32]byte]*PayloadEntry),
		capacity: capacity,
	}
}

// Push inserts a proof for an attestation, FIFO-evicting when over capacity.
func (pb *PayloadBuffer) Push(dataRoot [32]byte, attData *types.AttestationData, proof *types.AggregatedSignatureProof) {
	if entry, ok := pb.data[dataRoot]; ok {
		// Skip duplicate proofs (same participants)
		for _, existing := range entry.Proofs {
			if bitlistEqual(existing.Participants, proof.Participants) {
				return
			}
		}
		entry.Proofs = append(entry.Proofs, proof)
		pb.totalProofs++
	} else {
		pb.data[dataRoot] = &PayloadEntry{
			Data:   attData,
			Proofs: []*types.AggregatedSignatureProof{proof},
		}
		pb.order = append(pb.order, dataRoot)
		pb.totalProofs++
	}

	// Evict oldest until under capacity.
	for pb.totalProofs > pb.capacity && len(pb.order) > 0 {
		evicted := pb.order[0]
		pb.order = pb.order[1:]
		if entry, ok := pb.data[evicted]; ok {
			pb.totalProofs -= len(entry.Proofs)
			delete(pb.data, evicted)
		}
	}
}

// PushBatch inserts multiple entries.
func (pb *PayloadBuffer) PushBatch(entries []PayloadKV) {
	for _, e := range entries {
		pb.Push(e.DataRoot, e.Data, e.Proof)
	}
}

// Drain takes all entries, leaving the buffer empty.
func (pb *PayloadBuffer) Drain() []PayloadKV {
	result := make([]PayloadKV, 0, pb.totalProofs)
	for _, dataRoot := range pb.order {
		entry := pb.data[dataRoot]
		for _, proof := range entry.Proofs {
			result = append(result, PayloadKV{
				DataRoot: dataRoot,
				Data:     entry.Data,
				Proof:    proof,
			})
		}
	}
	pb.data = make(map[[32]byte]*PayloadEntry)
	pb.order = nil
	pb.totalProofs = 0
	return result
}

// Len returns the number of distinct attestation data entries.
func (pb *PayloadBuffer) Len() int {
	return len(pb.data)
}

// TotalProofs returns the total number of proofs across all entries.
func (pb *PayloadBuffer) TotalProofs() int {
	return pb.totalProofs
}

// ExtractLatestAttestations returns per-validator latest attestations from participation bits.
func (pb *PayloadBuffer) ExtractLatestAttestations() map[uint64]*types.AttestationData {
	result := make(map[uint64]*types.AttestationData)
	for _, entry := range pb.data {
		for _, proof := range entry.Proofs {
			participantLen := types.BitlistLen(proof.Participants)
			for vid := uint64(0); vid < participantLen; vid++ {
				if types.BitlistGet(proof.Participants, vid) {
					existing, ok := result[vid]
					if !ok || existing.Slot < entry.Data.Slot {
						result[vid] = entry.Data
					}
				}
			}
		}
	}
	return result
}

// Entries returns all (data_root, data, proofs) for block building.
func (pb *PayloadBuffer) Entries() map[[32]byte]*PayloadEntry {
	return pb.data
}

// PayloadKV is a flattened (data_root, data, proof) tuple.
type PayloadKV struct {
	DataRoot [32]byte
	Data     *types.AttestationData
	Proof    *types.AggregatedSignatureProof
}

func bitlistEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
