package main

import (
	"testing"
	"time"

	"github.com/geanlabs/gean/logger"
	"github.com/geanlabs/gean/node"
	"github.com/geanlabs/gean/storage"
	"github.com/geanlabs/gean/types"
)

func TestMain(m *testing.M) {
	logger.Quiet = true
	m.Run()
}

func newTestStore() *node.ConsensusStore {
	return node.NewConsensusStore(storage.NewInMemoryBackend())
}

func TestRecoverStoreTime_PostGenesis(t *testing.T) {
	s := newTestStore()
	nowMs := uint64(time.Now().UnixMilli())
	genesisSec := (nowMs / 1000) - 10 // genesis 10 seconds ago

	recoverStoreTime(s, genesisSec)

	got := s.Time()
	expectedMin := uint64(10*1000/types.MillisecondsPerInterval) - 1
	expectedMax := uint64(10*1000/types.MillisecondsPerInterval) + 1
	if got < expectedMin || got > expectedMax {
		t.Fatalf("intervals: expected ~%d, got %d", (expectedMin+expectedMax)/2, got)
	}
}

func TestRecoverStoreTime_PreGenesis(t *testing.T) {
	s := newTestStore()
	// Seed a non-zero time to verify the function overwrites it.
	s.SetTime(999)

	farFuture := uint64(time.Now().Unix()) + 86400
	recoverStoreTime(s, farFuture)

	if got := s.Time(); got != 0 {
		t.Fatalf("expected time=0 for pre-genesis wall clock, got %d", got)
	}
}

func TestRecoverStoreTime_OverwritesStaleValue(t *testing.T) {
	s := newTestStore()
	s.SetTime(1) // simulate a stale value persisted from a previous run

	genesisSec := uint64(time.Now().Unix()) - 60 // 60s ago → 75 intervals
	recoverStoreTime(s, genesisSec)

	if got := s.Time(); got < 70 {
		t.Fatalf("recovered time did not overwrite stale value: got %d, want >=70", got)
	}
}
