package statetransition

import "testing"

func TestSlotIsJustifiableAfter(t *testing.T) {
	tests := []struct {
		slot, finalized uint64
		want            bool
	}{
		{1, 0, true},
		{5, 0, true},
		{6, 1, true},
		{9, 0, true},
		{16, 0, true},
		{25, 0, true},
		{36, 0, true},
		{100, 0, true},
		{6, 0, true},
		{12, 0, true},
		{20, 0, true},
		{30, 0, true},
		{42, 0, true},
		{7, 0, false},
		{8, 0, false},
		{10, 0, false},
		{11, 0, false},
		{13, 0, false},
		{14, 0, false},
		{15, 0, false},
		{17, 0, false},
		{18, 0, false},
		{19, 0, false},
		{21, 0, false},
		{0, 0, true},
		{10, 10, true},
		{5, 10, false},
	}

	for _, tt := range tests {
		got := SlotIsJustifiableAfter(tt.slot, tt.finalized)
		if got != tt.want {
			t.Errorf("SlotIsJustifiableAfter(%d, %d) = %v, want %v",
				tt.slot, tt.finalized, got, tt.want)
		}
	}
}

func TestIsSlotJustifiedIgnoresBitlistTerminator(t *testing.T) {
	state := makeGenesisState(1)
	state.LatestFinalized.Slot = 0

	if IsSlotJustified(state, 0, 1) {
		t.Fatal("empty justified bitlist must not justify slot 1")
	}

	setSlotJustified(state, 0, 1)
	if !IsSlotJustified(state, 0, 1) {
		t.Fatal("explicitly justified slot 1 should be recognized")
	}
}
