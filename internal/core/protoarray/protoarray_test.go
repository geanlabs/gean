package protoarray

import "testing"

func root(n byte) [32]byte { return [32]byte{n} }

func TestProtoArrayPartialValueGuards(t *testing.T) {
	var nilArray *Array
	if nilArray.Len() != 0 {
		t.Fatal("nil protoarray length should be 0")
	}
	if nilArray.Nodes() != nil {
		t.Fatal("nil protoarray nodes should be nil")
	}
	nilArray.OnBlock(1, root(2), root(1))
	nilArray.ApplyScoreChanges([]int64{1}, 0)
	if got := nilArray.FindHead(root(1)); got != root(1) {
		t.Fatalf("nil protoarray head=%x, want justified root", got[:4])
	}
	nilArray.Prune(root(1))

	var zero Array
	anchor := root(1)
	zero.OnBlock(0, anchor, [32]byte{})
	if zero.Len() != 1 {
		t.Fatalf("zero protoarray len=%d, want 1", zero.Len())
	}
	if got := zero.FindHead(anchor); got != anchor {
		t.Fatalf("zero protoarray head=%x, want anchor", got[:4])
	}
}
