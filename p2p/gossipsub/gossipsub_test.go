package gossipsub

import (
	"testing"
)

func TestDefaultParams(t *testing.T) {
	params := DefaultParams()

	if params.D != 8 {
		t.Errorf("D = %d, want 8", params.D)
	}
	if params.DLow != 6 {
		t.Errorf("DLow = %d, want 6", params.DLow)
	}
	if params.DHigh != 12 {
		t.Errorf("DHigh = %d, want 12", params.DHigh)
	}
	if params.ValidationMode != "strict_no_sign" {
		t.Errorf("ValidationMode = %s, want strict_no_sign", params.ValidationMode)
	}
	// SeenTTL = SECONDS_PER_SLOT * JUSTIFICATION_LOOKBACK_SLOTS * 2 = 4 * 3 * 2 = 24
	if params.SeenTTL != 24 {
		t.Errorf("SeenTTL = %d, want 24", params.SeenTTL)
	}
}
