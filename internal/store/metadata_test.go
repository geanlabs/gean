package store_test

import (
	"testing"
	"time"

	"github.com/geanlabs/gean/internal/storage"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

func TestMetadataRoundtrip(t *testing.T) {
	s := makeTestStore()

	s.SetTime(42)
	if s.Time() != 42 {
		t.Fatalf("time: expected 42, got %d", s.Time())
	}

	var root [32]byte
	root[0] = 0xab
	s.SetHead(root)
	if s.Head() != root {
		t.Fatal("head mismatch")
	}

	cp := makeCheckpoint(0xcd, 10)
	s.SetLatestJustified(cp)
	got := s.LatestJustified()
	if got.Root != cp.Root || got.Slot != cp.Slot {
		t.Fatal("justified mismatch")
	}

	cp2 := makeCheckpoint(0xef, 5)
	s.SetLatestFinalized(cp2)
	got2 := s.LatestFinalized()
	if got2.Root != cp2.Root || got2.Slot != cp2.Slot {
		t.Fatal("finalized mismatch")
	}
}

func TestConfigNilIgnored(t *testing.T) {
	s := store.NewConsensusStore(failingWriteBackend{InMemoryBackend: storage.NewInMemoryBackend()})
	s.SetConfig(nil)
	if got := s.Config(); got == nil {
		t.Fatal("Config should return an empty config on read failure")
	}
}

func TestWriteFailureDoesNotPersistMetadata(t *testing.T) {
	s := store.NewConsensusStore(failingWriteBackend{InMemoryBackend: storage.NewInMemoryBackend()})
	s.SetTime(99)
	if got := s.Time(); got != 0 {
		t.Fatalf("time after failed write=%d, want 0", got)
	}
}

func TestPutMetadataReturnsWriteErrors(t *testing.T) {
	s := store.NewConsensusStore(failingWriteBackend{InMemoryBackend: storage.NewInMemoryBackend()})
	if err := s.PutTime(99); err == nil {
		t.Fatal("expected PutTime write error")
	}
	if err := s.PutHead([32]byte{0x01}); err == nil {
		t.Fatal("expected PutHead write error")
	}
	if err := s.PutConfig(nil); err == nil {
		t.Fatal("expected PutConfig nil config error")
	}
}

func TestOnTickReturnsWhenTimeWriteFails(t *testing.T) {
	s := store.NewConsensusStore(failingWriteBackend{InMemoryBackend: storage.NewInMemoryBackend()})
	done := make(chan struct{})
	go func() {
		store.OnTick(s, types.MillisecondsPerInterval*10, false)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("OnTick did not return after metadata write failure")
	}
}

func TestSetConfigRoundtrip(t *testing.T) {
	s := makeTestStore()
	s.SetConfig(&types.ChainConfig{GenesisTime: 1234})
	if got := s.Config().GenesisTime; got != 1234 {
		t.Fatalf("genesis time=%d, want 1234", got)
	}
}
