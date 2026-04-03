package p2p

import "fmt"

// Network name matching ethlambda p2p/lib.rs L192.
const NetworkName = "devnet0"

// Topic kind constants matching ethlambda p2p/gossipsub/messages.rs.
const (
	BlockTopicKind       = "block"
	AttestationTopicKind = "attestation"
	AggregationTopicKind = "aggregation"
)

// TopicString builds a gossipsub topic string.
// Format: /leanconsensus/{network}/{kind}/ssz_snappy
// Matches ethlambda p2p/lib.rs L190-200.
func TopicString(kind string) string {
	return fmt.Sprintf("/leanconsensus/%s/%s/ssz_snappy", NetworkName, kind)
}

// BlockTopic returns the block gossipsub topic.
func BlockTopic() string {
	return TopicString(BlockTopicKind)
}

// AttestationSubnetTopic returns the attestation topic for a given subnet.
// Format: /leanconsensus/{network}/attestation_{subnet_id}/ssz_snappy
// Matches ethlambda p2p/lib.rs L208-212.
func AttestationSubnetTopic(subnetID uint64) string {
	return TopicString(fmt.Sprintf("%s_%d", AttestationTopicKind, subnetID))
}

// AggregationTopic returns the aggregation gossipsub topic.
func AggregationTopic() string {
	return TopicString(AggregationTopicKind)
}

// SubnetID computes the subnet for a validator.
// Matches ethlambda store.rs compute_subnet_id (L36).
func SubnetID(validatorID, committeeCount uint64) uint64 {
	return validatorID % committeeCount
}
