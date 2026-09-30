package zkvectors

import "testing"

func TestSuiteCoversJustificationAndRejects(t *testing.T) {
	cases, err := Suite()
	if err != nil {
		t.Fatal(err)
	}
	var accept, reject, voted int
	finalized := false
	for _, c := range cases {
		if c.WantErr {
			reject++
			continue
		}
		accept++
		if len(c.Block.Body.Attestations) > 0 {
			voted++
		}
		if c.Pre.LatestFinalized.Slot > 0 {
			finalized = true
		}
	}
	if reject != 6 {
		t.Errorf("reject cases = %d, want 6", reject)
	}
	if accept < 40 || voted < 20 {
		t.Errorf("accept=%d voted=%d, want a substantial honest chain", accept, voted)
	}
	if !finalized {
		t.Error("no case starts from a finalized checkpoint past genesis")
	}
}
