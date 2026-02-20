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
	Attestations []*Attestation `ssz-max:"4096"`
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

// BlockSignatures is the aggregated signature list for a block envelope.
type BlockSignatures = [][XMSSSignatureSize]byte

// SignedBlockWithAttestation is the gossip/wire envelope for blocks.
type SignedBlockWithAttestation struct {
	Message   *BlockWithAttestation
	Signature BlockSignatures `ssz-max:"4096" ssz-size:"?,3112"`
}
