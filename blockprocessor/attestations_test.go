package blockprocessor

import (
	"testing"

	"github.com/geanlabs/gean/db"
	"github.com/geanlabs/gean/store"
	"github.com/geanlabs/gean/types"
)

func TestImportBlockAttestationsAddsKnownPayload(t *testing.T) {
	s := store.NewConsensusStore(db.NewInMemoryBackend())
	att := processorAttestation()
	types.BitlistSet(att.AggregationBits, 0)

	importBlockAttestations(s, &types.SignedBlock{
		Block: processorBlock(att),
		Proof: &types.MultiMessageAggregate{},
	})

	if s.KnownPayloads().Len() != 1 {
		t.Fatalf("known payloads=%d, want 1", s.KnownPayloads().Len())
	}
	if s.KnownPayloads().TotalProofs() != 0 {
		t.Fatal("block attestation must not gain head weight at import")
	}
}

func TestImportBlockAttestationsSkipsMalformedEntries(t *testing.T) {
	s := store.NewConsensusStore(db.NewInMemoryBackend())

	importBlockAttestations(s, &types.SignedBlock{
		Block: processorBlock(&types.AggregatedAttestation{}),
		Proof: &types.MultiMessageAggregate{},
	})

	if s.KnownPayloads().Len() != 0 {
		t.Fatalf("known payloads=%d, want 0", s.KnownPayloads().Len())
	}
}
