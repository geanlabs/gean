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

type MultiMessageAggregate struct {
	Proof []byte `json:"proof" ssz-max:"524288"`
}

type SignedBlock struct {
	Block *Block                 `json:"block"`
	Proof *MultiMessageAggregate `json:"proof"`
}

// Header returns the block's header. A block without a body gets a zero body
// root, as a pending block's stored header does.
func (b *Block) Header() (*BlockHeader, error) {
	header := &BlockHeader{
		Slot:          b.Slot,
		ProposerIndex: b.ProposerIndex,
		ParentRoot:    b.ParentRoot,
		StateRoot:     b.StateRoot,
	}
	if b.Body != nil {
		bodyRoot, err := b.Body.HashTreeRoot()
		if err != nil {
			return nil, err
		}
		header.BodyRoot = bodyRoot
	}
	return header, nil
}
