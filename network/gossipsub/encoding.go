package gossipsub

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"

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

// PublishAggregatedAttestation publishes an aggregated attestation to gossip.
// Wire format: data_ssz_len(4) + data_ssz + bits_len(4) + bits + agg_sig.
func PublishAggregatedAttestation(ctx context.Context, topic *pubsub.Topic, agg *types.AggregatedAttestation) error {
	dataSSZ, err := agg.Data.MarshalSSZ()
	if err != nil {
		return err
	}

	var buf []byte
	dataLen := make([]byte, 4)
	binary.LittleEndian.PutUint32(dataLen, uint32(len(dataSSZ)))
	buf = append(buf, dataLen...)
	buf = append(buf, dataSSZ...)

	bitsLen := make([]byte, 4)
	binary.LittleEndian.PutUint32(bitsLen, uint32(len(agg.AggregationBits)))
	buf = append(buf, bitsLen...)
	buf = append(buf, agg.AggregationBits...)

	buf = append(buf, agg.AggregatedSignature...)

	return topic.Publish(ctx, snappy.Encode(nil, buf))
}

// DecodeAggregatedAttestation decodes a raw aggregated attestation message.
func DecodeAggregatedAttestation(data []byte) (*types.AggregatedAttestation, error) {
	if len(data) < 8 {
		return nil, fmt.Errorf("message too short: %d", len(data))
	}

	offset := 0
	dataLen := int(binary.LittleEndian.Uint32(data[offset : offset+4]))
	offset += 4
	if offset+dataLen > len(data) {
		return nil, fmt.Errorf("data length exceeds message")
	}
	ad := new(types.AttestationData)
	if err := ad.UnmarshalSSZ(data[offset : offset+dataLen]); err != nil {
		return nil, fmt.Errorf("unmarshal attestation data: %w", err)
	}
	offset += dataLen

	if offset+4 > len(data) {
		return nil, fmt.Errorf("missing bits length")
	}
	bitsLen := int(binary.LittleEndian.Uint32(data[offset : offset+4]))
	offset += 4
	if offset+bitsLen > len(data) {
		return nil, fmt.Errorf("bits length exceeds message")
	}
	bits := make([]byte, bitsLen)
	copy(bits, data[offset:offset+bitsLen])
	offset += bitsLen

	aggSig := make([]byte, len(data)-offset)
	copy(aggSig, data[offset:])

	return &types.AggregatedAttestation{
		Data:                ad,
		AggregationBits:     bits,
		AggregatedSignature: aggSig,
	}, nil
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
