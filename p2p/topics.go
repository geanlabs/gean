package p2p

import "fmt"

// ForkDigest is the fork identifier embedded in every gossipsub topic string.
// Lowercase hex, no "0x" prefix (matches the beacon-chain convention adopted
// by leanSpec PR #622). Every Lean client currently agrees on this dummy
// placeholder; this will be derived from the fork version and genesis
// validators root once the spec defines fork identification.
//
// TODO: derive dynamically once the spec defines fork identification.
const ForkDigest = "12345678"

// Topic kind constants rs.
const (
	BlockTopicKind       = "block"
	AttestationTopicKind = "attestation"
	AggregationTopicKind = "aggregation"
)

// BuildGossipTopic assembles a full gossipsub topic string for the given fork
// digest and topic name. Single source of truth for the wire format:
//
//	/leanconsensus/{fork_digest}/{topic_name}/ssz_snappy
func BuildGossipTopic(forkDigest, topicName string) string {
	return fmt.Sprintf("/leanconsensus/%s/%s/ssz_snappy", forkDigest, topicName)
}

// TopicString builds a gossipsub topic string for the current runtime fork
// digest. Retained for callers that always use the live fork digest; for
// cross-fork parameterization (spec tests, etc.) use BuildGossipTopic.
func TopicString(kind string) string {
	return BuildGossipTopic(ForkDigest, kind)
}

// BlockTopic returns the block gossipsub topic.
func BlockTopic() string {
	return TopicString(BlockTopicKind)
}

// AttestationSubnetTopic returns the attestation topic for a given subnet.
// Format: /leanconsensus/{fork_digest}/attestation_{subnet_id}/ssz_snappy
func AttestationSubnetTopic(subnetID uint64) string {
	return TopicString(fmt.Sprintf("%s_%d", AttestationTopicKind, subnetID))
}

// AggregationTopic returns the aggregation gossipsub topic.
func AggregationTopic() string {
	return TopicString(AggregationTopicKind)
}

// SubnetID computes the subnet for a validator.
func SubnetID(validatorID, committeeCount uint64) uint64 {
	return validatorID % committeeCount
}
