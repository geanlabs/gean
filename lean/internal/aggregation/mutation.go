package aggregation

import (
	"github.com/geanlabs/gean/lean/internal/pools"
)

// Locally-built aggregates land in the new pool; the accept-attestations tick
// promotes them to known, matching leanSpec aggregate (latest_new_aggregated_payloads).
func applyAggregationMutations(p *pools.Attestations, payloads []pools.PayloadKV, deletes []pools.AttestationDeleteKey) {
	p.NewPayloads.PushBatch(payloads)
	p.AttestationSignatures.Delete(deletes)
}
