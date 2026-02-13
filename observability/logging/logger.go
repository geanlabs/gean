package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
	"time"
)

// Component names used as log source tags.
const (
	CompNode       = "node"
	CompValidator  = "validator"
	CompConsensus  = "consensus"
	CompForkChoice = "forkchoice"
	CompNetwork    = "network"
	CompGossip     = "gossip"
	CompReqResp    = "reqresp"
	CompMetrics    = "metrics"
)

// ANSI color codes.
const (
	reset   = "\033[0m"
	dim     = "\033[2m"
	red     = "\033[31m"
	yellow  = "\033[33m"
	cyan    = "\033[36m"
	green   = "\033[32m"
	magenta = "\033[35m"
)

var defaultLogger *slog.Logger
var once sync.Once

// Init sets up the global logger with the given level.
func Init(level slog.Level) {
	once.Do(func() {
		handler := &prettyHandler{
			out:   os.Stdout,
			level: level,
		}
		defaultLogger = slog.New(handler)
		slog.SetDefault(defaultLogger)
	})
}

// NewComponentLogger returns a logger tagged with a component name.
func NewComponentLogger(component string) *slog.Logger {
	if defaultLogger == nil {
		Init(slog.LevelInfo)
	}
	return defaultLogger.With(slog.String("comp", component))
}

// ShortHash returns the first 8 hex chars of a [32]byte hash.
func ShortHash(h [32]byte) string {
	return fmt.Sprintf("%x", h[:4])
}

// prettyHandler is a custom slog.Handler that produces colored, aligned output.
//
// Format:
//   2026-02-13 14:23:45.123 INF [node] message  key=value key=value
type prettyHandler struct {
	out   io.Writer
	level slog.Level
	attrs []slog.Attr
	group string
}

func (h *prettyHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *prettyHandler) Handle(_ context.Context, r slog.Record) error {
	// Suppress messages from libraries (no comp attr) unless they're errors.
	hasComp := false
	for _, a := range h.attrs {
		if a.Key == "comp" {
			hasComp = true
			break
		}
	}
	if !hasComp && r.Level < slog.LevelError {
		return nil
	}

	timestamp := r.Time.Format("2006-01-02 15:04:05.000")

	var levelStr string
	var levelColor string
	switch {
	case r.Level >= slog.LevelError:
		levelStr = "ERR"
		levelColor = red
	case r.Level >= slog.LevelWarn:
		levelStr = "WRN"
		levelColor = yellow
	case r.Level >= slog.LevelInfo:
		levelStr = "INF"
		levelColor = green
	default:
		levelStr = "DBG"
		levelColor = dim
	}

	// Extract component from pre-set attrs.
	comp := ""
	var filteredAttrs []slog.Attr
	for _, a := range h.attrs {
		if a.Key == "comp" {
			comp = a.Value.String()
		} else {
			filteredAttrs = append(filteredAttrs, a)
		}
	}

	compTag := ""
	if comp != "" {
		compTag = fmt.Sprintf(" %s[%s]%s", cyan, comp, reset)
	}

	// Build attribute string.
	attrStr := ""
	for _, a := range filteredAttrs {
		attrStr += fmt.Sprintf("  %s%s=%s%s", dim, a.Key, a.Value.String(), reset)
	}
	r.Attrs(func(a slog.Attr) bool {
		attrStr += fmt.Sprintf("  %s%s=%s%s", dim, a.Key, a.Value.String(), reset)
		return true
	})

	line := fmt.Sprintf("%s%s%s %s%-3s%s%s %s%s\n",
		dim, timestamp, reset,
		levelColor, levelStr, reset,
		compTag,
		r.Message,
		attrStr,
	)

	_, err := fmt.Fprint(h.out, line)
	return err
}

func (h *prettyHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make([]slog.Attr, len(h.attrs), len(h.attrs)+len(attrs))
	copy(newAttrs, h.attrs)
	newAttrs = append(newAttrs, attrs...)
	return &prettyHandler{out: h.out, level: h.level, attrs: newAttrs, group: h.group}
}

func (h *prettyHandler) WithGroup(name string) slog.Handler {
	return &prettyHandler{out: h.out, level: h.level, attrs: h.attrs, group: name}
}

// Banner prints the startup banner.
func Banner(version string) {
	if defaultLogger == nil {
		Init(slog.LevelInfo)
	}
	fmt.Println()
	fmt.Printf("  %sgean%s %s%s%s\n", magenta, reset, dim, version, reset)
	fmt.Printf("  %sLean Ethereum Go Client%s\n", dim, reset)
	fmt.Println()
}

// TimeSince returns a duration string since the given start time.
func TimeSince(start time.Time) string {
	d := time.Since(start)
	if d < time.Millisecond {
		return fmt.Sprintf("%dµs", d.Microseconds())
	}
	return fmt.Sprintf("%dms", d.Milliseconds())
}
