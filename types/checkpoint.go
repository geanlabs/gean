package types

// Checkpoint represents a checkpoint in the chain's history.
type Checkpoint struct {
	Root [32]byte `ssz-size:"32"`
	Slot uint64
}
