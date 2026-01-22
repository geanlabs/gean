package types

//go:generate sszgen --path=. --objs=Checkpoint,Config,Vote,SignedVote,BlockHeader,BlockBody,Block,SignedBlock,State

type Slot uint64
type ValidatorIndex uint64
type Root [32]byte
type Bytes32 = Root
type Bytes48 [48]byte
type Bytes96 [96]byte

const (
	SecondsPerSlot         uint64 = 4
	IntervalsPerSlot       uint64 = 4
	SecondsPerInterval     uint64 = SecondsPerSlot / IntervalsPerSlot // 1
	HistoricalRootsLimit   uint64 = 262144                            // 2^18
	ValidatorRegistryLimit uint64 = 4096                              // 2^12
)

func (r Root) IsZero() bool {
	return r == Root{}
}

func SlotToTime(slot Slot, genesisTime uint64) uint64 {
	return genesisTime + uint64(slot)*SecondsPerSlot
}

func TimeToSlot(time, genesisTime uint64) Slot {
	if time < genesisTime {
		return 0
	}
	return Slot((time - genesisTime) / SecondsPerSlot)
}

// Checkpoint represents a justified or finalized checkpoint.
type Checkpoint struct {
	Root Root `ssz-size:"32"`
	Slot Slot `ssz-size:"8"`
}

// Config holds chain configuration parameters.
type Config struct {
	NumValidators uint64 `ssz-size:"8"`
	GenesisTime   uint64 `ssz-size:"8"`
}

// Vote represents a validator's vote for chain head.
type Vote struct {
	ValidatorID uint64 `ssz-size:"8"`
	Slot        Slot   `ssz-size:"8"`
	Head        Checkpoint
	Target      Checkpoint
	Source      Checkpoint
}

// SignedVote is a vote with its signature.
type SignedVote struct {
	Data      Vote
	Signature Bytes32 `ssz-size:"32"`
}

// BlockHeader summarizes a block without the body.
type BlockHeader struct {
	Slot          Slot   `ssz-size:"8"`
	ProposerIndex uint64 `ssz-size:"8"`
	ParentRoot    Root   `ssz-size:"32"`
	StateRoot     Root   `ssz-size:"32"`
	BodyRoot      Root   `ssz-size:"32"`
}

// BlockBody contains the block's payload.
type BlockBody struct {
	Attestations []SignedVote `ssz-max:"4096"` // ValidatorRegistryLimit
}

// Block is a complete block including header fields and body.
type Block struct {
	Slot          Slot   `ssz-size:"8"`
	ProposerIndex uint64 `ssz-size:"8"`
	ParentRoot    Root   `ssz-size:"32"`
	StateRoot     Root   `ssz-size:"32"`
	Body          BlockBody
}

// SignedBlock is a block with its proposer signature.
type SignedBlock struct {
	Message   Block
	Signature Bytes32 `ssz-size:"32"` // Placeholder; actual signature is larger
}

// State is the beacon state.
type State struct {
	Config                   Config
	Slot                     Slot        `ssz-size:"8"`
	LatestBlockHeader        BlockHeader
	LatestJustified          Checkpoint
	LatestFinalized          Checkpoint
	HistoricalBlockHashes    []Root `ssz-max:"262144"`                   // HistoricalRootsLimit
	JustifiedSlots           []byte `ssz-max:"262144" ssz:"bitlist"`     // HistoricalRootsLimit
	JustificationsRoots      []Root `ssz-max:"262144"`                   // HistoricalRootsLimit
	JustificationsValidators []byte `ssz-max:"1073741824" ssz:"bitlist"` // 2^30 (262144 × 4096)
}
