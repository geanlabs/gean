package store

import (
	"fmt"

	"github.com/geanlabs/gean/internal/storage"
)

func (s *ConsensusStore) beginRead(label string) (storage.ReadView, error) {
	if s == nil || s.Backend == nil {
		return nil, fmt.Errorf("%s: store backend is nil", label)
	}
	rv, err := s.Backend.BeginRead()
	if err != nil {
		return nil, fmt.Errorf("%s: begin read: %w", label, err)
	}
	if rv == nil {
		return nil, fmt.Errorf("%s: read view is nil", label)
	}
	return rv, nil
}

func (s *ConsensusStore) beginWrite(label string) (storage.WriteBatch, error) {
	if s == nil || s.Backend == nil {
		return nil, fmt.Errorf("%s: store backend is nil", label)
	}
	wb, err := s.Backend.BeginWrite()
	if err != nil {
		return nil, fmt.Errorf("%s: begin write: %w", label, err)
	}
	if wb == nil {
		return nil, fmt.Errorf("%s: write batch is nil", label)
	}
	return wb, nil
}
