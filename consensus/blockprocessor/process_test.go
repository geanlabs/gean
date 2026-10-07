package blockprocessor

import (
	"errors"
	"strings"
	"testing"

	"github.com/geanlabs/gean/consensus/statetransition"
	"github.com/geanlabs/gean/storage/db"
	"github.com/geanlabs/gean/storage/store"
	"github.com/geanlabs/gean/types"
)

func TestOnBlockRejectsNilStore(t *testing.T) {
	if err := OnBlock(nil, nil, nil, nil); err == nil {
		t.Fatal("expected nil store error")
	}
}

func TestOnBlockWithoutVerificationPersistsBlock(t *testing.T) {
	s, parentState, parentRoot := processorStoreWithParent(t)
	block := processorEmptyBlockWithStateRoot(t, parentState, parentRoot)

	if err := OnBlockWithoutVerification(s, nil, &types.SignedBlock{
		Block: block,
		Proof: &types.MultiMessageAggregate{},
	}); err != nil {
		t.Fatalf("process block: %v", err)
	}

	blockRoot, err := block.HashTreeRoot()
	if err != nil {
		t.Fatalf("hash block: %v", err)
	}
	if !s.HasState(blockRoot) {
		t.Fatal("post-state was not persisted")
	}
	header := s.GetBlockHeader(blockRoot)
	if header == nil {
		t.Fatal("block header was not persisted")
	}
	if header.StateRoot != block.StateRoot {
		t.Fatalf("header state root=0x%x, want 0x%x", header.StateRoot, block.StateRoot)
	}
	if signed := s.GetSignedBlock(blockRoot); signed == nil || signed.Block == nil {
		t.Fatal("signed block was not persisted")
	}
}

func TestOnBlockWithoutVerificationReturnsPersistenceError(t *testing.T) {
	backend := &writeFailingBackend{InMemoryBackend: db.NewInMemoryBackend()}
	s, parentState, parentRoot := processorStoreOnBackend(t, backend)
	block := processorEmptyBlockWithStateRoot(t, parentState, parentRoot)
	backend.failWrites = true

	err := OnBlockWithoutVerification(s, nil, &types.SignedBlock{
		Block: block,
		Proof: &types.MultiMessageAggregate{},
	})
	if err == nil {
		t.Fatal("expected persistence error")
	}
	if !strings.Contains(err.Error(), "begin write") {
		t.Fatalf("error=%v, want begin write context", err)
	}
}

// writeFailingBackend fails every write once failWrites is set, so a test can
// build a store normally and then make persistence fail.
type writeFailingBackend struct {
	*db.InMemoryBackend
	failWrites bool
}

func (b *writeFailingBackend) BeginWrite() (db.WriteBatch, error) {
	if b.failWrites {
		return nil, errors.New("write failed")
	}
	return b.InMemoryBackend.BeginWrite()
}

func processorStoreWithParent(t *testing.T) (*store.ConsensusStore, *types.State, [32]byte) {
	t.Helper()
	return processorStoreOnBackend(t, db.NewInMemoryBackend())
}

func processorStoreOnBackend(t *testing.T, backend db.Backend) (*store.ConsensusStore, *types.State, [32]byte) {
	t.Helper()

	parentState := &types.State{
		Config:                   &types.ChainConfig{GenesisTime: 1000},
		Slot:                     0,
		LatestBlockHeader:        &types.BlockHeader{Slot: 0},
		LatestJustified:          &types.Checkpoint{Slot: 0},
		LatestFinalized:          &types.Checkpoint{Slot: 0},
		JustifiedSlots:           types.NewBitlistSSZ(0),
		JustificationsValidators: types.NewBitlistSSZ(0),
		Validators:               []*types.Validator{{Index: 0}},
	}

	stateRoot, err := parentState.HashTreeRoot()
	if err != nil {
		t.Fatalf("hash parent state: %v", err)
	}
	parentState.LatestBlockHeader.StateRoot = stateRoot

	parentRoot, err := parentState.LatestBlockHeader.HashTreeRoot()
	if err != nil {
		t.Fatalf("hash parent header: %v", err)
	}

	s := store.NewConsensusStore(backend)
	s.InsertState(parentRoot, parentState)
	s.InsertBlockHeader(parentRoot, parentState.LatestBlockHeader)
	s.SetHead(parentRoot)
	s.SetLatestJustified(&types.Checkpoint{Root: parentRoot, Slot: 0})
	s.SetLatestFinalized(&types.Checkpoint{Root: parentRoot, Slot: 0})
	return s, parentState, parentRoot
}

func processorEmptyBlockWithStateRoot(t *testing.T, parentState *types.State, parentRoot [32]byte) *types.Block {
	t.Helper()

	block := &types.Block{
		Slot:          1,
		ProposerIndex: 0,
		ParentRoot:    parentRoot,
		Body:          &types.BlockBody{},
	}

	trial, err := parentState.Clone()
	if err != nil {
		t.Fatalf("clone state: %v", err)
	}
	if err := statetransition.ProcessSlots(trial, block.Slot); err != nil {
		t.Fatalf("process slots: %v", err)
	}
	if err := statetransition.ProcessBlock(trial, block); err != nil {
		t.Fatalf("process block: %v", err)
	}
	stateRoot, err := trial.HashTreeRoot()
	if err != nil {
		t.Fatalf("hash post state: %v", err)
	}
	block.StateRoot = stateRoot
	return block
}

func processorBlockRoot(t *testing.T, block *types.Block) [32]byte {
	t.Helper()

	root, err := block.HashTreeRoot()
	if err != nil {
		t.Fatalf("hash block: %v", err)
	}
	return root
}
