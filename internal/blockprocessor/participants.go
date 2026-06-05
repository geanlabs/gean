package blockprocessor

import (
	"fmt"

	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
	"github.com/geanlabs/gean/xmss"
)

func validatorAt(state *types.State, index uint64) (*types.Validator, error) {
	if state == nil {
		return nil, &store.StoreError{Kind: store.ErrMissingParentState, Message: "state missing"}
	}
	if index >= uint64(len(state.Validators)) {
		return nil, &store.StoreError{
			Kind:    store.ErrInvalidValidatorIndex,
			Message: fmt.Sprintf("validator %d out of range", index),
		}
	}
	validator := state.Validators[index]
	if validator == nil {
		return nil, &store.StoreError{
			Kind:    store.ErrInvalidValidatorIndex,
			Message: fmt.Sprintf("validator %d is nil", index),
		}
	}
	return validator, nil
}

func participantPubkeys(
	s *store.ConsensusStore,
	targetState *types.State,
	proof *types.AggregatedSignatureProof,
) ([]xmss.CPubKey, error) {
	if s == nil {
		return nil, fmt.Errorf("consensus store is nil")
	}
	if targetState == nil {
		return nil, &store.StoreError{Kind: store.ErrMissingTargetState, Message: "target state missing"}
	}
	if proof == nil {
		return nil, &store.StoreError{Kind: store.ErrAttestationSignatureMismatch, Message: "signature proof missing"}
	}
	if s.PubKeyCache == nil {
		return nil, &store.StoreError{Kind: store.ErrPubkeyDecodingFailed, Message: "pubkey cache missing"}
	}

	participantIDs := types.BitlistIndices(proof.Participants)
	pubkeys := make([]xmss.CPubKey, 0, len(participantIDs))
	for _, vid := range participantIDs {
		validator, err := validatorAt(targetState, vid)
		if err != nil {
			return nil, err
		}
		handle, err := s.PubKeyCache.Get(validator.AttestationPubkey)
		if err != nil {
			return nil, &store.StoreError{Kind: store.ErrPubkeyDecodingFailed, Message: fmt.Sprintf("validator %d: %v", vid, err)}
		}
		pubkeys = append(pubkeys, handle)
	}
	return pubkeys, nil
}
