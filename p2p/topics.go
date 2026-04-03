package p2p

import "fmt"

// Network name rs L192.
const NetworkName = "devnet0"

// Topic kind constants rs.
const (
	BlockTopicKind       = "block"
	AttestationTopicKind = "attestation"
	AggregationTopicKind = "aggregation"
)

// TopicString builds a gossipsub topic string.
// Format: /leanconsensus/{network}/{kind}/ssz_snappy
func TopicString(kind string) string {
	return fmt.Sprintf("/leanconsensus/%s/%s/ssz_snappy", NetworkName, kind)
}

// BlockTopic returns the block gossipsub topic.
func BlockTopic() string {
	return TopicString(BlockTopicKind)
}

// AttestationSubnetTopic returns the attestation topic for a given subnet.
// Format: /leanconsensus/{network}/attestation_{subnet_id}/ssz_snappy
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
