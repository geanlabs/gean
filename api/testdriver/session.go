package testdriver

import (
	"github.com/geanlabs/gean/crypto"
	"sync"

	"github.com/geanlabs/gean/consensus/forkchoice"
	"github.com/geanlabs/gean/storage/store"
)

type Session struct {
	mu         sync.Mutex
	scheme     crypto.Scheme
	store      *store.ConsensusStore
	fc         *forkchoice.ForkChoice
	labelRoots map[string][32]byte
}

// NewSession runs test-driver steps, verifying signatures with scheme.
func NewSession(scheme crypto.Scheme) *Session {
	return &Session{scheme: scheme, labelRoots: make(map[string][32]byte)}
}
