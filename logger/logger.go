package logger

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

const (
	reset  = "\033[0m"
	dim    = "\033[2m"
	red    = "\033[31m"
	green  = "\033[32m"
	yellow = "\033[33m"
	cyan   = "\033[36m"
	bold   = "\033[1m"
)

var (
	mu     sync.Mutex
	output io.Writer = os.Stderr
	quiet  bool
)

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
	return time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
}

func Info(component, format string, args ...any) {
	emit(green, "INFO", component, format, args...)
}

func Warn(component, format string, args ...any) {
	emit(yellow, "WARN", component, format, args...)
}

func Error(component, format string, args ...any) {
	emit(red, "ERROR", component, format, args...)
}

func SetOutput(w io.Writer) {
	mu.Lock()
	defer mu.Unlock()
	output = w
}

func SetQuiet(enabled bool) {
	mu.Lock()
	defer mu.Unlock()
	quiet = enabled
}

func IsQuiet() bool {
	mu.Lock()
	defer mu.Unlock()
	return quiet
}

func emit(levelColor, level, component, format string, args ...any) {
	mu.Lock()
	defer mu.Unlock()

	if quiet || output == nil {
		return
	}
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(output, "%s%s%s %s%s%s%s %s[%s]%s %s\n",
		dim, timestamp(), reset, bold, levelColor, level, reset, cyan, component, reset, msg)
}
