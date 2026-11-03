package store_test

import (
	"errors"
	"testing"

	"github.com/geanlabs/gean/internal/storage"
	"github.com/geanlabs/gean/internal/store"
)

func TestReserveSignature(t *testing.T) {
	msgA, msgB := [32]byte{0xa}, [32]byte{0xb}
	const validator = uint64(3)

	tests := []struct {
		name     string
		role     store.SigningRole
		other    uint64
		slot     uint64
		message  [32]byte
		conflict bool
	}{
		{name: "same slot same message", slot: 10, message: msgA},
		{name: "same slot different message", slot: 10, message: msgB, conflict: true},
		{name: "earlier slot", slot: 9, message: msgB, conflict: true},
		{name: "later slot", slot: 11, message: msgB},
		{name: "other key of the validator", role: store.SigningProposal, slot: 10, message: msgB},
		{name: "other validator", other: 1, slot: 10, message: msgB},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := makeTestStore()
			if err := s.ReserveSignature(store.SigningAttestation, validator, 10, msgA); err != nil {
				t.Fatal(err)
			}
			err := s.ReserveSignature(tt.role, validator+tt.other, tt.slot, tt.message)
			if got := errors.Is(err, store.ErrSigningConflict); got != tt.conflict {
				t.Fatalf("err=%v, want conflict=%v", err, tt.conflict)
			}
			if err != nil && !tt.conflict {
				t.Fatal(err)
			}
		})
	}
}

// The record must outlive the process: a restart is the case it exists for.
func TestReserveSignatureSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	open := func() (*storage.PebbleBackend, *store.ConsensusStore) {
		b, err := storage.NewPebbleBackend(dir)
		if err != nil {
			t.Fatal(err)
		}
		return b, store.NewConsensusStore(b)
	}

	b, s := open()
	if err := s.ReserveSignature(store.SigningAttestation, 0, 10, [32]byte{0xa}); err != nil {
		t.Fatal(err)
	}
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}

	b, s = open()
	defer b.Close()
	if err := s.ReserveSignature(store.SigningAttestation, 0, 10, [32]byte{0xb}); !errors.Is(err, store.ErrSigningConflict) {
		t.Fatalf("err=%v, want conflict after reopening", err)
	}
}

func BenchmarkReserveSignatureSync(b *testing.B) {
	backend, err := storage.NewPebbleBackend(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	defer backend.Close()
	s := store.NewConsensusStore(backend)
	for i := range b.N {
		if err := s.ReserveSignature(store.SigningAttestation, 0, uint64(i+1), [32]byte{}); err != nil {
			b.Fatal(err)
		}
	}
}
