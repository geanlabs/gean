package blockprocessor

import "fmt"

func slot32(slot uint64) (uint32, error) {
	converted := uint32(slot)
	if uint64(converted) != slot {
		return 0, fmt.Errorf("slot %d overflows uint32", slot)
	}
	return converted, nil
}
