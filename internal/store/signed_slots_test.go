package store_test

import (
	"testing"

	"github.com/geanlabs/gean/internal/storage"
	"github.com/geanlabs/gean/internal/store"
)

func TestReserveSigningSlot(t *testing.T) {
	dir := t.TempDir()
	backend, err := storage.NewPebbleBackend(dir)
	if err != nil {
		t.Fatalf("open pebble: %v", err)
	}
	s := store.NewConsensusStore(backend)
	if err := s.ReserveSigningSlot(store.RoleAttestation, 5, []uint64{1, 2}); err != nil {
		t.Fatalf("first reservation: %v", err)
	}
	if err := backend.Close(); err != nil {
		t.Fatalf("close pebble: %v", err)
	}

	backend, err = storage.NewPebbleBackend(dir)
	if err != nil {
		t.Fatalf("reopen pebble: %v", err)
	}
	defer backend.Close()
	s = store.NewConsensusStore(backend)

	steps := []struct {
		name    string
		role    store.SigningRole
		slot    uint64
		ids     []uint64
		wantErr bool
	}{
		{"same slot after restart", store.RoleAttestation, 5, []uint64{2}, true},
		{"earlier slot", store.RoleAttestation, 4, []uint64{1}, true},
		{"other role keeps its own record", store.RoleProposal, 5, []uint64{1}, false},
		{"later slot", store.RoleAttestation, 6, []uint64{1}, false},
		{"refused when any validator already reserved", store.RoleAttestation, 6, []uint64{3, 1}, true},
		{"refusal recorded nothing for the others", store.RoleAttestation, 6, []uint64{3}, false},
	}
	// Steps run in order: each depends on the records the earlier ones left.
	for _, step := range steps {
		t.Run(step.name, func(t *testing.T) {
			err := s.ReserveSigningSlot(step.role, step.slot, step.ids)
			if (err != nil) != step.wantErr {
				t.Fatalf("error=%v, wantErr=%v", err, step.wantErr)
			}
		})
	}
}
