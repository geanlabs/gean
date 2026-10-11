package zkvectors

import (
	"bytes"
	"testing"

	"github.com/geanlabs/gean/internal/zkstf"
)

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

// TestMutationsAreDeterministicAndNamed pins the vector set to its seed and
// checks every Go failure it provokes has a kind the Rust port can match.
func TestMutationsAreDeterministicAndNamed(t *testing.T) {
	cases, err := Suite()
	if err != nil {
		t.Fatal(err)
	}
	a, err := Mutations(cases, 7, 120)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Mutations(cases, 7, 120)
	if err != nil {
		t.Fatal(err)
	}
	var accepted int
	for i := range a {
		if a[i].Name != b[i].Name || !bytes.Equal(a[i].Bytes, b[i].Bytes) {
			t.Fatalf("%s: differs between runs", a[i].Name)
		}
		_, err := zkstf.Apply(a[i].Bytes)
		if err == nil {
			accepted++
			continue
		}
		if _, kerr := zkstf.ErrorKind(err); kerr != nil {
			t.Fatalf("%s: %v", a[i].Name, kerr)
		}
	}
	if accepted < len(a)/3 {
		t.Fatalf("only %d of %d mutations apply; the vote paths are barely reached", accepted, len(a))
	}
}
