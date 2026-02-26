package types

// BlockHeader contains metadata for a block.
type BlockHeader struct {
	Slot          uint64
	ProposerIndex uint64
	ParentRoot    [32]byte `ssz-size:"32"`
	StateRoot     [32]byte `ssz-size:"32"`
	BodyRoot      [32]byte `ssz-size:"32"`
}

// BlockBody contains the payload of a block.
type BlockBody struct {
	Attestations []*AggregatedAttestation `ssz-max:"4096"`
}

// Block is a complete block including header fields and body.
type Block struct {
	Slot          uint64
	ProposerIndex uint64
	ParentRoot    [32]byte `ssz-size:"32"`
	StateRoot     [32]byte `ssz-size:"32"`
	Body          *BlockBody
}

// BlockWithAttestation wraps a block and the proposer's own attestation.
type BlockWithAttestation struct {
	Block               *Block
	ProposerAttestation *Attestation
}

// BlockSignatures contains per-aggregated-attestation proofs and proposer sig.
type BlockSignatures struct {
	AttestationSignatures []*AggregatedSignatureProof `ssz-max:"4096"`
	ProposerSignature     [3112]byte                  `ssz-size:"3112"`
}

// SignedBlockWithAttestation is the gossip/wire envelope for blocks.
type SignedBlockWithAttestation struct {
	Message   *BlockWithAttestation
	Signature BlockSignatures
}
