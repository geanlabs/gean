package testdriver

import (
	"sync"

	"github.com/geanlabs/gean/lean/consensus/forkchoice"
	"github.com/geanlabs/gean/lean/internal/pools"
	"github.com/geanlabs/gean/lean/internal/store"
)

type Session struct {
	mu         sync.Mutex
	store      *store.ConsensusStore
	pools      *pools.Attestations
	fc         *forkchoice.ForkChoice
	labelRoots map[string][32]byte
}

func NewSession() *Session {
	return &Session{labelRoots: make(map[string][32]byte)}
}
