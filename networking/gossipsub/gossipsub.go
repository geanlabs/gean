// gossipsub.go contains gossipsub construction and parameter configuration.
package gossipsub

import (
	"context"
	"time"

	"github.com/devylongs/gean/types"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
)

// New creates a gossipsub instance with Lean consensus parameters.
func New(ctx context.Context, h host.Host) (*pubsub.PubSub, error) {
	// SeenTTL = SECONDS_PER_SLOT * JUSTIFICATION_LOOKBACK_SLOTS * 2 = 24 seconds
	seenTTL := int(types.SecondsPerSlot) * int(types.JustificationLookbackSlots) * 2

	gsParams := pubsub.DefaultGossipSubParams()
	gsParams.D = 8                                                         // d: target mesh peers
	gsParams.Dlo = 6                                                       // d_low: low watermark (prune below)
	gsParams.Dhi = 12                                                      // d_high: high watermark (graft above)
	gsParams.Dlazy = 6                                                     // d_lazy: gossip-only peers
	gsParams.HeartbeatInterval = time.Duration(0.7 * float64(time.Second)) // heartbeat_interval_secs
	gsParams.FanoutTTL = 60 * time.Second                                  // fanout_ttl_secs
	gsParams.HistoryLength = 6                                             // mcache_len
	gsParams.HistoryGossip = 3                                             // mcache_gossip

	opts := []pubsub.Option{
		pubsub.WithMessageIdFn(computeMessageID),
		pubsub.WithGossipSubParams(gsParams),
		pubsub.WithSeenMessagesTTL(time.Duration(seenTTL) * time.Second),
		pubsub.WithMessageSignaturePolicy(pubsub.StrictNoSign),
		pubsub.WithFloodPublish(false),
	}

	return pubsub.NewGossipSub(ctx, h, opts...)
}
