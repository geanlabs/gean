package main

import (
	"os"
	"testing"

	"github.com/geanlabs/gean/db"
	"github.com/geanlabs/gean/logger"
	"github.com/geanlabs/gean/store"
	"github.com/geanlabs/gean/types"
)

func TestMain(m *testing.M) {
	logger.SetQuiet(true)
	os.Exit(m.Run())
}

func newTestStore() *store.ConsensusStore {
	return store.NewConsensusStore(db.NewInMemoryBackend())
}

func TestBootstrapStoreRejectsPreDevnet5Database(t *testing.T) {
	s := newTestStore()
	root := [32]byte{0x01}
	s.SetHead(root)
	s.InsertBlockHeader(root, &types.BlockHeader{Slot: 1})
	s.InsertState(root, &types.State{})

	err := bootstrapStore(s, nil, "")
	if err == nil {
		t.Fatal("expected incompatible data directory error")
	}
}
