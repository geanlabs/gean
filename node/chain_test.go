package node

import (
	"testing"
	"time"

	"github.com/geanlabs/gean/db"
	"github.com/geanlabs/gean/store"
	"github.com/geanlabs/gean/types"
)

func TestBootstrapStoreRejectsPreDevnet5Database(t *testing.T) {
	s := store.NewConsensusStore(db.NewInMemoryBackend())
	root := [32]byte{0x01}
	s.SetHead(root)
	s.InsertBlockHeader(root, &types.BlockHeader{Slot: 1})
	s.InsertState(root, &types.State{})

	_, err := OpenChain(s, nil, "", time.Now())
	if err == nil {
		t.Fatal("expected incompatible data directory error")
	}
}
