package types

// AttestationData contains the vote data for a validator's attestation.
type AttestationData struct {
	Slot   uint64
	Head   *Checkpoint
	Target *Checkpoint
	Source *Checkpoint
}

// Attestation wraps a validator ID and attestation data (unsigned, goes in block body).
type Attestation struct {
	ValidatorID uint64
	Data        *AttestationData
}

// SignedAttestation is the gossip envelope for attestations.
// The message field contains the full Attestation (validator_id + data) per leanSpec.
type SignedAttestation struct {
	Message   *Attestation
	Signature [XMSSSignatureSize]byte `ssz-size:"3112"`
}
