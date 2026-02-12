// copy.go contains deep-copy and bitlist helper utilities used by transition steps.
package transition

import (
	"github.com/OffchainLabs/go-bitfield"
	"github.com/devylongs/gean/types"
)

// Copy creates a deep copy of the state.
func Copy(s *types.State) *types.State {
	cp := *s
	cp.HistoricalBlockHashes = append([]types.Root{}, s.HistoricalBlockHashes...)
	cp.JustifiedSlots = append([]byte{}, s.JustifiedSlots...)
	cp.Validators = append([]types.Validator{}, s.Validators...)
	cp.JustificationRoots = append([]types.Root{}, s.JustificationRoots...)
	cp.JustificationValidators = append([]byte{}, s.JustificationValidators...)
	return &cp
}

// setBit sets a bit at the given index.
// If the bitlist needs to grow, it creates a new one with sufficient capacity.
func setBit(bits []byte, index int, val bool) []byte {
	bl := bitfield.Bitlist(bits)
	idx := uint64(index)

	// If we need more capacity, create a larger bitlist
	if idx >= bl.Len() {
		newBl := bitfield.NewBitlist(idx + 1)
		// Copy existing bits
		for i := uint64(0); i < bl.Len(); i++ {
			if bl.BitAt(i) {
				newBl.SetBitAt(i, true)
			}
		}
		bl = newBl
	}

	bl.SetBitAt(idx, val)
	return bl
}

// appendBitAt appends a bit at the given index, extending the bitlist if needed.
func appendBitAt(bits []byte, index int, val bool) []byte {
	if len(bits) == 0 {
		bits = bitfield.NewBitlist(uint64(index) + 1)
	}
	return setBit(bits, index, val)
}
