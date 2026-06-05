package store_test

import (
	"testing"

	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

func TestAttestationSignatureInsertAndDelete(t *testing.T) {
	gsm := store.NewAttestationSignatureMap()
	var dr [32]byte
	dr[0] = 1
	data := &types.AttestationData{Slot: 5}
	var sig [types.SignatureSize]byte

	gsm.Insert(dr, data, 0, sig)
	gsm.Insert(dr, data, 1, sig)

	if gsm.Len() != 1 {
		t.Fatalf("expected 1 entry, got %d", gsm.Len())
	}
	snap := gsm.Snapshot()
	if len(snap[dr].Signatures) != 2 {
		t.Fatal("expected 2 signatures")
	}

	gsm.Delete([]store.AttestationDeleteKey{{ValidatorID: 0, DataRoot: dr}})
	snap = gsm.Snapshot()
	if len(snap[dr].Signatures) != 1 {
		t.Fatal("expected 1 signature after delete")
	}
}

func TestAttestationSignaturePruneBelow(t *testing.T) {
	gsm := store.NewAttestationSignatureMap()
	var sig [types.SignatureSize]byte
	for i := range uint64(5) {
		var dr [32]byte
		dr[0] = byte(i)
		gsm.Insert(dr, &types.AttestationData{Slot: i}, 0, sig)
	}

	pruned := gsm.PruneBelow(2)
	if pruned != 3 {
		t.Fatalf("expected 3 pruned, got %d", pruned)
	}
	if gsm.Len() != 2 {
		t.Fatalf("expected 2 remaining, got %d", gsm.Len())
	}
}

func TestAttestationSignatureRejectsNilData(t *testing.T) {
	gsm := store.NewAttestationSignatureMap()
	var sig [types.SignatureSize]byte

	gsm.Insert([32]byte{1}, nil, 0, sig)

	if gsm.Len() != 0 {
		t.Fatalf("nil data inserted entries=%d, want 0", gsm.Len())
	}
}

func TestAttestationSignatureMapZeroValueInsert(t *testing.T) {
	var gsm store.AttestationSignatureMap
	var sig [types.SignatureSize]byte
	gsm.Insert([32]byte{1}, &types.AttestationData{Slot: 1}, 0, sig)

	if gsm.Len() != 1 {
		t.Fatalf("zero-value map entries=%d, want 1", gsm.Len())
	}
}

func TestAttestationSignatureSnapshotReturnsCopy(t *testing.T) {
	gsm := store.NewAttestationSignatureMap()
	var dr [32]byte
	dr[0] = 1
	var sig [types.SignatureSize]byte
	data := &types.AttestationData{
		Slot:   1,
		Head:   &types.Checkpoint{Slot: 1, Root: [32]byte{0x01}},
		Target: &types.Checkpoint{Slot: 1, Root: [32]byte{0x02}},
		Source: &types.Checkpoint{Slot: 0, Root: [32]byte{0x03}},
	}

	gsm.Insert(dr, data, 0, sig)
	gsm.Insert(dr, data, 1, sig)
	data.Head.Root = [32]byte{0xff}

	snap := gsm.Snapshot()
	snap[dr].Signatures = nil
	snap[dr].Data.Head.Root = [32]byte{0xee}

	fresh := gsm.Snapshot()
	if len(fresh[dr].Signatures) != 2 {
		t.Fatalf("snapshot mutation changed store signatures=%d, want 2", len(fresh[dr].Signatures))
	}
	if fresh[dr].Data.Head.Root[0] != 0x01 {
		t.Fatal("snapshot mutation changed store attestation data")
	}
}
