package attestation_test

import (
	"testing"

	"github.com/geanlabs/gean/internal/attestation"
	"github.com/geanlabs/gean/internal/types"
)

func TestProduceAttestationDataNilWhenHeadStateMissing(t *testing.T) {
	s := makeValidationStore()
	s.SetHead([32]byte{0xAA})
	if got := attestation.ProduceAttestationData(s, 5); got != nil {
		t.Fatalf("expected nil when head state missing, got %+v", got)
	}
}

// At genesis (head state's LatestBlockHeader.Slot == 0) the attestation source
// root is rewritten to the head root rather than the stored justified root.
func TestProduceAttestationDataGenesisSource(t *testing.T) {
	s := makeValidationStore()
	head := [32]byte{0xAA}
	justifiedRoot := [32]byte{0xBB}

	s.SetHead(head)
	s.SetSafeTarget(head)
	s.SetLatestJustified(&types.Checkpoint{Root: justifiedRoot, Slot: 2})
	s.SetLatestFinalized(&types.Checkpoint{Slot: 0})
	s.InsertBlockHeader(head, &types.BlockHeader{Slot: 0})
	s.InsertState(head, &types.State{
		Config:                   &types.ChainConfig{GenesisTime: 1000},
		LatestBlockHeader:        &types.BlockHeader{Slot: 0}, // genesis
		LatestJustified:          &types.Checkpoint{Root: justifiedRoot, Slot: 2},
		LatestFinalized:          &types.Checkpoint{},
		JustifiedSlots:           types.NewBitlistSSZ(0),
		JustificationsValidators: types.NewBitlistSSZ(0),
	})

	data := attestation.ProduceAttestationData(s, 5)
	if data == nil {
		t.Fatal("expected attestation data")
	}
	if data.Source.Root != head {
		t.Fatalf("genesis source root = %x, want head %x", data.Source.Root, head)
	}
	if data.Source.Slot != 2 {
		t.Fatalf("source slot = %d, want justified slot 2", data.Source.Slot)
	}
	if data.Slot != 5 {
		t.Fatalf("data slot = %d, want 5", data.Slot)
	}
}

// Post-genesis the source is the stored justified checkpoint unchanged.
func TestProduceAttestationDataNormalSource(t *testing.T) {
	s := makeValidationStore()
	head := [32]byte{0xAA}
	justifiedRoot := [32]byte{0xBB}

	s.SetHead(head)
	s.SetSafeTarget(head)
	s.SetLatestJustified(&types.Checkpoint{Root: justifiedRoot, Slot: 4})
	s.SetLatestFinalized(&types.Checkpoint{Slot: 6})
	s.InsertBlockHeader(head, &types.BlockHeader{Slot: 6})
	s.InsertState(head, &types.State{
		Config:                   &types.ChainConfig{GenesisTime: 1000},
		LatestBlockHeader:        &types.BlockHeader{Slot: 6}, // non-genesis
		LatestJustified:          &types.Checkpoint{Root: justifiedRoot, Slot: 4},
		LatestFinalized:          &types.Checkpoint{Slot: 6},
		JustifiedSlots:           types.NewBitlistSSZ(0),
		JustificationsValidators: types.NewBitlistSSZ(0),
	})

	data := attestation.ProduceAttestationData(s, 6)
	if data == nil {
		t.Fatal("expected attestation data")
	}
	if data.Source.Root != justifiedRoot {
		t.Fatalf("source root = %x, want justified %x", data.Source.Root, justifiedRoot)
	}
	if data.Head.Root != head {
		t.Fatalf("head root = %x, want %x", data.Head.Root, head)
	}
}
