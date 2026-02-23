package types

// SSZ limits matching the reference spec.
const (
	HistoricalRootsLimit   = 1 << 18                                       // 262144
	ValidatorRegistryLimit = 1 << 12                                       // 4096
	JustificationValsLimit = HistoricalRootsLimit * ValidatorRegistryLimit // 1073741824
)

// Validator represents a validator in the registry.
type Validator struct {
	Pubkey [52]byte `ssz-size:"52"`
	Index  uint64
}

// State is the main consensus state object.
type State struct {
	Config                   *Config      `json:"config"`
	Slot                     uint64       `json:"slot"`
	LatestBlockHeader        *BlockHeader `json:"latest_block_header"`
	LatestJustified          *Checkpoint  `json:"latest_justified"`
	LatestFinalized          *Checkpoint  `json:"latest_finalized"`
	HistoricalBlockHashes    [][32]byte   `json:"historical_block_hashes"    ssz-max:"262144"`
	JustifiedSlots           []byte       `json:"justified_slots"            ssz:"bitlist" ssz-max:"262144"`
	Validators               []*Validator `json:"validators"                 ssz-max:"4096"`
	JustificationsRoots      [][32]byte   `json:"justifications_roots"       ssz-max:"262144"`
	JustificationsValidators []byte       `json:"justifications_validators"  ssz:"bitlist" ssz-max:"1073741824"`
}

// Copy returns a deep copy of the state.
func (s *State) Copy() *State {
	out := &State{
		Slot: s.Slot,
	}

	if s.Config != nil {
		out.Config = &Config{GenesisTime: s.Config.GenesisTime}
	}
	if s.LatestBlockHeader != nil {
		h := *s.LatestBlockHeader
		out.LatestBlockHeader = &h
	}
	if s.LatestJustified != nil {
		out.LatestJustified = &Checkpoint{Root: s.LatestJustified.Root, Slot: s.LatestJustified.Slot}
	}
	if s.LatestFinalized != nil {
		out.LatestFinalized = &Checkpoint{Root: s.LatestFinalized.Root, Slot: s.LatestFinalized.Slot}
	}
	if s.HistoricalBlockHashes != nil {
		out.HistoricalBlockHashes = make([][32]byte, len(s.HistoricalBlockHashes))
		copy(out.HistoricalBlockHashes, s.HistoricalBlockHashes)
	}
	if s.JustifiedSlots != nil {
		out.JustifiedSlots = make([]byte, len(s.JustifiedSlots))
		copy(out.JustifiedSlots, s.JustifiedSlots)
	}
	if s.Validators != nil {
		out.Validators = make([]*Validator, len(s.Validators))
		for i, v := range s.Validators {
			cp := *v
			out.Validators[i] = &cp
		}
	}
	if s.JustificationsRoots != nil {
		out.JustificationsRoots = make([][32]byte, len(s.JustificationsRoots))
		copy(out.JustificationsRoots, s.JustificationsRoots)
	}
	if s.JustificationsValidators != nil {
		out.JustificationsValidators = make([]byte, len(s.JustificationsValidators))
		copy(out.JustificationsValidators, s.JustificationsValidators)
	}

	return out
}
