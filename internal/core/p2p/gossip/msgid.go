package gossip

import (
	"crypto/sha256"
	"encoding/binary"
)

var (
	domainValidSnappy   = [4]byte{0x01, 0x00, 0x00, 0x00}
	domainInvalidSnappy = [4]byte{0x00, 0x00, 0x00, 0x00}
)

// MessageID is the gossipsub message id: the first 20 bytes of
// sha256(domain || uint64_le(len(topic)) || topic || data), where data is the
// decompressed payload under the valid-snappy domain, or the raw payload under
// the invalid-snappy domain when it does not decompress.
func MessageID(topic string, rawData []byte) []byte {
	h := sha256.New()

	decompressed, err := Decompress(rawData)

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
	return hash[:20]
}
