// message_id.go contains gossipsub message-id computation.
package gossipsub

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/golang/snappy"
	pb "github.com/libp2p/go-libp2p-pubsub/pb"
)

// Message domains for gossipsub message ID computation.
var (
	messageDomainInvalidSnappy = [4]byte{0x00, 0x00, 0x00, 0x00}
	messageDomainValidSnappy   = [4]byte{0x01, 0x00, 0x00, 0x00}
)

// computeMessageID computes the 20-byte message ID for gossipsub deduplication.
// ID = SHA256(domain + len(topic) + topic + data)[:20]
func computeMessageID(msg *pb.Message) string {
	var domain [4]byte
	var data []byte

	// Try to decompress with snappy
	decoded, err := snappy.Decode(nil, msg.Data)
	if err == nil {
		domain = messageDomainValidSnappy
		data = decoded
	} else {
		domain = messageDomainInvalidSnappy
		data = msg.Data
	}

	topic := msg.GetTopic()
	topicBytes := []byte(topic)
	topicLen := make([]byte, 8)
	binary.LittleEndian.PutUint64(topicLen, uint64(len(topicBytes)))

	h := sha256.New()
	h.Write(domain[:])
	h.Write(topicLen)
	h.Write(topicBytes)
	h.Write(data)

	return string(h.Sum(nil)[:20])
}
