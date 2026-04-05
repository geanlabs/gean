package types

// State is the full beacon consensus state at a given slot.
type State struct {
	Config                   *ChainConfig  `json:"config"`
	Slot                     uint64        `json:"slot"`
	LatestBlockHeader        *BlockHeader  `json:"latest_block_header"`
	LatestJustified          *Checkpoint   `json:"latest_justified"`
	LatestFinalized          *Checkpoint   `json:"latest_finalized"`
	HistoricalBlockHashes    [][]byte      `json:"historical_block_hashes" ssz-max:"262144" ssz-size:"?,32"`
	JustifiedSlots           []byte        `json:"justified_slots" ssz:"bitlist" ssz-max:"262144"`
	Validators               []*Validator  `json:"validators" ssz-max:"4096"`
	JustificationsRoots      [][]byte      `json:"justifications_roots" ssz-max:"262144" ssz-size:"?,32"`
	JustificationsValidators []byte        `json:"justifications_validators" ssz:"bitlist" ssz-max:"1073741824"`
}

// NumValidators returns the validator count.
func (s *State) NumValidators() uint64 {
	return uint64(len(s.Validators))
}
