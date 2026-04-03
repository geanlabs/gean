package types

// Validator represents a consensus validator. Devnet-3 uses a single pubkey
// for both attestation and proposal duties.
type Validator struct {
	Pubkey [PubkeySize]byte `json:"pubkey" ssz-size:"52"`
	Index  uint64           `json:"index"`
}
