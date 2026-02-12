// attestations.go contains attestation processing and justified/finalized checkpoint updates.
package transition

import (
	"github.com/OffchainLabs/go-bitfield"
	"github.com/devylongs/gean/types"
)

// ProcessAttestations processes attestations and updates justification/finalization (3SF-mini).
// Each valid attestation individually justifies its target (no supermajority counting).
// Finalization: if slots N and N+1 are both justified, slot N becomes finalized.
// TODO: rewrite to use 2/3 supermajority counting.
func ProcessAttestations(s *types.State, attestations []types.Attestation) (*types.State, error) {
	newState := Copy(s)

	bl := bitfield.Bitlist(newState.JustifiedSlots)
	justifiedSlots := make([]bool, bl.Len())
	for i := uint64(0); i < bl.Len(); i++ {
		justifiedSlots[i] = bl.BitAt(i)
	}

	latestJustified := newState.LatestJustified
	latestFinalized := newState.LatestFinalized
	// Original finalized slot used for gap checks (immutable during this call).
	originalFinalizedSlot := newState.LatestFinalized.Slot

	for _, att := range attestations {
		source := att.Data.Source
		target := att.Data.Target

		sourceSlot := int(source.Slot)
		targetSlot := int(target.Slot)

		// Validate source comes before target
		if source.Slot >= target.Slot {
			continue
		}

		// Check if source is justified
		if sourceSlot >= len(justifiedSlots) {
			continue
		}
		if !justifiedSlots[sourceSlot] {
			continue
		}

		// Justify target if not already justified
		alreadyJustified := targetSlot < len(justifiedSlots) && justifiedSlots[targetSlot]
		if !alreadyJustified {
			for len(justifiedSlots) <= targetSlot {
				justifiedSlots = append(justifiedSlots, false)
			}
			justifiedSlots[targetSlot] = true

			if target.Slot > latestJustified.Slot {
				latestJustified = target
			}
		}

		if hasNoJustifiableGap(source.Slot, target.Slot, originalFinalizedSlot) && source.Slot >= latestFinalized.Slot {
			latestFinalized = source
		}
	}

	// Write justifiedSlots back to bitlist
	newBl := bitfield.NewBitlist(uint64(len(justifiedSlots)))
	for i, v := range justifiedSlots {
		if v {
			newBl.SetBitAt(uint64(i), true)
		}
	}
	newState.JustifiedSlots = newBl
	newState.LatestJustified = latestJustified
	newState.LatestFinalized = latestFinalized

	return newState, nil
}
