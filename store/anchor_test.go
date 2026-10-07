package store_test

import (
	"testing"
	"time"

	"github.com/geanlabs/gean/db"
	"github.com/geanlabs/gean/store"
	"github.com/geanlabs/gean/types"
)

func anchorState() *types.State {
	return &types.State{
		Config:                   &types.ChainConfig{GenesisTime: 1},
		LatestBlockHeader:        &types.BlockHeader{},
		LatestJustified:          &types.Checkpoint{},
		LatestFinalized:          &types.Checkpoint{},
		JustifiedSlots:           types.NewBitlistSSZ(0),
		JustificationsValidators: types.NewBitlistSSZ(0),
		Validators:               []*types.Validator{{}},
	}
}

func TestInitFromGenesisPersistsAnchor(t *testing.T) {
	s := makeTestStore()
	state := anchorState()
	bodyRoot, err := (&types.BlockBody{}).HashTreeRoot()
	if err != nil {
		t.Fatalf("hash empty body: %v", err)
	}
	state.LatestBlockHeader.BodyRoot = bodyRoot // as genesis.GenesisState sets it
	root, err := s.InitFromGenesis(state)
	if err != nil {
		t.Fatalf("initialize store: %v", err)
	}
	if s.Head() != root || s.SafeTarget() != root {
		t.Fatalf("head=%x safe target=%x, want %x", s.Head(), s.SafeTarget(), root)
	}
	if s.LatestJustified().Root != root || s.LatestFinalized().Root != root {
		t.Fatal("anchor is not justified and finalized")
	}
	header := s.GetBlockHeader(root)
	signed := s.GetSignedBlock(root)
	if header == nil || s.GetState(root) == nil || signed == nil {
		t.Fatal("anchor header, state or block not persisted")
	}
	// The stored genesis block must hash to the anchor root, which requires its
	// state root to have been filled in before the block was built.
	blockRoot, err := signed.Block.HashTreeRoot()
	if err != nil {
		t.Fatalf("hash genesis block: %v", err)
	}
	if blockRoot != root || header.StateRoot == types.ZeroRoot {
		t.Fatalf("genesis block root=%x state_root=%x, want root %x and a state root", blockRoot, header.StateRoot, root)
	}
}

func TestInitFromAnchorRejectsMalformedInput(t *testing.T) {
	if _, err := makeTestStore().InitFromAnchor(&types.State{}, nil); err == nil {
		t.Fatal("expected malformed state error")
	}
	s := store.NewConsensusStore(failingWriteBackend{InMemoryBackend: db.NewInMemoryBackend()})
	if _, err := s.InitFromGenesis(anchorState()); err == nil {
		t.Fatal("expected write error")
	}
}

func TestRecoverTime(t *testing.T) {
	const genesis = 1_000_000
	genesisTime := time.Unix(genesis, 0)
	tests := []struct {
		name   string
		stale  uint64
		now    time.Time
		genSec uint64
		want   uint64
	}{
		{"post genesis", 0, genesisTime.Add(10 * time.Second), genesis, 10_000 / types.MillisecondsPerInterval},
		{"pre genesis resets", 999, genesisTime.Add(-time.Hour), genesis, 0},
		{"overwrites stale value", 1, genesisTime.Add(time.Minute), genesis, 60_000 / types.MillisecondsPerInterval},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := makeTestStore()
			s.SetTime(tt.stale)
			if err := s.RecoverTime(tt.genSec, tt.now); err != nil {
				t.Fatalf("recover time: %v", err)
			}
			if got := s.Time(); got != tt.want {
				t.Fatalf("time=%d, want %d", got, tt.want)
			}
		})
	}
	if err := makeTestStore().RecoverTime(^uint64(0), genesisTime); err == nil {
		t.Fatal("expected overflow error")
	}
}
