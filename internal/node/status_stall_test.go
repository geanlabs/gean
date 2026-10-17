package node

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/geanlabs/gean/internal/logger"
)

// A proof that holds the prover past proverStuckAfter is reported once per
// episode, and the report resets when the prover is released.
func TestSampleStallsReportsAStuckProverOnce(t *testing.T) {
	var buf bytes.Buffer
	logger.SetOutput(&buf)
	logger.SetQuiet(false)
	t.Cleanup(func() {
		logger.SetOutput(os.Stderr)
		logger.SetQuiet(true)
	})
	stuckLogs := func() int { return strings.Count(buf.String(), "prover held for") }

	e := makeTestEngine()
	start := time.Now()
	if !e.ProvingGate.Acquire(context.Background(), true) {
		t.Fatal("acquire failed")
	}

	if reported := e.sampleStalls(start.Add(proverStuckAfter/2), false); reported || stuckLogs() != 0 {
		t.Fatalf("reported=%t logs=%d below the threshold", reported, stuckLogs())
	}
	reported := e.sampleStalls(start.Add(proverStuckAfter+time.Second), false)
	reported = e.sampleStalls(start.Add(proverStuckAfter+2*time.Second), reported)
	if !reported || stuckLogs() != 1 {
		t.Fatalf("reported=%t logs=%d past the threshold, want one report", reported, stuckLogs())
	}
	if !strings.Contains(buf.String(), "by a proposal") {
		t.Fatalf("report does not name the holder: %q", buf.String())
	}

	e.ProvingGate.Release(true)
	if reported = e.sampleStalls(start.Add(2*proverStuckAfter), reported); reported {
		t.Fatal("episode still marked reported after the prover was released")
	}
}
