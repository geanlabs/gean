package storage

import "encoding/binary"

// Metadata keys matching ethlambda store.rs L62-72.
var (
	KeyTime             = []byte("time")
	KeyConfig           = []byte("config")
	KeyHead             = []byte("head")
	KeySafeTarget       = []byte("safe_target")
	KeyLatestJustified  = []byte("latest_justified")
	KeyLatestFinalized  = []byte("latest_finalized")
)

// Retention constants matching ethlambda store.rs L75-78.
const (
	BlocksToKeep = 21_600 // ~1 day at 4s slots
	StatesToKeep = 3_000  // ~3.3 hours at 4s slots
)

// EncodeLiveChainKey encodes a LiveChain key: slot (8 bytes big-endian) || root (32 bytes).
// Big-endian ensures lexicographic ordering matches numeric ordering.
// Matches ethlambda store.rs L232-236.
func EncodeLiveChainKey(slot uint64, root [32]byte) []byte {
	key := make([]byte, 8+32)
	binary.BigEndian.PutUint64(key[:8], slot)
	copy(key[8:], root[:])
	return key
}

// DecodeLiveChainKey decodes a LiveChain key into (slot, root).
// Matches ethlambda store.rs L239-243.
func DecodeLiveChainKey(key []byte) (uint64, [32]byte) {
	slot := binary.BigEndian.Uint64(key[:8])
	var root [32]byte
	copy(root[:], key[8:])
	return slot, root
}
