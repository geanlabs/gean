package testdriver

import (
	"sync"
)

type Session struct {
	mu     sync.Mutex
	replay *Replay
}

func NewSession() *Session {
	return &Session{}
}
