package types

import "testing"

func TestBlockHeaderHashesToBlockRoot(t *testing.T) {
	block := &Block{Slot: 3, ProposerIndex: 1, ParentRoot: [32]byte{1}, StateRoot: [32]byte{2}, Body: &BlockBody{}}
	header, err := block.Header()
	if err != nil {
		t.Fatal(err)
	}
	headerRoot, err := header.HashTreeRoot()
	if err != nil {
		t.Fatal(err)
	}
	blockRoot, err := block.HashTreeRoot()
	if err != nil {
		t.Fatal(err)
	}
	if headerRoot != blockRoot {
		t.Fatalf("header root=0x%x, want block root 0x%x", headerRoot, blockRoot)
	}

	block.Body = nil
	if header, err = block.Header(); err != nil || header.BodyRoot != ZeroRoot {
		t.Fatalf("bodiless header=%+v err=%v, want a zero body root", header, err)
	}
}
