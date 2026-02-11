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

// Topic format: /leanconsensus/<network>/<type>/ssz_snappy
// NetworkName stays "devnet0" — all interop clients use this regardless of version.
var (
	BlockTopic       = "/leanconsensus/" + NetworkName + "/block/ssz_snappy"
	AttestationTopic = "/leanconsensus/" + NetworkName + "/attestation/ssz_snappy"
)

// Message domains for gossipsub message ID computation.
var (
	messageDomainInvalidSnappy = [4]byte{0x00, 0x00, 0x00, 0x00}
	messageDomainValidSnappy   = [4]byte{0x01, 0x00, 0x00, 0x00}
)

// NewGossipSub creates a gossipsub instance with Lean consensus parameters.
func NewGossipSub(ctx context.Context, h host.Host) (*pubsub.PubSub, error) {
	// SeenTTL = SECONDS_PER_SLOT * JUSTIFICATION_LOOKBACK_SLOTS * 2 = 24 seconds
	seenTTL := int(types.SecondsPerSlot) * int(types.JustificationLookbackSlots) * 2

	gsParams := pubsub.DefaultGossipSubParams()
	gsParams.D = 8                                                    // d: target mesh peers
	gsParams.Dlo = 6                                                  // d_low: low watermark (prune below)
	gsParams.Dhi = 12                                                 // d_high: high watermark (graft above)
	gsParams.Dlazy = 6                                                // d_lazy: gossip-only peers
	gsParams.HeartbeatInterval = time.Duration(0.7 * float64(time.Second)) // heartbeat_interval_secs
	gsParams.FanoutTTL = 60 * time.Second                             // fanout_ttl_secs
	gsParams.HistoryLength = 6                                        // mcache_len
	gsParams.HistoryGossip = 3                                        // mcache_gossip

	opts := []pubsub.Option{
		pubsub.WithMessageIdFn(computePubsubMessageID),
		pubsub.WithGossipSubParams(gsParams),
		pubsub.WithSeenMessagesTTL(time.Duration(seenTTL) * time.Second),
		pubsub.WithMessageSignaturePolicy(pubsub.StrictNoSign),
		pubsub.WithFloodPublish(false),
	}

	return pubsub.NewGossipSub(ctx, h, opts...)
}

// computePubsubMessageID computes the 20-byte message ID for gossipsub deduplication.
// ID = SHA256(domain + len(topic) + topic + data)[:20]
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
