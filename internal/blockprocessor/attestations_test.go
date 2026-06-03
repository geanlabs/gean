package blockprocessor

import (
	"testing"

	"github.com/geanlabs/gean/internal/storage"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

func TestImportBlockAttestationsAddsKnownPayload(t *testing.T) {
	s := store.NewConsensusStore(storage.NewInMemoryBackend())
	att := processorAttestation()
	types.BitlistSet(att.AggregationBits, 0)
	proof := &types.AggregatedSignatureProof{Participants: att.AggregationBits}

	importBlockAttestations(s, &types.SignedBlock{
		Block: processorBlock(att),
		Signature: &types.BlockSignatures{
			AttestationSignatures: []*types.AggregatedSignatureProof{proof},
		},
	})

	if s.KnownPayloads.Len() != 1 {
		t.Fatalf("known payloads=%d, want 1", s.KnownPayloads.Len())
	}
}

func TestImportBlockAttestationsSkipsMalformedEntries(t *testing.T) {
	s := store.NewConsensusStore(storage.NewInMemoryBackend())

	importBlockAttestations(s, &types.SignedBlock{
		Block: processorBlock(&types.AggregatedAttestation{}),
		Signature: &types.BlockSignatures{
			AttestationSignatures: []*types.AggregatedSignatureProof{{}},
		},
	})

	if s.KnownPayloads.Len() != 0 {
		t.Fatalf("known payloads=%d, want 0", s.KnownPayloads.Len())
	}
}

func TestImportBlockAttestationsSkipsParticipantMismatch(t *testing.T) {
	s := store.NewConsensusStore(storage.NewInMemoryBackend())
	att := processorAttestation()
	types.BitlistSet(att.AggregationBits, 0)
	participants := types.NewBitlistSSZ(2)
	types.BitlistSet(participants, 1)

	importBlockAttestations(s, &types.SignedBlock{
		Block: processorBlock(att),
		Signature: &types.BlockSignatures{
			AttestationSignatures: []*types.AggregatedSignatureProof{{Participants: participants}},
		},
	})

	if s.KnownPayloads.Len() != 0 {
		t.Fatalf("known payloads=%d, want 0", s.KnownPayloads.Len())
	}
}
