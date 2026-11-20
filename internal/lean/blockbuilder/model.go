package blockbuilder

import (
	"github.com/geanlabs/gean/internal/lean/attestationproof"
	"github.com/geanlabs/gean/internal/lean/types"
)

type AttestationPayload struct {
	DataRoot [32]byte
	Data     *types.AttestationData
	Proofs   []*types.SingleMessageAggregate
}

type Input struct {
	HeadState     *types.State
	Slot          uint64
	ProposerIndex uint64
	ParentRoot    [32]byte
	// KnownBlockRoots reports whether this node has stored a block root.
	KnownBlockRoots func(root [32]byte) bool
	Payloads        []AttestationPayload
	ProofMerger     attestationproof.MergeProvider
}

type Result struct {
	Block             *types.Block
	AttestationProofs []*types.SingleMessageAggregate
	PayloadErrors     []PayloadError
}

type PayloadError struct {
	DataRoot [32]byte
	Err      error
}
