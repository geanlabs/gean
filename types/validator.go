package types

// Validator represents a validator's static metadata.
// Devnet-4: separate keys for attestation and proposal duties.
type Validator struct {
	AttestationPubkey [PubkeySize]byte `json:"attestation_pubkey" ssz-size:"52"`
	ProposalPubkey    [PubkeySize]byte `json:"proposal_pubkey" ssz-size:"52"`
	Index             uint64           `json:"index"`
}
