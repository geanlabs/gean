package attestationproof

import (
	"errors"

	"github.com/geanlabs/gean/internal/types"
)

var ErrMergeUnavailable = errors.New("proof merge unavailable")
var ErrNoUsableProofs = errors.New("no usable proofs")

// MergeProvider merges proofs for the same attestation data into one proof.
// Selection depends only on this contract, so it runs without native crypto;
// aggregation.ProofMerger is the XMSS implementation.
type MergeProvider interface {
	Merge(
		proofs []*types.SingleMessageAggregate,
		attData *types.AttestationData,
		state *types.State,
	) (*types.SingleMessageAggregate, error)
}
