package types

//go:generate go run github.com/ferranbt/fastssz/sszgen --path=. --objs=Checkpoint,Config,Vote,SignedVote,BlockHeader,BlockBody,Block,SignedBlock,State,Status,BlocksByRootRequest

// SSZ Containers

// Checkpoint is a (root, slot) pair identifying a block in the chain.
type Checkpoint struct {
	Root Root `ssz-size:"32"`
	Slot Slot
}

type Config struct {
	NumValidators uint64
	GenesisTime   uint64
}

// Vote is a validator's attestation for head, target, and source.
type Vote struct {
	ValidatorID uint64
	Slot        Slot
	Head        Checkpoint
	Target      Checkpoint
	Source      Checkpoint
}

type SignedVote struct {
	Data      Vote
	Signature Root `ssz-size:"32"`
}

type BlockHeader struct {
	Slot          Slot
	ProposerIndex uint64
	ParentRoot    Root `ssz-size:"32"`
	StateRoot     Root `ssz-size:"32"`
	BodyRoot      Root `ssz-size:"32"`
}

type BlockBody struct {
	Attestations []SignedVote `ssz-max:"4096"`
}

type Block struct {
	Slot          Slot
	ProposerIndex uint64
	ParentRoot    Root `ssz-size:"32"`
	StateRoot     Root `ssz-size:"32"`
	Body          BlockBody
}

type SignedBlock struct {
	Message   Block
	Signature Root `ssz-size:"32"`
}

// State is the main consensus state object.
type State struct {
	Config            Config
	Slot              Slot
	LatestBlockHeader BlockHeader

	LatestJustified Checkpoint
	LatestFinalized Checkpoint

	HistoricalBlockHashes []Root `ssz-max:"262144" ssz-size:"?,32"`
	JustifiedSlots        []byte `ssz:"bitlist" ssz-max:"262144"` // Bitlist[HISTORICAL_ROOTS_LIMIT]

	// Justification tracking (unused in Devnet 0 but required for SSZ compatibility)
	JustificationRoots      []Root `ssz-max:"262144" ssz-size:"?,32"`
	JustificationValidators []byte `ssz:"bitlist" ssz-max:"1073741824"` // Bitlist[HISTORICAL_ROOTS_LIMIT * VALIDATOR_REGISTRY_LIMIT]
}
