package logger

import (
	"bytes"
	"io"
	"strings"
	"testing"
	"time"
)

func TestTimestampUsesUTC(t *testing.T) {
	before := time.Now().UTC().Add(-time.Second)
	got := timestamp()
	parsed, err := time.Parse("2006-01-02T15:04:05.000Z", got)
	if err != nil {
		t.Fatalf("parse timestamp %q: %v", got, err)
	}
	after := time.Now().UTC().Add(time.Second)

	if parsed.Before(before) || parsed.After(after) {
		t.Fatalf("timestamp %q parsed as %s, outside UTC window %s..%s",
			got, parsed, before, after)
	}
}

func TestLogLevelsWriteFormattedLines(t *testing.T) {
	var buf bytes.Buffer
	restoreLogger(t, &buf, false)

	Info(Chain, "height=%d", 12)
	Warn(Network, "peer=%s", "abc")
	Error(Store, "failed=%v", true)

	got := buf.String()
	for _, want := range []string{
		"INFO",
		"[chain]",
		"height=12",
		"WARN",
		"[network]",
		"peer=abc",
		"ERROR",
		"[store]",
		"failed=true",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("log output %q does not contain %q", got, want)
		}
	}
	if lines := strings.Count(got, "\n"); lines != 3 {
		t.Fatalf("lines=%d, want 3 in %q", lines, got)
	}
}

func TestQuietSuppressesLogs(t *testing.T) {
	var buf bytes.Buffer
	restoreLogger(t, &buf, true)

	Info(Chain, "hidden")
	Warn(Chain, "hidden")
	Error(Chain, "hidden")

	if got := buf.String(); got != "" {
		t.Fatalf("quiet output=%q, want empty", got)
	}
}

func TestNilOutputSuppressesLogs(t *testing.T) {
	restoreLogger(t, nil, false)

	Info(Chain, "hidden")
	Warn(Chain, "hidden")
	Error(Chain, "hidden")
}

func restoreLogger(t *testing.T, w io.Writer, quiet bool) {
	t.Helper()
	mu.Lock()
	oldOutput := output
	mu.Unlock()
	oldQuiet := IsQuiet()

	SetOutput(w)
	SetQuiet(quiet)
	t.Cleanup(func() {
		SetOutput(oldOutput)
		SetQuiet(oldQuiet)
	})
}
