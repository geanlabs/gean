package types

type Checkpoint struct {
	Root [RootSize]byte `json:"root" ssz-size:"32"`
	Slot uint64         `json:"slot"`
}
