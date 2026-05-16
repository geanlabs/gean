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

// AggregatedSignatureProof is a zkVM proof that a set of validators signed.
type AggregatedSignatureProof struct {
	Participants []byte `json:"participants" ssz:"bitlist" ssz-max:"4096"`
	ProofData    []byte `json:"proof_data" ssz-max:"1048576"`
}

// BlockSignatures carries the XMSS signatures for a block.
// ProposerSignature signs hash_tree_root(block) with the proposer's proposal key.
type BlockSignatures struct {
	AttestationSignatures []*AggregatedSignatureProof `json:"attestation_signatures" ssz-max:"4096"`
	ProposerSignature     [SignatureSize]byte         `json:"proposer_signature" ssz-size:"2536"`
}

// SignedBlock is the complete signed block as gossiped on the network.
// Devnet-4: proposer signs hash_tree_root(block) with proposal key.
// BlockWithAttestation removed per leanSpec PR #449.
// Spec: lean_spec/subspecs/containers/block/block.py
type SignedBlock struct {
	Block     *Block           `json:"block"`
	Signature *BlockSignatures `json:"signature"`
}
