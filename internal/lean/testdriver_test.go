package lean

import (
	"context"
	"net"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

// In test-driver mode Run serves the driver on the API port, and nothing else
// gean serves, until its context ends.
func TestRunTestDriverServesOnlyTheDriver(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, Config{TestDriver: true, HTTPAddr: "127.0.0.1", APIPort: port}) }()

	base := "http://" + net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	waitForServer(t, base+"/lean/v0/health")
	for _, c := range []struct {
		method, path string
		wantStatus   int
	}{
		{http.MethodPost, "/lean/v0/test_driver/state_transition/run", http.StatusBadRequest},
		{http.MethodGet, "/lean/v0/checkpoints/justified", http.StatusNotFound},
	} {
		request, err := http.NewRequest(c.method, base+c.path, strings.NewReader("not json"))
		if err != nil {
			t.Fatal(err)
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != c.wantStatus {
			t.Errorf("%s %s: status %d, want %d", c.method, c.path, response.StatusCode, c.wantStatus)
		}
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after its context ended")
	}
}

func waitForServer(t *testing.T, url string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		response, err := http.Get(url)
		if err == nil {
			response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s never answered: %v", url, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
