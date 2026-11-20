package p2p

import (
	"fmt"

	"github.com/geanlabs/gean/internal/core/logger"
)

func (h *Host) subscribeStartupTopics(
	committeeCount uint64,
	validatorIDs []uint64,
	isAggregator bool,
	aggregateSubnetIDs []uint64,
) error {
	logger.Info(logger.Network, "joining gossipsub topics")
	if err := h.JoinTopic(BlockTopic(), h.validateGossip); err != nil {
		return fmt.Errorf("join block topic: %w", err)
	}
	if err := h.JoinTopic(AggregationTopic(), h.validateGossip); err != nil {
		return fmt.Errorf("join aggregation topic: %w", err)
	}

	for subnetID := range initialAttestationSubnets(committeeCount, validatorIDs, isAggregator, aggregateSubnetIDs) {
		if err := h.JoinTopic(AttestationSubnetTopic(subnetID), h.validateGossip); err != nil {
			return fmt.Errorf("join attestation subnet %d: %w", subnetID, err)
		}
	}

	for _, topic := range h.Topics() {
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
