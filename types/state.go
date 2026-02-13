package types

// SSZ limits matching the reference spec.
const (
	HistoricalRootsLimit   = 1 << 18                                      // 262144
	ValidatorRegistryLimit = 1 << 12                                      // 4096
	JustificationValsLimit = HistoricalRootsLimit * ValidatorRegistryLimit // 1073741824
)

// State is the main consensus state object.
type State struct {
	Config                   *Config    `json:"config"`
	Slot                     uint64     `json:"slot"`
	LatestBlockHeader        *BlockHeader `json:"latest_block_header"`
	LatestJustified          *Checkpoint  `json:"latest_justified"`
	LatestFinalized          *Checkpoint  `json:"latest_finalized"`
	HistoricalBlockHashes    [][32]byte `json:"historical_block_hashes"    ssz-max:"262144"`
	JustifiedSlots           []byte     `json:"justified_slots"            ssz:"bitlist" ssz-max:"262144"`
	JustificationsRoots      [][32]byte `json:"justifications_roots"       ssz-max:"262144"`
	JustificationsValidators []byte     `json:"justifications_validators"  ssz:"bitlist" ssz-max:"1073741824"`
}
