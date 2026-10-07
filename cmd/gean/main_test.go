package main

import (
	"os"
	"testing"

	"github.com/geanlabs/gean/logger"
)

func TestMain(m *testing.M) {
	logger.SetQuiet(true)
	os.Exit(m.Run())
}
