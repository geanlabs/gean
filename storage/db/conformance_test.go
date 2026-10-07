package db_test

import (
	"testing"

	"github.com/geanlabs/gean/storage/db"
	"github.com/geanlabs/gean/storage/db/dbtest"
)

func TestInMemoryBackendConformance(t *testing.T) {
	dbtest.Run(t, func(*testing.T) db.Backend { return db.NewInMemoryBackend() })
}
