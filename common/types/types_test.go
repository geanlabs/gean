package types

import "testing"

func TestSlotToTime(t *testing.T) {
	genesis := uint64(1700000000)

	tests := []struct {
		slot Slot
		want uint64
	}{
		{0, 1700000000},
		{1, 1700000004},
		{100, 1700000400},
	}

	for _, tt := range tests {
		if got := SlotToTime(tt.slot, genesis); got != tt.want {
			t.Errorf("SlotToTime(%d) = %d, want %d", tt.slot, got, tt.want)
		}
	}
}

func TestTimeToSlot(t *testing.T) {
	genesis := uint64(1700000000)

	tests := []struct {
		time uint64
		want Slot
	}{
		{1700000000, 0},
		{1700000004, 1},
		{1700000007, 1},
		{1699999999, 0},
	}

	for _, tt := range tests {
		if got := TimeToSlot(tt.time, genesis); got != tt.want {
			t.Errorf("TimeToSlot(%d) = %d, want %d", tt.time, got, tt.want)
		}
	}
}

func TestRootIsZero(t *testing.T) {
	var zero Root
	if !zero.IsZero() {
		t.Error("zero root should be zero")
	}

	nonZero := Root{1}
	if nonZero.IsZero() {
		t.Error("non-zero root should not be zero")
	}
}

