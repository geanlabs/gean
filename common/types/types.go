package types

type Slot uint64
type ValidatorIndex uint64
type Epoch uint64
type Root [32]byte

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
