package node

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/geanlabs/gean/internal/logger"
)

// A dispatch event that overruns an interval must name itself in the log: the
// loop also keeps the slot clock, so otherwise the only symptom is a node that
// drifts behind the head with nothing saying why.
func TestTimeEventLogsOnlySlowEvents(t *testing.T) {
	var buf bytes.Buffer
	logger.SetOutput(&buf)
	logger.SetQuiet(false)
	saved := slowDispatchEvent
	slowDispatchEvent = 20 * time.Millisecond
	t.Cleanup(func() {
		logger.SetOutput(os.Stderr)
		slowDispatchEvent = saved
	})

	timeEvent("fast", func() {})
	if buf.Len() != 0 {
		t.Fatalf("fast event logged: %q", buf.String())
	}

	timeEvent("block", func() { time.Sleep(30 * time.Millisecond) })
	if got := buf.String(); !strings.Contains(got, "slow dispatch event=block") {
		t.Fatalf("slow event not reported, log: %q", got)
	}
}
