package aggregation

import (
	"fmt"

	"github.com/geanlabs/gean/consensus/attestationproof"
	"github.com/geanlabs/gean/crypto"
	"github.com/geanlabs/gean/types"
)

// ProofMerger merges attestation proofs for the same data into one recursive
// proof. It implements attestationproof.MergeProvider.
type ProofMerger struct {
	scheme crypto.Scheme
}

func NewProofMerger(scheme crypto.Scheme) *ProofMerger {
	return &ProofMerger{scheme: scheme}
}

func (m *ProofMerger) Merge(
	proofs []*types.SingleMessageAggregate,
	attData *types.AttestationData,
	state *types.State,
) (*types.SingleMessageAggregate, error) {
	if len(proofs) < 2 {
		return nil, fmt.Errorf("%w: fewer than two proofs", attestationproof.ErrMergeUnavailable)
	}
	if attData == nil {
		return nil, fmt.Errorf("%w: attestation data is nil", attestationproof.ErrMergeUnavailable)
	}
	slot := uint32(attData.Slot)
	if uint64(slot) != attData.Slot {
		return nil, fmt.Errorf("%w: slot %d overflows uint32", attestationproof.ErrMergeUnavailable, attData.Slot)
	}
	if state == nil {
		return nil, fmt.Errorf("%w: state is nil", attestationproof.ErrMergeUnavailable)
	}
	if m == nil || m.scheme == nil {
		return nil, fmt.Errorf("%w: scheme is nil", attestationproof.ErrMergeUnavailable)
	}

	children := make([]crypto.Proof, 0, len(proofs))
	allIDs := make([]uint64, 0)
	seen := make(map[uint64]bool)
	for _, proof := range proofs {
		if proof == nil || len(proof.Proof) == 0 || types.BitlistCount(proof.Participants) == 0 {
			return nil, fmt.Errorf("%w: malformed child proof", attestationproof.ErrMergeUnavailable)
		}

		pubkeys := make([]crypto.PublicKey, 0, types.BitlistLen(proof.Participants))
		for vid := range types.BitlistLen(proof.Participants) {
			if !types.BitlistGet(proof.Participants, vid) {
				continue
			}
			if seen[vid] {
				return nil, fmt.Errorf("%w: participant %d appears in multiple proofs", attestationproof.ErrMergeUnavailable, vid)
			}
			seen[vid] = true
			if vid >= uint64(len(state.Validators)) {
				return nil, fmt.Errorf("%w: participant %d exceeds validator count %d",
					attestationproof.ErrMergeUnavailable, vid, len(state.Validators))
			}
			validator := state.Validators[vid]
			if validator == nil {
				return nil, fmt.Errorf("%w: validator %d is nil", attestationproof.ErrMergeUnavailable, vid)
			}

			pubkeys = append(pubkeys, validator.AttestationPubkey)
			allIDs = append(allIDs, vid)
		}

		if len(pubkeys) == 0 {
			return nil, fmt.Errorf("%w: child proof has no known participants", attestationproof.ErrMergeUnavailable)
		}
		children = append(children, crypto.Proof{
			PublicKeys: pubkeys,
			Proof:      append([]byte(nil), proof.Proof...),
		})
	}

	if len(children) < 2 {
		return nil, fmt.Errorf("%w: fewer than two usable child proofs", attestationproof.ErrMergeUnavailable)
	}

	dataRoot, err := attData.HashTreeRoot()
	if err != nil {
		return nil, fmt.Errorf("attestation data root: %w", err)
	}
	mergedBytes, err := m.scheme.Aggregate(nil, children, dataRoot, slot)
	if err != nil {
		return nil, fmt.Errorf("aggregate child proofs: %w", err)
	}

	return &types.SingleMessageAggregate{
		Participants: types.BitlistFromIndices(allIDs),
		Proof:        mergedBytes,
	}, nil
}
