package logger

import (
	"fmt"
	"os"
	"time"
)

// ANSI color codes.
const (
	reset   = "\033[0m"
	dim     = "\033[2m"
	red     = "\033[31m"
	green   = "\033[32m"
	yellow  = "\033[33m"
	blue    = "\033[34m"
	magenta = "\033[35m"
	cyan    = "\033[36m"
	white   = "\033[37m"
	bold    = "\033[1m"
)

// Component tags matching lean client conventions.
const (
	Chain      = "chain"
	Validator  = "validator"
	Gossip     = "gossip"
	Network    = "network"
	Signature  = "signature"
	Forkchoice = "forkchoice"
	Sync       = "sync"
	Node       = "node"
	State      = "state"
	Store      = "store"
)

func timestamp() string {
	return time.Now().Format("2006-01-02T15:04:05.000Z")
}

// Info logs an info-level message with a component tag.
func Info(component, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "%s%s%s %s%sINFO%s %s[%s]%s %s\n",
		dim, timestamp(), reset, bold, green, reset, cyan, component, reset, msg)
}

// Warn logs a warning-level message with a component tag.
func Warn(component, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "%s%s%s %s%sWARN%s %s[%s]%s %s\n",
		dim, timestamp(), reset, bold, yellow, reset, cyan, component, reset, msg)
}

// Error logs an error-level message with a component tag.
func Error(component, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "%s%s%s %s%sERROR%s %s[%s]%s %s\n",
		dim, timestamp(), reset, bold, red, reset, cyan, component, reset, msg)
}
