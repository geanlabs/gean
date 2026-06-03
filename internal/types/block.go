package types

type BlockHeader struct {
	Slot          uint64         `json:"slot"`
	ProposerIndex uint64         `json:"proposer_index"`
	ParentRoot    [RootSize]byte `json:"parent_root" ssz-size:"32"`
	StateRoot     [RootSize]byte `json:"state_root" ssz-size:"32"`
	BodyRoot      [RootSize]byte `json:"body_root" ssz-size:"32"`
}

type BlockBody struct {
	Attestations []*AggregatedAttestation `json:"attestations" ssz-max:"4096"`
}

type Block struct {
	Slot          uint64         `json:"slot"`
	ProposerIndex uint64         `json:"proposer_index"`
	ParentRoot    [RootSize]byte `json:"parent_root" ssz-size:"32"`
	StateRoot     [RootSize]byte `json:"state_root" ssz-size:"32"`
	Body          *BlockBody     `json:"body"`
}

type AggregatedSignatureProof struct {
	Participants []byte `json:"participants" ssz:"bitlist" ssz-max:"4096"`
	ProofData    []byte `json:"proof_data" ssz-max:"1048576"`
}

type BlockSignatures struct {
	AttestationSignatures []*AggregatedSignatureProof `json:"attestation_signatures" ssz-max:"4096"`
	ProposerSignature     [SignatureSize]byte         `json:"proposer_signature" ssz-size:"2536"`
}

type SignedBlock struct {
	Block     *Block           `json:"block"`
	Signature *BlockSignatures `json:"signature"`
}
