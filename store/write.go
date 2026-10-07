package store

import (
	"fmt"

	"github.com/geanlabs/gean/db"
	"github.com/geanlabs/gean/logger"
)

func (s *ConsensusStore) putOne(table db.Table, key, value []byte, label string) error {
	wb, err := s.beginWrite(label)
	if err != nil {
		return err
	}
	if err := wb.PutBatch(table, []db.KV{{Key: key, Value: value}}); err != nil {
		return fmt.Errorf("%s: put: %w", label, err)
	}
	if err := wb.Commit(); err != nil {
		return fmt.Errorf("%s: commit: %w", label, err)
	}
	return nil
}

func commitDeletes(wb db.WriteBatch, label string) bool {
	if wb == nil {
		logger.Error(logger.Store, "%s: write batch is nil", label)
		return false
	}
	if err := wb.Commit(); err != nil {
		logger.Error(logger.Store, "%s: commit failed: %v", label, err)
		return false
	}
	return true
}
