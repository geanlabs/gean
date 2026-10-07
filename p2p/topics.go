package p2p

import (
	"fmt"
	"strconv"
	"strings"
)

const ForkDigest = "12345678"

const (
	BlockTopicKind       = "block"
	AttestationTopicKind = "attestation"
	AggregationTopicKind = "aggregation"
)

func BuildGossipTopic(forkDigest, topicName string) string {
	return fmt.Sprintf("/leanconsensus/%s/%s/ssz_snappy", forkDigest, topicName)
}

func TopicString(kind string) string {
	return BuildGossipTopic(ForkDigest, kind)
}

func BlockTopic() string {
	return TopicString(BlockTopicKind)
}

func AttestationSubnetTopic(subnetID uint64) string {
	return TopicString(fmt.Sprintf("%s_%d", AttestationTopicKind, subnetID))
}

func isAttestationSubnetTopic(topic string) bool {
	prefix := fmt.Sprintf("/leanconsensus/%s/%s_", ForkDigest, AttestationTopicKind)
	suffix := "/ssz_snappy"
	if !strings.HasPrefix(topic, prefix) || !strings.HasSuffix(topic, suffix) {
		return false
	}
	subnet := strings.TrimSuffix(strings.TrimPrefix(topic, prefix), suffix)
	_, err := strconv.ParseUint(subnet, 10, 64)
	return err == nil
}

func AggregationTopic() string {
	return TopicString(AggregationTopicKind)
}

func SubnetID(validatorID, committeeCount uint64) uint64 {
	if committeeCount == 0 {
		return 0
	}
	return validatorID % committeeCount
}
