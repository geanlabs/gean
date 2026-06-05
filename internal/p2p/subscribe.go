package p2p

import (
	"fmt"

	"github.com/geanlabs/gean/internal/logger"
)

func (h *Host) JoinTopic(topic string) error {
	t, err := h.pubsub.Join(topic)
	if err != nil {
		return fmt.Errorf("join topic %s: %w", topic, err)
	}
	sub, err := t.Subscribe()
	if err != nil {
		return fmt.Errorf("subscribe to %s: %w", topic, err)
	}
	h.topics[topic] = t
	h.subs[topic] = sub
	return nil
}

func (h *Host) ReannounceSubscriptions() error {
	if h.gossipHandler == nil {
		return fmt.Errorf("reannounce: gossip listeners not started yet")
	}
	for topic, oldSub := range h.subs {
		oldSub.Cancel()
		t, ok := h.topics[topic]
		if !ok {
			return fmt.Errorf("reannounce: topic %s missing", topic)
		}
		newSub, err := t.Subscribe()
		if err != nil {
			return fmt.Errorf("reannounce: subscribe %s: %w", topic, err)
		}
		h.subs[topic] = newSub
		go h.listenTopic(h.ctx, topic, newSub, h.gossipHandler)
		logger.Info(logger.Network, "re-announced subscription topic=%s", topic)
	}
	return nil
}

func (h *Host) subscribeStartupTopics(
	committeeCount uint64,
	validatorIDs []uint64,
	isAggregator bool,
	aggregateSubnetIDs []uint64,
) error {
	logger.Info(logger.Network, "joining gossipsub topics")
	if err := h.JoinTopic(BlockTopic()); err != nil {
		return fmt.Errorf("join block topic: %w", err)
	}
	if err := h.JoinTopic(AggregationTopic()); err != nil {
		return fmt.Errorf("join aggregation topic: %w", err)
	}

	for subnetID := range initialAttestationSubnets(committeeCount, validatorIDs, isAggregator, aggregateSubnetIDs) {
		if err := h.JoinTopic(AttestationSubnetTopic(subnetID)); err != nil {
			return fmt.Errorf("join attestation subnet %d: %w", subnetID, err)
		}
	}

	for topic := range h.topics {
		logger.Info(logger.Network, "subscribed topic=%s", topic)
	}
	return nil
}

func initialAttestationSubnets(
	committeeCount uint64,
	validatorIDs []uint64,
	isAggregator bool,
	aggregateSubnetIDs []uint64,
) map[uint64]bool {
	seen := make(map[uint64]bool)
	if isAggregator {
		for _, subnetID := range aggregateSubnetIDs {
			seen[subnetID] = true
		}
	}
	for _, vid := range validatorIDs {
		seen[SubnetID(vid, committeeCount)] = true
	}
	if isAggregator && len(seen) == 0 {
		seen[0] = true
	}
	return seen
}
