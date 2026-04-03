package types

// BlockHeader contains block metadata without the body.
type BlockHeader struct {
	Slot          uint64         `json:"slot"`
	ProposerIndex uint64         `json:"proposer_index"`
	ParentRoot    [RootSize]byte `json:"parent_root" ssz-size:"32"`
	StateRoot     [RootSize]byte `json:"state_root" ssz-size:"32"`
	BodyRoot      [RootSize]byte `json:"body_root" ssz-size:"32"`
}

// BlockBody contains the attestations included in a block.
type BlockBody struct {
	Attestations []*AggregatedAttestation `json:"attestations" ssz-max:"4096"`
}

// Block is the core block structure proposed by a validator.
type Block struct {
	Slot          uint64         `json:"slot"`
	ProposerIndex uint64         `json:"proposer_index"`
	ParentRoot    [RootSize]byte `json:"parent_root" ssz-size:"32"`
	StateRoot     [RootSize]byte `json:"state_root" ssz-size:"32"`
	Body          *BlockBody     `json:"body"`
}

// BlockWithAttestation pairs a block with the proposer's own attestation.
type BlockWithAttestation struct {
	Block               *Block       `json:"block"`
	ProposerAttestation *Attestation `json:"proposer_attestation"`
}

// AggregatedSignatureProof is a zkVM proof that a set of validators signed.
type AggregatedSignatureProof struct {
	Participants []byte `json:"participants" ssz:"bitlist" ssz-max:"4096"`
	ProofData    []byte `json:"proof_data" ssz-max:"1048576"`
}

// BlockSignatures carries the XMSS signatures for a block.
type BlockSignatures struct {
	AttestationSignatures []*AggregatedSignatureProof `json:"attestation_signatures" ssz-max:"4096"`
	ProposerSignature     [SignatureSize]byte          `json:"proposer_signature" ssz-size:"3112"`
}

// SignedBlockWithAttestation is the complete signed block as gossiped on the network.
type SignedBlockWithAttestation struct {
	Block     *BlockWithAttestation `json:"block"`
	Signature *BlockSignatures      `json:"signature"`
}
