package gossipsub

import (
	"context"
	"crypto/sha256"
	"encoding/binary"

	"github.com/golang/snappy"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pb "github.com/libp2p/go-libp2p-pubsub/pb"

	"github.com/geanlabs/gean/types"
)

// Message domains for ID computation.
var (
	DomainValidSnappy   = []byte{0x01, 0x00, 0x00, 0x00}
	DomainInvalidSnappy = []byte{0x00, 0x00, 0x00, 0x00}
)

// PublishBlock SSZ-encodes, snappy-compresses, and publishes a signed block.
func PublishBlock(ctx context.Context, topic *pubsub.Topic, sb *types.SignedBlock) error {
	data, err := sb.MarshalSSZ()
	if err != nil {
		return err
	}
	return topic.Publish(ctx, snappy.Encode(nil, data))
}

// PublishVote SSZ-encodes, snappy-compresses, and publishes a signed vote.
func PublishVote(ctx context.Context, topic *pubsub.Topic, sv *types.SignedVote) error {
	data, err := sv.MarshalSSZ()
	if err != nil {
		return err
	}
	return topic.Publish(ctx, snappy.Encode(nil, data))
}

// ComputeMessageID computes SHA256(domain + uint64_le(topic_len) + topic + data)[:20].
func ComputeMessageID(pmsg *pb.Message) string {
	topic := pmsg.GetTopic()
	data := pmsg.GetData()

	// Try snappy decompress to determine domain.
	domain := DomainInvalidSnappy
	msgData := data
	if decoded, err := snappy.Decode(nil, data); err == nil {
		domain = DomainValidSnappy
		msgData = decoded
	}

	topicBytes := []byte(topic)
	var topicLen [8]byte
	binary.LittleEndian.PutUint64(topicLen[:], uint64(len(topicBytes)))

	h := sha256.New()
	h.Write(domain)
	h.Write(topicLen[:])
	h.Write(topicBytes)
	h.Write(msgData)
	digest := h.Sum(nil)

	return string(digest[:20])
}
