package httpapi

import (
	"errors"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/geanlabs/gean/internal/core/lifecycle"
)

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
	srv := &Server{
		Name: "api",
		server: &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			close(entered)
			<-release
			_, _ = io.WriteString(w, "ok")
		})},
		listener: listenLocal(t),
	}
	var tasks lifecycle.Group
	Serve([]*Server{srv}, &tasks)

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
		srv.Shutdown()
		close(shutDown)
	}()
	// Only detects an early return; passing never depends on the window.
	select {
	case <-shutDown:
		t.Fatal("shutdown returned while a request was still running")
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	<-shutDown
	if err := <-responded; err != nil {
		t.Fatalf("drained request failed: %v", err)
	}
	tasks.Close()
}

// A server that stops for a reason other than shutdown reports to run.
func TestServeHTTPReportsServeFailure(t *testing.T) {
	l := listenLocal(t)
	l.Close() // Serve fails at once on a closed listener
	var tasks lifecycle.Group
	failed := Serve([]*Server{{Name: "api", server: &http.Server{}, listener: l}}, &tasks)

	if err := <-failed; err == nil || errors.Is(err, http.ErrServerClosed) {
		t.Fatalf("failure=%v, want a serve error", err)
	}
	tasks.Close()
}

// Shutdown is not a failure.
func TestServeHTTPShutdownIsNotFailure(t *testing.T) {
	srv := &Server{Name: "api", server: &http.Server{}, listener: listenLocal(t)}
	var tasks lifecycle.Group
	failed := Serve([]*Server{srv}, &tasks)
	srv.Shutdown()
	tasks.Close()

	select {
	case err := <-failed:
		t.Fatalf("shutdown reported as failure: %v", err)
	default:
	}
}
