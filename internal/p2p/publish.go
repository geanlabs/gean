package p2p

import (
	"context"
	"fmt"

	"github.com/geanlabs/gean/internal/types"
)

func (h *Host) PublishBlock(ctx context.Context, block *types.SignedBlock) error {
	if block == nil {
		return fmt.Errorf("publish block: nil block")
	}
	data, err := block.MarshalSSZ()
	if err != nil {
		return fmt.Errorf("marshal block: %w", err)
	}
	return h.publishToTopic(ctx, BlockTopic(), data)
}

func (h *Host) PublishAttestation(ctx context.Context, att *types.SignedAttestation, committeeCount uint64) error {
	if att == nil {
		return fmt.Errorf("publish attestation: nil attestation")
	}
	data, err := att.MarshalSSZ()
	if err != nil {
		return fmt.Errorf("marshal attestation: %w", err)
	}
	subnet := SubnetID(att.ValidatorID, committeeCount)
	topic := AttestationSubnetTopic(subnet)
	return h.publishToTopic(ctx, topic, data)
}

func (h *Host) PublishAggregatedAttestation(ctx context.Context, agg *types.SignedAggregatedAttestation) error {
	if agg == nil {
		return fmt.Errorf("publish aggregation: nil aggregation")
	}
	data, err := agg.MarshalSSZ()
	if err != nil {
		return fmt.Errorf("marshal aggregation: %w", err)
	}
	return h.publishToTopic(ctx, AggregationTopic(), data)
}

func (h *Host) publishToTopic(ctx context.Context, topic string, sszData []byte) error {
	compressed := SnappyRawEncode(sszData)
	t, ok := h.topics[topic]
	if !ok {
		return fmt.Errorf("not subscribed to topic: %s", topic)
	}
	return t.Publish(ctx, compressed)
}
