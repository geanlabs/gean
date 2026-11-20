package p2p

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/geanlabs/gean/internal/core/p2p/host"
	"github.com/geanlabs/gean/internal/core/p2p/peers"
	"github.com/geanlabs/gean/internal/core/p2p/reqresp"
)

// Core p2p only reports through its hooks, so a hook left unset loses a metric
// without any other test noticing.
func TestCoreP2PHooksReachMetrics(t *testing.T) {
	host.OnRequestOutcome("hook-probe")
	peers.OnScore("hook-probe", 7)
	reqresp.OnTimeout("hook-probe", "read")
	reqresp.OnResponseChunk("hook-probe", 10)

	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]bool)
	for _, family := range families {
		for _, m := range family.GetMetric() {
			for _, label := range m.GetLabel() {
				if label.GetValue() == "hook-probe" {
					seen[family.GetName()] = true
				}
			}
		}
	}
	for _, name := range []string{
		"lean_peer_request_outcomes_total",
		"lean_peer_score",
		"lean_p2p_reqresp_timeout_total",
		"lean_reqresp_response_chunk_size_bytes",
	} {
		if !seen[name] {
			t.Errorf("%s was not updated through its core hook", name)
		}
	}
	peers.OnRemove("hook-probe")
}
