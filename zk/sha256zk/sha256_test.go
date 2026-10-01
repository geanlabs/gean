package sha256

import (
	"bytes"
	stdsha256 "crypto/sha256"
	"testing"
)

// TestMatchesStdlib checks Sum256 and a hash written in two parts against
// crypto/sha256 on every length across the padding boundaries of three blocks.
func TestMatchesStdlib(t *testing.T) {
	data := make([]byte, 3*BlockSize+1)
	for i := range data {
		data[i] = byte(i*7 + 3)
	}
	for n := range len(data) {
		want := stdsha256.Sum256(data[:n])
		if got := Sum256(data[:n]); got != want {
			t.Fatalf("Sum256 of %d bytes = %x, want %x", n, got, want)
		}
		h := New()
		h.Write(data[:n/3])
		h.Write(data[n/3 : n])
		if got := h.Sum(nil); !bytes.Equal(got, want[:]) {
			t.Fatalf("New of %d bytes = %x, want %x", n, got, want)
		}
	}
}
