package types

// XMSSSignatureSize is the fixed size of an individual XMSS signature.
const XMSSSignatureSize = 3112

// AggregatedAttestation contains an attestation aggregated from multiple
// validators. Signatures are concatenated in validator index order.
type AggregatedAttestation struct {
	Data            *AttestationData
	AggregationBits []byte `ssz:"bitlist" ssz-max:"4096"`
	// AggregatedSignature is the concatenation of XMSS signatures in
	// ascending validator index order: sig_0 || sig_1 || sig_2 || ...
	AggregatedSignature []byte `ssz-max:"12738672"` // 4096 * 3112
}

// SignedAggregatedAttestation is the gossip envelope for aggregated attestations.
type SignedAggregatedAttestation struct {
	Message *AggregatedAttestation
}
