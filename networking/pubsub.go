package networking

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"time"

	"github.com/devylongs/gean/types"
	"github.com/golang/snappy"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/libp2p/go-libp2p/core/host"
)

const NetworkName = "devnet0"

// Topic names (per networking spec: block and vote)
var (
	BlockTopic = "/leanconsensus/" + NetworkName + "/block/ssz_snappy"
	VoteTopic  = "/leanconsensus/" + NetworkName + "/vote/ssz_snappy"
)

// Domain types for message ID isolation (per networking spec)
var (
	messageDomainInvalidSnappy = [4]byte{0x00, 0x00, 0x00, 0x00}
	messageDomainValidSnappy   = [4]byte{0x01, 0x00, 0x00, 0x00}
)

// NewGossipSub creates a new gossipsub instance with lean consensus parameters.
func NewGossipSub(ctx context.Context, h host.Host) (*pubsub.PubSub, error) {
	// SeenTTL = SECONDS_PER_SLOT * JUSTIFICATION_LOOKBACK_SLOTS * 2
	// For Devnet 0: 4 * 3 * 2 = 24 seconds
	seenTTL := int(types.SecondsPerSlot) * int(types.JustificationLookbackSlots) * 2

	// Start with default gossipsub params and override what we need
	gsParams := pubsub.DefaultGossipSubParams()
	gsParams.D = 8      // Target mesh peers
	gsParams.Dlo = 6    // Low watermark
	gsParams.Dhi = 12   // High watermark
	gsParams.Dlazy = 6  // Gossip-only peers
	gsParams.HeartbeatInterval = time.Duration(0.7 * float64(time.Second))
	gsParams.FanoutTTL = 60 * time.Second
	gsParams.HistoryLength = 6 // MCacheLen
	gsParams.HistoryGossip = 3 // MCacheGossip

	opts := []pubsub.Option{
		pubsub.WithMessageIdFn(computePubsubMessageID),
		pubsub.WithGossipSubParams(gsParams),
		pubsub.WithSeenMessagesTTL(time.Duration(seenTTL) * time.Second),
		pubsub.WithMessageSignaturePolicy(pubsub.StrictNoSign),
		pubsub.WithFloodPublish(false),
	}

	return pubsub.NewGossipSub(ctx, h, opts...)
}

// computePubsubMessageID computes the message ID for gossipsub.
// ID = SHA256(domain + uint64_le(len(topic)) + topic + data)[:20]
func computePubsubMessageID(msg *pb.Message) string {
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

// CompressMessage compresses data using snappy for gossipsub.
func CompressMessage(data []byte) []byte {
	return snappy.Encode(nil, data)
}

// DecompressMessage decompresses snappy-compressed data.
func DecompressMessage(data []byte) ([]byte, error) {
	return snappy.Decode(nil, data)
}
