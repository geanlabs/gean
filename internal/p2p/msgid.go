package p2p

import (
	"crypto/sha256"
	"encoding/binary"
)

var (
	domainValidSnappy   = [4]byte{0x01, 0x00, 0x00, 0x00}
	domainInvalidSnappy = [4]byte{0x00, 0x00, 0x00, 0x00}
)

func ComputeMessageID(topic string, rawData []byte) []byte {
	h := sha256.New()

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
	return hash[:20]
}
