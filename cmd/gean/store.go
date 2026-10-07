package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/geanlabs/gean/db"
	"github.com/geanlabs/gean/logger"
	"github.com/geanlabs/gean/store"
)

func openStore(dataDir string) (*db.PebbleBackend, *store.ConsensusStore, error) {
	absDataDir, err := filepath.Abs(dataDir)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve data dir: %w", err)
	}
	if err := os.MkdirAll(absDataDir, 0o755); err != nil {
		return nil, nil, fmt.Errorf("create data dir: %w", err)
	}
	logger.Info(logger.Node, "storage: %s", absDataDir)

	backend, err := db.NewPebbleBackend(absDataDir)
	if err != nil {
		return nil, nil, err
	}
	return backend, store.NewConsensusStore(backend), nil
}
