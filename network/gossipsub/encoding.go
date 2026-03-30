package gossipsub

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"sync"

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
func PublishBlock(ctx context.Context, topic *pubsub.Topic, sb *types.SignedBlockWithAttestation) error {
	data, err := sb.MarshalSSZ()
	if err != nil {
		return err
	}
	return topic.Publish(ctx, snappy.Encode(nil, data))
}

// PublishAttestation SSZ-encodes, snappy-compresses, and publishes a signed attestation.
func PublishAttestation(ctx context.Context, topic *pubsub.Topic, sa *types.SignedAttestation) error {
	data, err := sa.MarshalSSZ()
	if err != nil {
		return err
	}
	return topic.Publish(ctx, snappy.Encode(nil, data))
}

// PublishAggregatedAttestation SSZ-encodes, snappy-compresses, and publishes a signed aggregated attestation.
func PublishAggregatedAttestation(ctx context.Context, topic *pubsub.Topic, saa *types.SignedAggregatedAttestation) error {
	data, err := saa.MarshalSSZ()
	if err != nil {
		return err
	}
	return topic.Publish(ctx, snappy.Encode(nil, data))
}

// decodePool reuses snappy decode buffers to reduce allocations in the
// gossipsub event loop hot path (ComputeMessageID is called per message).
var decodePool = sync.Pool{
	New: func() any { b := make([]byte, 0, 8192); return &b },
}

// ComputeMessageID computes SHA256(domain + uint64_le(topic_len) + topic + data)[:20].
func ComputeMessageID(pmsg *pb.Message) string {
	topic := pmsg.GetTopic()
	data := pmsg.GetData()

	// Try snappy decompress to determine domain.
	domain := DomainInvalidSnappy
	msgData := data

	bufp := decodePool.Get().(*[]byte)
	buf := *bufp
	if dLen, err := snappy.DecodedLen(data); err == nil && dLen > 0 {
		if cap(buf) < dLen {
			buf = make([]byte, 0, dLen)
		}
		if decoded, err := snappy.Decode(buf[:0], data); err == nil {
			domain = DomainValidSnappy
			msgData = decoded
		}
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

	*bufp = buf
	decodePool.Put(bufp)

	return string(digest[:20])
}
