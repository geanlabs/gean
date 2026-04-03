package p2p

import (
	"crypto/sha256"
	"encoding/binary"
)

// Message ID domains matching ethlambda p2p/lib.rs L619-638.
var (
	domainValidSnappy   = [4]byte{0x01, 0x00, 0x00, 0x00}
	domainInvalidSnappy = [4]byte{0x00, 0x00, 0x00, 0x00}
)

// ComputeMessageID computes a gossipsub message ID.
// Format: SHA256(domain || uint64_le(topic_len) || topic || data)[:20]
// Matches ethlambda p2p/lib.rs compute_message_id (L619-638).
//
// domain = 0x01000000 if snappy decompression succeeds (valid)
// domain = 0x00000000 if snappy decompression fails (invalid)
// data = decompressed bytes (if valid) or raw compressed bytes (if invalid)
func ComputeMessageID(topic string, rawData []byte) []byte {
	h := sha256.New()

	// Try to decompress — determines domain and data used for hashing.
	decompressed, err := SnappyRawDecode(rawData)

	var domain [4]byte
	var data []byte
	if err == nil {
		domain = domainValidSnappy
		data = decompressed
	} else {
		domain = domainInvalidSnappy
		data = rawData
	}

	topicBytes := []byte(topic)
	var topicLen [8]byte
	binary.LittleEndian.PutUint64(topicLen[:], uint64(len(topicBytes)))

	h.Write(domain[:])
	h.Write(topicLen[:])
	h.Write(topicBytes)
	h.Write(data)

	hash := h.Sum(nil)
	return hash[:20] // truncate to 20 bytes
}
