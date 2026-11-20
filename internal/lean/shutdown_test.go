package lean

import (
	"net"
	"slices"
	"sync"
	"testing"
)

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

// A taken port fails startup, and an address already bound is released.
func TestListenHTTPFailsOnTakenPort(t *testing.T) {
	taken := listenLocal(t)
	defer taken.Close()
	free := listenLocal(t)
	apiPort := free.Addr().(*net.TCPAddr).Port
	free.Close()

	cfg := Config{HTTPAddr: "127.0.0.1", APIPort: apiPort, MetricsPort: taken.Addr().(*net.TCPAddr).Port}
	if _, err := listenHTTP(cfg, nil, nil, nil); err == nil {
		t.Fatal("listenHTTP succeeded on a taken metrics port")
	}
	l, err := net.Listen("tcp", cfg.apiAddress())
	if err != nil {
		t.Fatalf("api port still held after the failed start: %v", err)
	}
	l.Close()
}
