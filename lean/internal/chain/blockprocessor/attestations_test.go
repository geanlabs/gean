package blockprocessor

import (
	"testing"

	"github.com/geanlabs/gean/lean/consensus/types"
	"github.com/geanlabs/gean/lean/internal/pools"
)

func TestImportBlockAttestationsAddsKnownPayload(t *testing.T) {
	known := pools.New().KnownPayloads
	att := processorAttestation()
	types.BitlistSet(att.AggregationBits, 0)

	importBlockAttestations(known, &types.SignedBlock{
		Block: processorBlock(att),
		Proof: &types.MultiMessageAggregate{},
	})

	if known.Len() != 1 {
		t.Fatalf("known payloads=%d, want 1", known.Len())
	}
	if known.TotalProofs() != 0 {
		t.Fatal("block attestation must not gain head weight at import")
	}
}

func TestImportBlockAttestationsSkipsMalformedEntries(t *testing.T) {
	known := pools.New().KnownPayloads

	importBlockAttestations(known, &types.SignedBlock{
		Block: processorBlock(&types.AggregatedAttestation{}),
		Proof: &types.MultiMessageAggregate{},
	})

	if known.Len() != 0 {
		t.Fatalf("known payloads=%d, want 0", known.Len())
	}
}
