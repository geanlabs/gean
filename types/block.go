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
	Attestations []*SignedVote `ssz-max:"4096"`
}

// Block is a complete block including header fields and body.
type Block struct {
	Slot          uint64
	ProposerIndex uint64
	ParentRoot    [32]byte `ssz-size:"32"`
	StateRoot     [32]byte `ssz-size:"32"`
	Body          *BlockBody
}

// SignedBlock is a container for a block and the proposer's signature.
type SignedBlock struct {
	Message   *Block
	Signature [32]byte `ssz-size:"32"`
}
