package statetransition

import "testing"

func TestSlotIsJustifiableAfter(t *testing.T) {
	tests := []struct {
		slot, finalized uint64
		want            bool
	}{
		// Rule 1: delta <= 5
		{1, 0, true},  // delta=1
		{5, 0, true},  // delta=5
		{6, 1, true},  // delta=5

		// Rule 2: perfect squares
		{9, 0, true},   // delta=9 = 3^2
		{16, 0, true},  // delta=16 = 4^2
		{25, 0, true},  // delta=25 = 5^2
		{36, 0, true},  // delta=36 = 6^2
		{100, 0, true}, // delta=100 = 10^2

		// Rule 3: pronic numbers n*(n+1)
		{6, 0, true},   // delta=6 = 2*3 (also <= 5+1, but pronic)
		{12, 0, true},  // delta=12 = 3*4
		{20, 0, true},  // delta=20 = 4*5
		{30, 0, true},  // delta=30 = 5*6
		{42, 0, true},  // delta=42 = 6*7

		// NOT justifiable: delta > 5, not square, not pronic
		{7, 0, false},  // 7
		{8, 0, false},  // 8
		{10, 0, false}, // 10
		{11, 0, false}, // 11
		{13, 0, false}, // 13
		{14, 0, false}, // 14
		{15, 0, false}, // 15
		{17, 0, false}, // 17
		{18, 0, false}, // 18
		{19, 0, false}, // 19
		{21, 0, false}, // 21

		// Edge: slot <= finalized
		{0, 0, false},
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
