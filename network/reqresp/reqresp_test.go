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

func TestResponseCodeRoundTrip(t *testing.T) {
	var buf bytes.Buffer

	// Write success + status payload (simulates server response).
	buf.WriteByte(ResponseSuccess)
	in := Status{
		Finalized: &types.Checkpoint{Root: [32]byte{0x01}, Slot: 1},
		Head:      &types.Checkpoint{Root: [32]byte{0x02}, Slot: 2},
	}
	if err := writeStatus(&buf, in); err != nil {
		t.Fatalf("writeStatus: %v", err)
	}

	// Read back: code then payload (simulates client).
	code, err := readResponseCode(&buf)
	if err != nil {
		t.Fatalf("readResponseCode: %v", err)
	}
	if code != ResponseSuccess {
		t.Fatalf("expected success code 0x00, got 0x%02x", code)
	}
	out, err := readStatus(&buf)
	if err != nil {
		t.Fatalf("readStatus: %v", err)
	}
	if out.Finalized.Slot != 1 || out.Head.Slot != 2 {
		t.Fatal("status payload mismatch after response code")
	}
}

func TestResponseCodeError(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteByte(ResponseServerError)

	code, err := readResponseCode(&buf)
	if err != nil {
		t.Fatalf("readResponseCode: %v", err)
	}
	if code != ResponseServerError {
		t.Fatalf("expected error code 0x02, got 0x%02x", code)
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
