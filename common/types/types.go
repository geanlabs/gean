package types

//go:generate sszgen --path=. --objs=Checkpoint

type Slot uint64
type ValidatorIndex uint64
type Epoch uint64
type Root [32]byte

type Bytes4 [4]byte
type Bytes20 [20]byte
type Bytes32 = Root
type Bytes48 [48]byte
type Bytes52 [52]byte
type Bytes96 [96]byte

const SecondsPerSlot uint64 = 4

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

// Checkpoint represents a point in the chain used for justification and finalization.
// It pairs a block root with its slot number.
type Checkpoint struct {
	Root Root `ssz-size:"32"`
	Slot Slot `ssz-size:"8"`
}
