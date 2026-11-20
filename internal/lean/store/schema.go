package store

import (
	"encoding/binary"

	"github.com/geanlabs/gean/internal/core/storage"
)

// Lean's tables. The names are part of the on-disk layout.
const (
	TableBlockHeaders storage.Table = "block_headers"
	TableBlockBodies  storage.Table = "block_bodies"
	TableSignedBlocks storage.Table = "signed_blocks"
	TableStates       storage.Table = "states"
	TableMetadata     storage.Table = "metadata"
	TableLiveChain    storage.Table = "live_chain"
)

var AllTables = []storage.Table{
	TableBlockHeaders,
	TableBlockBodies,
	TableSignedBlocks,
	TableStates,
	TableMetadata,
	TableLiveChain,
}

// Keys in TableMetadata.
var (
	keyTime            = []byte("time")
	keyConfig          = []byte("config")
	KeyHead            = []byte("head")
	keySafeTarget      = []byte("safe_target")
	keyLatestJustified = []byte("latest_justified")
	keyLatestFinalized = []byte("latest_finalized")
)

// liveChainKeySize is the length of a TableLiveChain key: a big-endian slot, so
// keys sort by slot, then the block root.
const liveChainKeySize = 40

func encodeLiveChainKey(slot uint64, root [32]byte) []byte {
	key := make([]byte, liveChainKeySize)
	binary.BigEndian.PutUint64(key[:8], slot)
	copy(key[8:liveChainKeySize], root[:])
	return key
}

// decodeLiveChainKey reports false for a key too short to hold a slot and root.
func decodeLiveChainKey(key []byte) (uint64, [32]byte, bool) {
	if len(key) < liveChainKeySize {
		return 0, [32]byte{}, false
	}
	slot := binary.BigEndian.Uint64(key[:8])
	var root [32]byte
	copy(root[:], key[8:liveChainKeySize])
	return slot, root, true
}
