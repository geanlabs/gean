package p2p

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"time"

	"github.com/devylongs/gean/p2p/gossipsub"
	"github.com/golang/snappy"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/libp2p/go-libp2p/core/host"
)

const (
	NetworkName = "devnet0"
)

// Topic names (per networking spec: block and vote)
var (
	BlockTopic = "/leanconsensus/" + NetworkName + "/block/ssz_snappy"
	VoteTopic  = "/leanconsensus/" + NetworkName + "/vote/ssz_snappy"
)

// NewGossipSub creates a new gossipsub instance with lean consensus parameters.
func NewGossipSub(ctx context.Context, h host.Host) (*pubsub.PubSub, error) {
	params := gossipsub.DefaultParams()

	// Start with default gossipsub params and override what we need
	gsParams := pubsub.DefaultGossipSubParams()
	gsParams.D = params.D
	gsParams.Dlo = params.DLow
	gsParams.Dhi = params.DHigh
	gsParams.Dlazy = params.DLazy
	gsParams.HeartbeatInterval = time.Duration(params.HeartbeatInterval * float64(time.Second))
	gsParams.FanoutTTL = time.Duration(params.FanoutTTL) * time.Second
	gsParams.HistoryLength = params.MCacheLen
	gsParams.HistoryGossip = params.MCacheGossip

	opts := []pubsub.Option{
		pubsub.WithMessageIdFn(computePubsubMessageID),
		pubsub.WithGossipSubParams(gsParams),
		pubsub.WithSeenMessagesTTL(time.Duration(params.SeenTTL) * time.Second),
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
		domain = gossipsub.MessageDomainValidSnappy
		data = decoded
	} else {
		domain = gossipsub.MessageDomainInvalidSnappy
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
