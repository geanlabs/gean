//go:build spectests

package lean

import (
	"testing"

	"github.com/geanlabs/gean/internal/lean/specfixtures"
	"github.com/geanlabs/gean/internal/lean/types"
)

type slotClockFixture struct {
	specfixtures.Header
	// The spec writes some inputs as JSON floats (1700000001600.0); every value
	// is well below 2^53, so float64 holds them exactly.
	Operation struct {
		Kind                    string  `json:"kind"`
		GenesisTime             float64 `json:"genesisTime"`
		CurrentTimeMilliseconds float64 `json:"currentTimeMilliseconds"`
		UnixSeconds             float64 `json:"unixSeconds"`
		Slot                    float64 `json:"slot"`
	} `json:"operation"`
	Config struct {
		SecondsPerSlot          uint64 `json:"secondsPerSlot"`
		IntervalsPerSlot        uint64 `json:"intervalsPerSlot"`
		MillisecondsPerInterval uint64 `json:"millisecondsPerInterval"`
	} `json:"config"`
	Output struct {
		Slot           uint64 `json:"slot"`
		Interval       uint64 `json:"interval"`
		TotalIntervals uint64 `json:"totalIntervals"`
	} `json:"output"`
}

func runSlotClock(t *testing.T, fixture *slotClockFixture) {
	// The fixture's timing config must match gean's constants, or every
	// assertion below compares numbers with different meanings.
	config := fixture.Config
	if config.SecondsPerSlot != types.SecondsPerSlot ||
		config.IntervalsPerSlot != types.IntervalsPerSlot ||
		config.MillisecondsPerInterval != types.MillisecondsPerInterval {
		t.Fatalf("fixture timing %+v differs from gean's %d s/slot, %d intervals/slot, %d ms/interval",
			config, types.SecondsPerSlot, types.IntervalsPerSlot, types.MillisecondsPerInterval)
	}

	operation := fixture.Operation
	genesisTime := uint64(operation.GenesisTime)
	nowMilliseconds := uint64(operation.CurrentTimeMilliseconds)
	var got, want uint64
	switch operation.Kind {
	case "current_slot":
		got, want = types.CurrentSlot(genesisTime, nowMilliseconds), fixture.Output.Slot
	case "current_interval":
		got, want = types.CurrentInterval(genesisTime, nowMilliseconds), fixture.Output.Interval
	case "total_intervals":
		got, want = types.TotalIntervals(genesisTime, nowMilliseconds), fixture.Output.TotalIntervals
	case "from_slot":
		got, want = uint64(operation.Slot)*types.IntervalsPerSlot, fixture.Output.Interval
	case "from_unix_time":
		got, want = types.TotalIntervals(genesisTime, uint64(operation.UnixSeconds)*1000), fixture.Output.Interval
	default:
		t.Fatalf("unknown slot clock operation %q", operation.Kind)
	}
	if got != want {
		t.Errorf("%s(%+v) = %d, want %d", operation.Kind, operation, got, want)
	}
}
