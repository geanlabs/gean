package forkchoice

import "github.com/geanlabs/gean/types"

// View is an immutable picture of fork choice and the checkpoints it was
// computed against. The engine publishes a new View after each head and
// safe-target update, so readers on other goroutines see one consistent state
// without touching the live fork choice, which only the engine may use. It
// carries everything a reader reports, because pruning may delete the stored
// blocks and states it refers to at any time.
type View struct {
	Nodes          []ViewNode
	Head           [32]byte
	Justified      types.Checkpoint
	Finalized      types.Checkpoint
	SafeTarget     [32]byte
	ValidatorCount uint64
}

// ViewNode is a fork choice node with the proposer of its block.
type ViewNode struct {
	ProtoNode
	ProposerIndex uint64
}
