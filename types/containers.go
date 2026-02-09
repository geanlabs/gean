package types

//go:generate go run github.com/ferranbt/fastssz/sszgen --path=. --objs=Checkpoint,Config,Vote,SignedVote,BlockHeader,BlockBody,Block,SignedBlock,State

// SSZ containers for the Lean Ethereum consensus protocol.
// Field order is critical for SSZ serialization and must match the spec exactly.

// Checkpoint identifies a block at a specific slot in the chain.
// Used for justification and finalization tracking.
type Checkpoint struct {
	Root Root `ssz-size:"32"`
	Slot Slot
}

// Config holds immutable chain configuration parameters.
type Config struct {
	NumValidators uint64
	GenesisTime   uint64
}

// Vote is the attestation data describing a validator's observed chain view.
type Vote struct {
	ValidatorID uint64
	Slot        Slot
	Head        Checkpoint
	Target      Checkpoint
	Source      Checkpoint
}

// SignedVote wraps a Vote with a signature.
// Devnet 0 uses placeholder signatures (zero bytes).
type SignedVote struct {
	Data      Vote
	Signature Root `ssz-size:"32"`
}

// BlockHeader is the fixed-size portion of a block, used for parent chain linking.
// The StateRoot is initially zero and filled by the next ProcessSlot call.
type BlockHeader struct {
	Slot          Slot
	ProposerIndex uint64
	ParentRoot    Root `ssz-size:"32"`
	StateRoot     Root `ssz-size:"32"`
	BodyRoot      Root `ssz-size:"32"`
}

// BlockBody contains the variable-length block contents.
// Attestations are capped at VALIDATOR_REGISTRY_LIMIT (4096).
type BlockBody struct {
	Attestations []SignedVote `ssz-max:"4096"`
}

// Block is a consensus block containing header fields and a body.
type Block struct {
	Slot          Slot
	ProposerIndex uint64
	ParentRoot    Root `ssz-size:"32"`
	StateRoot     Root `ssz-size:"32"`
	Body          BlockBody
}

// SignedBlock wraps a Block with a signature.
// Devnet 0 uses placeholder signatures (zero bytes).
type SignedBlock struct {
	Message   Block
	Signature Root `ssz-size:"32"`
}

// State is the full consensus state object.
// Field order must match the spec exactly for correct SSZ serialization.
type State struct {
	Config            Config
	Slot              Slot
	LatestBlockHeader BlockHeader

	LatestJustified Checkpoint
	LatestFinalized Checkpoint

	HistoricalBlockHashes []Root `ssz-max:"262144" ssz-size:"?,32"`         // List[Bytes32, HISTORICAL_ROOTS_LIMIT]
	JustifiedSlots        []byte `ssz:"bitlist" ssz-max:"262144"`           // Bitlist[HISTORICAL_ROOTS_LIMIT]
	JustificationRoots      []Root `ssz-max:"262144" ssz-size:"?,32"`       // List[Bytes32, HISTORICAL_ROOTS_LIMIT]
	JustificationValidators []byte `ssz:"bitlist" ssz-max:"1073741824"`     // Bitlist[HISTORICAL_ROOTS_LIMIT * VALIDATOR_REGISTRY_LIMIT]
}
