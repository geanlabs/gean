package types

// XMSSSignatureSize is the fixed size of an individual XMSS signature.
const XMSSSignatureSize = 3112

// AggregatedAttestation contains attestation data and participant bitlist.
type AggregatedAttestation struct {
	AggregationBits []byte `ssz:"bitlist" ssz-max:"4096"`
	Data            *AttestationData
}

// AggregatedSignatureProof carries the participants bitlist and proof payload.
type AggregatedSignatureProof struct {
	Participants []byte `ssz:"bitlist" ssz-max:"4096"`
	ProofData    []byte `ssz-max:"1048576"` // ByteListMiB in leanSpec
}
