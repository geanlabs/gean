package unit

import (
	"testing"

	"github.com/devylongs/gean/network/reqresp"
)

func TestReqRespProtocolIDsMatchLeanSpec(t *testing.T) {
	if reqresp.StatusProtocol != "/leanconsensus/req/status/1/" {
		t.Fatalf("status protocol mismatch: got %q", reqresp.StatusProtocol)
	}
	if reqresp.BlocksByRootProtocol != "/leanconsensus/req/blocks_by_root/1/" {
		t.Fatalf("blocks_by_root protocol mismatch: got %q", reqresp.BlocksByRootProtocol)
	}
}
