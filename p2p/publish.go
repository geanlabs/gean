package p2p

import (
	"context"
	"fmt"

	"github.com/geanlabs/gean/types"
)

// PublishBlock publishes a signed block to the block gossipsub topic.
// SSZ encode -> snappy raw compress -> publish.
func (h *Host) PublishBlock(ctx context.Context, block *types.SignedBlock) error {
	data, err := block.MarshalSSZ()
	if err != nil {
		return fmt.Errorf("marshal block: %w", err)
	}
	return h.publishToTopic(ctx, BlockTopic(), data)
}

// PublishAttestation publishes a signed attestation to the appropriate subnet topic.
// Subnet = validator_id % committee_count.
func (h *Host) PublishAttestation(ctx context.Context, att *types.SignedAttestation, committeeCount uint64) error {
	data, err := att.MarshalSSZ()
	if err != nil {
		return fmt.Errorf("marshal attestation: %w", err)
	}
	subnet := SubnetID(att.ValidatorID, committeeCount)
	topic := AttestationSubnetTopic(subnet)
	return h.publishToTopic(ctx, topic, data)
}

// PublishAggregatedAttestation publishes an aggregated attestation to the aggregation topic.
func (h *Host) PublishAggregatedAttestation(ctx context.Context, agg *types.SignedAggregatedAttestation) error {
	data, err := agg.MarshalSSZ()
	if err != nil {
		return fmt.Errorf("marshal aggregation: %w", err)
	}
	return h.publishToTopic(ctx, AggregationTopic(), data)
}

// publishToTopic SSZ-encodes, snappy-compresses, and publishes to a topic.
func (h *Host) publishToTopic(ctx context.Context, topic string, sszData []byte) error {
	compressed := SnappyRawEncode(sszData)
	t, ok := h.topics[topic]
	if !ok {
		return fmt.Errorf("not subscribed to topic: %s", topic)
	}
	return t.Publish(ctx, compressed)
}
