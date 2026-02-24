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
// Per devnet-1 spec, Message is a nested *Attestation (not flattened).
type SignedAttestation struct {
	Message   *Attestation
	Signature [3112]byte `ssz-size:"3112"`
}
