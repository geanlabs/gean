package aggregation

import "github.com/geanlabs/gean/internal/store"

func applyAggregationMutations(s *store.ConsensusStore, payloads []store.PayloadKV, deletes []store.AttestationDeleteKey) {
	s.KnownPayloads.PushBatch(payloads)
	s.AttestationSignatures.Delete(deletes)
}
