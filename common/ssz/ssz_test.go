package ssz

import (
	"testing"

	"github.com/devlongs/gean/common/types"
)

func TestHash(t *testing.T) {
	h := Hash([]byte("hello"))
	if h.IsZero() {
		t.Error("hash should not be zero")
	}

	h2 := Hash([]byte("hello"))
	if h != h2 {
		t.Error("hash should be deterministic")
	}
}

func TestHashNodes(t *testing.T) {
	a := types.Root{1}
	b := types.Root{2}

	h := HashNodes(a, b)
	if h.IsZero() {
		t.Error("hash should not be zero")
	}

	h2 := HashNodes(b, a)
	if h == h2 {
		t.Error("order should matter")
	}
}

func TestHashTreeRootUint64(t *testing.T) {
	r := HashTreeRootUint64(100)

	if r[0] != 100 {
		t.Errorf("expected first byte to be 100, got %d", r[0])
	}

	for i := 8; i < 32; i++ {
		if r[i] != 0 {
			t.Errorf("byte %d should be 0", i)
		}
	}
}

func TestMerkleize(t *testing.T) {
	chunk := types.Root{1, 2, 3}
	root := Merkleize([]types.Root{chunk}, 0)
	if root != chunk {
		t.Error("single chunk should be its own root")
	}

	a := types.Root{1}
	b := types.Root{2}
	root2 := Merkleize([]types.Root{a, b}, 0)
	expected := HashNodes(a, b)
	if root2 != expected {
		t.Error("two chunks should hash together")
	}

	empty := Merkleize(nil, 0)
	if empty != ZeroHash {
		t.Error("empty should return ZeroHash")
	}
}

func TestMixInLength(t *testing.T) {
	root := types.Root{1}
	mixed := MixInLength(root, 42)

	if mixed == root {
		t.Error("mixing should change the root")
	}

	mixed2 := MixInLength(root, 42)
	if mixed != mixed2 {
		t.Error("should be deterministic")
	}
}

func TestNextPowerOfTwo(t *testing.T) {
	tests := []struct {
		in, want int
	}{
		{0, 1},
		{1, 1},
		{2, 2},
		{3, 4},
		{5, 8},
		{8, 8},
		{9, 16},
	}

	for _, tt := range tests {
		if got := nextPowerOfTwo(tt.in); got != tt.want {
			t.Errorf("nextPowerOfTwo(%d) = %d, want %d", tt.in, got, tt.want)
		}
	}
}
