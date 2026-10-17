package logger

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
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
	emit("INFO", component, format, args...)
}

func Warn(component, format string, args ...any) {
	emit("WARN", component, format, args...)
}

func Error(component, format string, args ...any) {
	emit("ERROR", component, format, args...)
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

func emit(level, component, format string, args ...any) {
	mu.Lock()
	defer mu.Unlock()

	if quiet || output == nil {
		return
	}
	fmt.Fprintf(output, "%s %s [%s] %s\n", timestamp(), level, component, fmt.Sprintf(format, args...))
}
