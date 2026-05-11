package types

import "testing"

func TestCurrentSlot(t *testing.T) {
	const gt = 1700000000
	const gtMs = gt * 1000
	tests := []struct {
		name        string
		currentMs   uint64
		expectedSlot uint64
	}{
		{"before_genesis", gtMs - 5000, 0},
		{"at_genesis", gtMs, 0},
		{"mid_slot_0", gtMs + 1500, 0},
		{"at_slot_1", gtMs + uint64(MillisecondsPerSlot), 1},
		{"slot_10", gtMs + 10*uint64(MillisecondsPerSlot), 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CurrentSlot(gt, tt.currentMs); got != tt.expectedSlot {
				t.Fatalf("CurrentSlot(%d, %d) = %d, want %d", gt, tt.currentMs, got, tt.expectedSlot)
			}
		})
	}
}

func TestCurrentInterval(t *testing.T) {
	const gt = 1700000000
	const gtMs = gt * 1000
	tests := []struct {
		name             string
		currentMs        uint64
		expectedInterval uint64
	}{
		{"before_genesis", gtMs - 5000, 0},
		{"at_slot_start", gtMs, 0},
		{"800ms_in", gtMs + 800, 1},
		{"1600ms_in", gtMs + 1600, 2},
		{"3200ms_in", gtMs + 3200, 4},
		{"wraps_at_next_slot", gtMs + uint64(MillisecondsPerSlot), 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CurrentInterval(gt, tt.currentMs); got != tt.expectedInterval {
				t.Fatalf("CurrentInterval(%d, %d) = %d, want %d", gt, tt.currentMs, got, tt.expectedInterval)
			}
		})
	}
}

func TestTotalIntervals(t *testing.T) {
	const gt = 1700000000
	const gtMs = gt * 1000
	tests := []struct {
		name      string
		currentMs uint64
		want      uint64
	}{
		{"before_genesis", gtMs - 1000, 0},
		{"at_genesis", gtMs, 0},
		{"one_interval", gtMs + 800, 1},
		{"one_slot", gtMs + uint64(MillisecondsPerSlot), IntervalsPerSlot},
		{"two_slots_plus_2_intervals", gtMs + 2*uint64(MillisecondsPerSlot) + 1600, 2*IntervalsPerSlot + 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TotalIntervals(gt, tt.currentMs); got != tt.want {
				t.Fatalf("TotalIntervals(%d, %d) = %d, want %d", gt, tt.currentMs, got, tt.want)
			}
		})
	}
}

func TestIntervalsFromSlot(t *testing.T) {
	if IntervalsFromSlot(0) != 0 {
		t.Fatal("slot 0 should map to interval 0")
	}
	if IntervalsFromSlot(1) != IntervalsPerSlot {
		t.Fatalf("slot 1 should map to %d, got %d", IntervalsPerSlot, IntervalsFromSlot(1))
	}
	if IntervalsFromSlot(100) != 100*IntervalsPerSlot {
		t.Fatalf("slot 100 should map to %d", 100*IntervalsPerSlot)
	}
}

func TestIntervalsFromUnixTime(t *testing.T) {
	const gt = 1700000000
	if got := IntervalsFromUnixTime(gt-5, gt); got != 0 {
		t.Fatalf("pre-genesis should be 0, got %d", got)
	}
	if got := IntervalsFromUnixTime(gt, gt); got != 0 {
		t.Fatalf("at genesis should be 0, got %d", got)
	}
	// 1 second = 1000ms / 800ms-per-interval = 1 (integer div).
	if got := IntervalsFromUnixTime(gt+1, gt); got != 1 {
		t.Fatalf("+1 second should be 1 interval, got %d", got)
	}
	// 4 seconds = 1 slot = IntervalsPerSlot intervals.
	if got := IntervalsFromUnixTime(gt+SecondsPerSlot, gt); got != IntervalsPerSlot {
		t.Fatalf("+1 slot should be %d intervals, got %d", IntervalsPerSlot, got)
	}
}
