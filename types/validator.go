package types

// Validator represents a consensus validator. Devnet-4 uses separate pubkeys
// for attestation and proposal duties to contain blast radius on XMSS leaf reuse.
type Validator struct {
	AttestationPubkey [PubkeySize]byte `json:"attestation_pubkey" ssz-size:"52"`
	ProposalPubkey    [PubkeySize]byte `json:"proposal_pubkey" ssz-size:"52"`
	Index             uint64           `json:"index"`
}
