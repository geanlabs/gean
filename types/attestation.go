package types

// AttestationData is the content of a validator's vote.
type AttestationData struct {
	Slot   uint64      `json:"slot"`
	Head   *Checkpoint `json:"head"`
	Target *Checkpoint `json:"target"`
	Source *Checkpoint `json:"source"`
}

// Attestation is a single validator's unsigned vote.
type Attestation struct {
	ValidatorID uint64           `json:"validator_id"`
	Data        *AttestationData `json:"data"`
}

// SignedAttestation is an individual validator attestation with XMSS signature.
type SignedAttestation struct {
	ValidatorID uint64              `json:"validator_id"`
	Data        *AttestationData    `json:"data"`
	Signature   [SignatureSize]byte `json:"signature" ssz-size:"3112"`
}

// AggregatedAttestation is a combined vote from multiple validators.
type AggregatedAttestation struct {
	AggregationBits []byte           `json:"aggregation_bits" ssz:"bitlist" ssz-max:"4096"`
	Data            *AttestationData `json:"data"`
}

// SignedAggregatedAttestation carries an aggregated vote with a zkVM proof.
type SignedAggregatedAttestation struct {
	Data  *AttestationData          `json:"data"`
	Proof *AggregatedSignatureProof `json:"proof"`
}
