package main

import (
	"fmt"

	"github.com/devlongs/gean/common/ssz"
	"github.com/devlongs/gean/common/types"
)

func main() {
	fmt.Println("Gean - Go Lean Ethereum Client")
	fmt.Println()

	// Primitive types
	slot := types.Slot(100)
	validatorIdx := types.ValidatorIndex(42)
	var root types.Root
	root[0] = 0xde
	root[1] = 0xad

	fmt.Printf("Slot: %d\n", slot)
	fmt.Printf("ValidatorIndex: %d\n", validatorIdx)
	fmt.Printf("Root: %x...\n", root[:4])
	fmt.Printf("Root.IsZero(): %v\n", root.IsZero())
	fmt.Println()

	// Time conversions
	genesis := uint64(1700000000)
	time := types.SlotToTime(slot, genesis)
	fmt.Printf("Slot %d -> Time %d\n", slot, time)
	fmt.Printf("Time %d -> Slot %d\n", time, types.TimeToSlot(time, genesis))
	fmt.Println()

	// SSZ merkleization
	h := ssz.HashTreeRootUint64(uint64(slot))
	fmt.Printf("HashTreeRoot(Slot): %x...\n", h[:8])

	chunks := []types.Root{{1}, {2}, {3}}
	merkleRoot := ssz.Merkleize(chunks, 0)
	fmt.Printf("Merkleize(3 chunks): %x...\n", merkleRoot[:8])

	fmt.Println()
	fmt.Println("Phase 1 complete!")
}
