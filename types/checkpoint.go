package types

// Checkpoint is a finality marker: a block root at a specific slot.
type Checkpoint struct {
	Root [RootSize]byte `json:"root" ssz-size:"32"`
	Slot uint64         `json:"slot"`
}
