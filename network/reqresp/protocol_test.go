package reqresp_test

import (
	"testing"

	"github.com/geanlabs/gean/network/reqresp"
)

func TestReqRespProtocolIDsMatchCrossClient(t *testing.T) {
	if reqresp.StatusProtocol != "/leanconsensus/req/status/1/ssz_snappy" {
		t.Fatalf("status protocol mismatch: got %q", reqresp.StatusProtocol)
	}
	if reqresp.BlocksByRootProtocol != "/leanconsensus/req/blocks_by_root/1/ssz_snappy" {
		t.Fatalf("blocks_by_root protocol mismatch: got %q", reqresp.BlocksByRootProtocol)
	}
}
