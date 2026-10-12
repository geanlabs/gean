package main

import (
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"slices"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/geanlabs/gean/internal/lifecycle"
)

// earlyReturnWindow is how long a test watches for a shutdown step that
// returns too early. It only detects that bug; no test needs it to pass.
const earlyReturnWindow = 100 * time.Millisecond

type stepLog struct {
	mu    sync.Mutex
	steps []string
}

func (l *stepLog) add(step string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.steps = append(l.steps, step)
}

func (l *stepLog) get() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.steps)
}

func (l *stepLog) components() *components {
	return &components{
		cancel:       func() { l.add("cancel") },
		closeNetwork: func() { l.add("network") },
		waitSync:     func() { l.add("sync") },
		waitEngine:   func() { l.add("engine") },
		closeStore:   func() error { l.add("store"); return nil },
		freeKeys:     func() { l.add("keys") },
	}
}

// The database and keys stay open until the engine, held mid-proof, returns.
func TestTeardownKeepsResourcesUntilEngineReturns(t *testing.T) {
	var log stepLog
	c := log.components()
	entered, release := make(chan struct{}), make(chan struct{})
	c.waitEngine = func() {
		close(entered)
		<-release
		log.add("engine")
	}

	done := make(chan struct{})
	go func() {
		c.teardown()
		close(done)
	}()
	<-entered
	for _, step := range log.get() {
		if step == "store" || step == "keys" {
			t.Fatalf("%s closed while the engine was still running: %v", step, log.get())
		}
	}
	close(release)
	<-done

	want := []string{"cancel", "network", "sync", "engine", "store", "keys"}
	if got := log.get(); !slices.Equal(got, want) {
		t.Fatalf("teardown order=%v, want %v", got, want)
	}
}

// A startup that failed before the engine and sync driver existed still
// closes the network before the database, and the database before the keys.
func TestTeardownAfterPartialStartup(t *testing.T) {
	var log stepLog
	c := log.components()
	c.waitSync, c.waitEngine = nil, nil
	c.teardown()

	want := []string{"cancel", "network", "store", "keys"}
	if got := log.get(); !slices.Equal(got, want) {
		t.Fatalf("teardown order=%v, want %v", got, want)
	}
}

func listenLocal(t *testing.T) net.Listener {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return l
}

// Shutdown of a server whose handlers read the database waits for an active
// request instead of cutting it off.
func TestHTTPShutdownDrainsActiveRequest(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	srv := &httpServer{
		name: "api",
		server: &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			close(entered)
			<-release
			io.WriteString(w, "ok")
		})},
		listener: listenLocal(t),
	}
	var tasks lifecycle.Group
	serveHTTP([]*httpServer{srv}, &tasks)

	responded := make(chan error, 1)
	go func() {
		resp, err := http.Get("http://" + srv.listener.Addr().String())
		if err == nil {
			resp.Body.Close()
		}
		responded <- err
	}()
	<-entered

	shutDown := make(chan struct{})
	go func() {
		srv.shutdown()
		close(shutDown)
	}()
	select {
	case <-shutDown:
		t.Fatal("shutdown returned while a request was still running")
	case <-time.After(earlyReturnWindow):
	}
	close(release)
	<-shutDown
	if err := <-responded; err != nil {
		t.Fatalf("drained request failed: %v", err)
	}
	tasks.Close()
}

// A taken port fails startup, and an address already bound is released.
func TestListenHTTPFailsOnTakenPort(t *testing.T) {
	taken := listenLocal(t)
	defer taken.Close()
	free := listenLocal(t)
	apiPort := free.Addr().(*net.TCPAddr).Port
	free.Close()

	cfg := config{HTTPAddr: "127.0.0.1", APIPort: apiPort, MetricsPort: taken.Addr().(*net.TCPAddr).Port}
	if _, err := listenHTTP(cfg, nil, nil, nil); err == nil {
		t.Fatal("listenHTTP succeeded on a taken metrics port")
	}
	l, err := net.Listen("tcp", cfg.apiAddress())
	if err != nil {
		t.Fatalf("api port still held after the failed start: %v", err)
	}
	l.Close()
}

// A server that stops for a reason other than shutdown reports to run.
func TestServeHTTPReportsServeFailure(t *testing.T) {
	l := listenLocal(t)
	l.Close() // Serve fails at once on a closed listener
	var tasks lifecycle.Group
	failed := serveHTTP([]*httpServer{{name: "api", server: &http.Server{}, listener: l}}, &tasks)

	if err := <-failed; err == nil || errors.Is(err, http.ErrServerClosed) {
		t.Fatalf("failure=%v, want a serve error", err)
	}
	tasks.Close()
}

// Shutdown is not a failure.
func TestServeHTTPShutdownIsNotFailure(t *testing.T) {
	srv := &httpServer{name: "api", server: &http.Server{}, listener: listenLocal(t)}
	var tasks lifecycle.Group
	failed := serveHTTP([]*httpServer{srv}, &tasks)
	srv.shutdown()
	tasks.Close()

	select {
	case err := <-failed:
		t.Fatalf("shutdown reported as failure: %v", err)
	default:
	}
}

// The first signal closes stop instead of killing the process, so a signal
// during startup still goes through teardown.
func TestFirstSignalClosesStop(t *testing.T) {
	sigCh := make(chan os.Signal, 1)
	stop := shutdownOn(sigCh)
	sigCh <- syscall.SIGINT
	<-stop
}

func TestWaitForShutdownReturnsServerFailure(t *testing.T) {
	failed := make(chan error, 1)
	want := errors.New("serve failed")
	failed <- want
	if err := waitForShutdown(make(chan struct{}), failed); !errors.Is(err, want) {
		t.Fatalf("err=%v, want %v", err, want)
	}

	stop := make(chan struct{})
	close(stop)
	if err := waitForShutdown(stop, make(chan error)); err != nil {
		t.Fatalf("err=%v on signal, want nil", err)
	}
}
