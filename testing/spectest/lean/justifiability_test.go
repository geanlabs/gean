//go:build spectests

package lean

import (
	"testing"

	"github.com/geanlabs/gean/internal/lean/specfixtures"
	"github.com/geanlabs/gean/internal/lean/transition"
)

type justifiabilityFixture struct {
	specfixtures.Header
	Slot          uint64 `json:"slot"`
	FinalizedSlot uint64 `json:"finalizedSlot"`
	Output        struct {
		// Delta is slot minus finalizedSlot, negative for a slot before the
		// finalized boundary.
		Delta         int64 `json:"delta"`
		IsJustifiable bool  `json:"isJustifiable"`
	} `json:"output"`
}

func runJustifiability(t *testing.T, fixture *justifiabilityFixture) {
	if delta := int64(fixture.Slot) - int64(fixture.FinalizedSlot); fixture.Output.Delta != delta {
		t.Fatalf("fixture delta %d, but slot %d - finalizedSlot %d = %d",
			fixture.Output.Delta, fixture.Slot, fixture.FinalizedSlot, delta)
	}
	if got := transition.SlotIsJustifiableAfter(fixture.Slot, fixture.FinalizedSlot); got != fixture.Output.IsJustifiable {
		t.Errorf("SlotIsJustifiableAfter(%d, %d) = %v, want %v",
			fixture.Slot, fixture.FinalizedSlot, got, fixture.Output.IsJustifiable)
	}
}
