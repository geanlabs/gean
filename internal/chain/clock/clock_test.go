package clock

import (
	"testing"
	"time"

	"github.com/devylongs/gean/types"
)

// mockTime creates a time function that returns a fixed time.
func mockTime(unixSeconds int64) func() time.Time {
	return func() time.Time {
		return time.Unix(unixSeconds, 0)
	}
}

func TestCurrentSlot_BeforeGenesis(t *testing.T) {
	genesisTime := uint64(1000)
	clock := NewWithTimeFunc(genesisTime, mockTime(500)) // 500 seconds before genesis

	slot := clock.CurrentSlot()
	if slot != 0 {
		t.Errorf("CurrentSlot before genesis = %d, want 0", slot)
	}
}

func TestCurrentSlot_AtGenesis(t *testing.T) {
	genesisTime := uint64(1000)
	clock := NewWithTimeFunc(genesisTime, mockTime(1000)) // exactly at genesis

	slot := clock.CurrentSlot()
	if slot != 0 {
		t.Errorf("CurrentSlot at genesis = %d, want 0", slot)
	}
}

func TestCurrentSlot_AfterSlots(t *testing.T) {
	genesisTime := uint64(1000)
	tests := []struct {
		name     string
		nowTime  int64
		wantSlot types.Slot
	}{
		{"1 second after genesis", 1001, 0},
		{"3 seconds after genesis", 1003, 0},
		{"4 seconds after genesis (slot 1)", 1004, 1},
		{"8 seconds after genesis (slot 2)", 1008, 2},
		{"100 seconds after genesis (slot 25)", 1100, 25},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := NewWithTimeFunc(genesisTime, mockTime(tt.nowTime))
			slot := clock.CurrentSlot()
			if slot != tt.wantSlot {
				t.Errorf("CurrentSlot = %d, want %d", slot, tt.wantSlot)
			}
		})
	}
}

func TestCurrentInterval(t *testing.T) {
	genesisTime := uint64(1000)
	tests := []struct {
		name         string
		nowTime      int64
		wantInterval Interval
	}{
		{"at genesis (interval 0)", 1000, 0},
		{"1 second after genesis (interval 1)", 1001, 1},
		{"2 seconds after genesis (interval 2)", 1002, 2},
		{"3 seconds after genesis (interval 3)", 1003, 3},
		{"4 seconds after genesis (new slot, interval 0)", 1004, 0},
		{"5 seconds after genesis (interval 1)", 1005, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := NewWithTimeFunc(genesisTime, mockTime(tt.nowTime))
			interval := clock.CurrentInterval()
			if interval != tt.wantInterval {
				t.Errorf("CurrentInterval = %d, want %d", interval, tt.wantInterval)
			}
		})
	}
}

func TestTotalIntervals(t *testing.T) {
	genesisTime := uint64(1000)
	tests := []struct {
		name     string
		nowTime  int64
		wantTotal Interval
	}{
		{"before genesis", 500, 0},
		{"at genesis", 1000, 0},
		{"1 second after genesis", 1001, 1},
		{"4 seconds after genesis (1 slot)", 1004, 4},
		{"8 seconds after genesis (2 slots)", 1008, 8},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := NewWithTimeFunc(genesisTime, mockTime(tt.nowTime))
			total := clock.TotalIntervals()
			if total != tt.wantTotal {
				t.Errorf("TotalIntervals = %d, want %d", total, tt.wantTotal)
			}
		})
	}
}

func TestSlotStartTime(t *testing.T) {
	genesisTime := uint64(1000)
	clock := New(genesisTime)

	tests := []struct {
		slot     types.Slot
		wantTime uint64
	}{
		{0, 1000},
		{1, 1004},
		{2, 1008},
		{100, 1400},
	}

	for _, tt := range tests {
		t.Run("slot_"+string(rune(tt.slot+'0')), func(t *testing.T) {
			startTime := clock.SlotStartTime(tt.slot)
			if startTime != tt.wantTime {
				t.Errorf("SlotStartTime(%d) = %d, want %d", tt.slot, startTime, tt.wantTime)
			}
		})
	}
}

func TestIsBeforeGenesis(t *testing.T) {
	genesisTime := uint64(1000)

	tests := []struct {
		name       string
		nowTime    int64
		wantBefore bool
	}{
		{"500 seconds before genesis", 500, true},
		{"1 second before genesis", 999, true},
		{"at genesis", 1000, false},
		{"after genesis", 1001, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := NewWithTimeFunc(genesisTime, mockTime(tt.nowTime))
			isBefore := clock.IsBeforeGenesis()
			if isBefore != tt.wantBefore {
				t.Errorf("IsBeforeGenesis = %v, want %v", isBefore, tt.wantBefore)
			}
		})
	}
}

func TestNew(t *testing.T) {
	genesisTime := uint64(1704085200)
	clock := New(genesisTime)

	if clock.GenesisTime != genesisTime {
		t.Errorf("GenesisTime = %d, want %d", clock.GenesisTime, genesisTime)
	}
	if clock.timeFunc == nil {
		t.Error("timeFunc should not be nil")
	}
}
