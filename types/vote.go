package types

// Vote represents a validator's vote for chain head.
type Vote struct {
	ValidatorID uint64
	Slot        uint64
	Head        *Checkpoint
	Target      *Checkpoint
	Source      *Checkpoint
}

// SignedVote is a container for a vote and its corresponding signature.
type SignedVote struct {
	Data      *Vote
	Signature [32]byte `ssz-size:"32"`
}
