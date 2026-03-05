package bolt_test

import (
	"path/filepath"
	"testing"

	boltstore "github.com/geanlabs/gean/storage/bolt"
	"github.com/geanlabs/gean/types"
)

func newTestStore(t *testing.T) *boltstore.Store {
	t.Helper()
	s, err := boltstore.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open bolt store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestPutGetBlock(t *testing.T) {
	s := newTestStore(t)
	root := [32]byte{1}
	block := &types.Block{Slot: 5, Body: &types.BlockBody{}}

	s.PutBlock(root, block)

	got, ok := s.GetBlock(root)
	if !ok {
		t.Fatal("expected block to be found")
	}
	if got.Slot != 5 {
		t.Fatalf("block slot = %d, want 5", got.Slot)
	}
}

func TestPutGetState(t *testing.T) {
	s := newTestStore(t)
	root := [32]byte{2}
	state := &types.State{
		Slot:                     10,
		Config:                   &types.Config{GenesisTime: 1000},
		LatestJustified:          &types.Checkpoint{},
		LatestFinalized:          &types.Checkpoint{},
		LatestBlockHeader:        &types.BlockHeader{},
		JustifiedSlots:           []byte{0x01},
		JustificationsValidators: []byte{0x01},
	}

	s.PutState(root, state)

	got, ok := s.GetState(root)
	if !ok {
		t.Fatal("expected state to be found")
	}
	if got.Slot != 10 {
		t.Fatalf("state slot = %d, want 10", got.Slot)
	}
}

func TestPutGetSignedBlock(t *testing.T) {
	s := newTestStore(t)
	root := [32]byte{3}
	sb := &types.SignedBlockWithAttestation{
		Message: &types.BlockWithAttestation{
			Block: &types.Block{Slot: 7, Body: &types.BlockBody{}},
		},
	}

	s.PutSignedBlock(root, sb)

	got, ok := s.GetSignedBlock(root)
	if !ok {
		t.Fatal("expected signed block to be found")
	}
	if got.Message.Block.Slot != 7 {
		t.Fatalf("signed block slot = %d, want 7", got.Message.Block.Slot)
	}
}

func TestGetMissingBlockReturnsFalse(t *testing.T) {
	s := newTestStore(t)
	_, ok := s.GetBlock([32]byte{0xff})
	if ok {
		t.Fatal("expected missing block to return false")
	}
}

func TestGetMissingStateReturnsFalse(t *testing.T) {
	s := newTestStore(t)
	_, ok := s.GetState([32]byte{0xff})
	if ok {
		t.Fatal("expected missing state to return false")
	}
}

func TestGetMissingSignedBlockReturnsFalse(t *testing.T) {
	s := newTestStore(t)
	_, ok := s.GetSignedBlock([32]byte{0xff})
	if ok {
		t.Fatal("expected missing signed block to return false")
	}
}

func TestGetAllBlocksCopiesMap(t *testing.T) {
	s := newTestStore(t)
	root := [32]byte{1}
	block := &types.Block{Slot: 1, Body: &types.BlockBody{}}
	s.PutBlock(root, block)

	all := s.GetAllBlocks()
	delete(all, root)

	_, ok := s.GetBlock(root)
	if !ok {
		t.Fatal("deleting from GetAllBlocks result should not affect store")
	}
}

func TestGetAllStatesCopiesMap(t *testing.T) {
	s := newTestStore(t)
	root := [32]byte{1}
	state := &types.State{
		Slot:                     1,
		Config:                   &types.Config{GenesisTime: 1000},
		LatestJustified:          &types.Checkpoint{},
		LatestFinalized:          &types.Checkpoint{},
		LatestBlockHeader:        &types.BlockHeader{},
		JustifiedSlots:           []byte{0x01},
		JustificationsValidators: []byte{0x01},
	}
	s.PutState(root, state)

	all := s.GetAllStates()
	delete(all, root)

	_, ok := s.GetState(root)
	if !ok {
		t.Fatal("deleting from GetAllStates result should not affect store")
	}
}

func TestFCMetadataRoundTrip(t *testing.T) {
	s := newTestStore(t)

	meta := &boltstore.FCMetadata{
		Head:          [32]byte{0xaa},
		SafeTarget:    [32]byte{0xbb},
		JustifiedRoot: [32]byte{0xcc},
		JustifiedSlot: 42,
		FinalizedRoot: [32]byte{0xdd},
		FinalizedSlot: 30,
		GenesisTime:   1700000000,
		Time:          1700000168,
		NumValidators: 64,
	}

	atts := map[uint64]*types.SignedAttestation{
		0: {
			ValidatorID: 0,
			Message: &types.AttestationData{
				Slot:   10,
				Head:   &types.Checkpoint{Root: [32]byte{1}, Slot: 10},
				Target: &types.Checkpoint{Root: [32]byte{2}, Slot: 8},
				Source: &types.Checkpoint{Root: [32]byte{3}, Slot: 4},
			},
		},
	}

	if err := s.PersistFCState(meta, atts); err != nil {
		t.Fatalf("persist fc state: %v", err)
	}

	got, err := s.LoadFCMetadata()
	if err != nil {
		t.Fatalf("load fc metadata: %v", err)
	}
	if got == nil {
		t.Fatal("expected metadata, got nil")
	}
	if got.Head != meta.Head {
		t.Fatalf("head mismatch: got %x, want %x", got.Head, meta.Head)
	}
	if got.JustifiedSlot != 42 {
		t.Fatalf("justified slot = %d, want 42", got.JustifiedSlot)
	}
	if got.FinalizedSlot != 30 {
		t.Fatalf("finalized slot = %d, want 30", got.FinalizedSlot)
	}
	if got.GenesisTime != 1700000000 {
		t.Fatalf("genesis time = %d, want 1700000000", got.GenesisTime)
	}
	if got.NumValidators != 64 {
		t.Fatalf("num validators = %d, want 64", got.NumValidators)
	}
}

func TestAttestationsRoundTrip(t *testing.T) {
	s := newTestStore(t)

	atts := map[uint64]*types.SignedAttestation{
		5: {
			ValidatorID: 5,
			Message: &types.AttestationData{
				Slot:   20,
				Head:   &types.Checkpoint{Root: [32]byte{0x10}, Slot: 20},
				Target: &types.Checkpoint{Root: [32]byte{0x20}, Slot: 16},
				Source: &types.Checkpoint{Root: [32]byte{0x30}, Slot: 12},
			},
		},
		9: {
			ValidatorID: 9,
			Message: &types.AttestationData{
				Slot:   21,
				Head:   &types.Checkpoint{Root: [32]byte{0x11}, Slot: 21},
				Target: &types.Checkpoint{Root: [32]byte{0x21}, Slot: 16},
				Source: &types.Checkpoint{Root: [32]byte{0x31}, Slot: 12},
			},
		},
	}

	meta := &boltstore.FCMetadata{NumValidators: 10}
	if err := s.PersistFCState(meta, atts); err != nil {
		t.Fatalf("persist: %v", err)
	}

	got, err := s.LoadAttestations()
	if err != nil {
		t.Fatalf("load attestations: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("attestation count = %d, want 2", len(got))
	}
	if got[5].Message.Slot != 20 {
		t.Fatalf("attestation 5 slot = %d, want 20", got[5].Message.Slot)
	}
	if got[9].Message.Slot != 21 {
		t.Fatalf("attestation 9 slot = %d, want 21", got[9].Message.Slot)
	}
}

func TestLoadFCMetadataFreshDB(t *testing.T) {
	s := newTestStore(t)
	meta, err := s.LoadFCMetadata()
	if err != nil {
		t.Fatalf("load from fresh db: %v", err)
	}
	if meta != nil {
		t.Fatal("expected nil metadata from fresh db")
	}
}

func TestClose(t *testing.T) {
	s, err := boltstore.New(filepath.Join(t.TempDir(), "close.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}
