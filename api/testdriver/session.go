package testdriver

import (
	"sync"

	"github.com/geanlabs/gean/forkchoice"
	"github.com/geanlabs/gean/store"
)

type Session struct {
	mu         sync.Mutex
	store      *store.ConsensusStore
	fc         *forkchoice.ForkChoice
	labelRoots map[string][32]byte
}

func NewSession() *Session {
	return &Session{labelRoots: make(map[string][32]byte)}
}
