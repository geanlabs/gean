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

func attestationSubnet(topic string) (uint64, bool) {
	prefix := fmt.Sprintf("/leanconsensus/%s/%s_", ForkDigest, AttestationTopicKind)
	suffix := "/ssz_snappy"
	if !strings.HasPrefix(topic, prefix) || !strings.HasSuffix(topic, suffix) {
		return 0, false
	}
	subnet, err := strconv.ParseUint(strings.TrimSuffix(strings.TrimPrefix(topic, prefix), suffix), 10, 64)
	return subnet, err == nil
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
