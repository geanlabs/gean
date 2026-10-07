package aggregation

import "github.com/geanlabs/gean/store"

// Result is what an aggregation session produced for the store: the
// aggregates to add and the gossip signatures they now represent.
type Result struct {
	Payloads []store.PayloadKV
	Deletes  []store.AttestationDeleteKey
}

// Apply adds the aggregates to the new pool, which the accept-attestations
// tick promotes to known (leanSpec latest_new_aggregated_payloads), and
// retires the signatures they cover. The store's owner calls it.
func (r Result) Apply(s *store.ConsensusStore) {
	s.NewPayloads().PushBatch(r.Payloads)
	s.AttestationSignatures().Delete(r.Deletes)
}
