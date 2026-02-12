// finalization.go contains helper logic for no-justifiable-gap finalization checks.
package transition

import "github.com/devylongs/gean/types"

// hasNoJustifiableGap reports whether there are no justifiable slots between source and target.
func hasNoJustifiableGap(sourceSlot types.Slot, targetSlot types.Slot, originalFinalizedSlot types.Slot) bool {
	// No justifiable gap means the chain of trust is unbroken.
	for s := int(sourceSlot) + 1; s < int(targetSlot); s++ {
		if types.Slot(s).IsJustifiableAfter(originalFinalizedSlot) {
			return false
		}
	}
	return true
}
