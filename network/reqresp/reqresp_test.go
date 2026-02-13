package reqresp

import (
	"bytes"
	"testing"

	"github.com/geanlabs/gean/types"
)

func TestStatusSSZRoundTrip(t *testing.T) {
	var finalizedRoot, headRoot [32]byte
	for i := range finalizedRoot {
		finalizedRoot[i] = 0xaa
		headRoot[i] = 0xbb
	}

	in := Status{
		Finalized: &types.Checkpoint{Root: finalizedRoot, Slot: 3},
		Head:      &types.Checkpoint{Root: headRoot, Slot: 7},
	}

	var buf bytes.Buffer
	if err := writeStatus(&buf, in); err != nil {
		t.Fatalf("writeStatus: %v", err)
	}

	out, err := readStatus(&buf)
	if err != nil {
		t.Fatalf("readStatus: %v", err)
	}

	if out.Finalized.Slot != in.Finalized.Slot || out.Finalized.Root != in.Finalized.Root {
		t.Fatalf("finalized mismatch: got (%d,%x), want (%d,%x)",
			out.Finalized.Slot, out.Finalized.Root, in.Finalized.Slot, in.Finalized.Root)
	}
	if out.Head.Slot != in.Head.Slot || out.Head.Root != in.Head.Root {
		t.Fatalf("head mismatch: got (%d,%x), want (%d,%x)",
			out.Head.Slot, out.Head.Root, in.Head.Slot, in.Head.Root)
	}
}

func TestReadStatusRejectsInvalidLength(t *testing.T) {
	for _, n := range []int{79, 81} {
		var buf bytes.Buffer
		payload := make([]byte, n)
		if err := writeSnappyFrame(&buf, payload); err != nil {
			t.Fatalf("writeSnappyFrame(%d): %v", n, err)
		}

		if _, err := readStatus(&buf); err == nil {
			t.Fatalf("expected readStatus error for payload length %d", n)
		}
	}
}
