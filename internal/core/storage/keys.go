package storage

import "encoding/binary"

var (
	KeyTime            = []byte("time")
	KeyConfig          = []byte("config")
	KeyHead            = []byte("head")
	KeySafeTarget      = []byte("safe_target")
	KeyLatestJustified = []byte("latest_justified")
	KeyLatestFinalized = []byte("latest_finalized")
)

const (
	LiveChainKeySize = 40
	BlocksToKeep     = 21_600
	StatesToKeep     = 3_000
)

func EncodeLiveChainKey(slot uint64, root [32]byte) []byte {
	key := make([]byte, LiveChainKeySize)
	binary.BigEndian.PutUint64(key[:8], slot)
	copy(key[8:LiveChainKeySize], root[:])
	return key
}

func DecodeLiveChainKey(key []byte) (uint64, [32]byte) {
	if len(key) < LiveChainKeySize {
		return 0, [32]byte{}
	}
	slot := binary.BigEndian.Uint64(key[:8])
	var root [32]byte
	copy(root[:], key[8:LiveChainKeySize])
	return slot, root
}
