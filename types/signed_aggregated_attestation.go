package types

// SignedAggregatedAttestation is the gossip container for aggregated attestations.
// Published on the "aggregation" topic by aggregator nodes at interval 2.
type SignedAggregatedAttestation struct {
	Data  *AttestationData
	Proof *AggregatedSignatureProof
}
