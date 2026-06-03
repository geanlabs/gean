package types

type Validator struct {
	AttestationPubkey [PubkeySize]byte `json:"attestation_pubkey" ssz-size:"52"`
	ProposalPubkey    [PubkeySize]byte `json:"proposal_pubkey" ssz-size:"52"`
	Index             uint64           `json:"index"`
}
