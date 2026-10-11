package zkstf

import (
	"errors"
	"fmt"

	ssz "github.com/ferranbt/fastssz"

	"github.com/geanlabs/gean/internal/statetransition"
)

// ErrorKind names the class of an Apply error. The Rust port (zk/stf) reports
// the same names, so the two implementations can be compared case by case.
// An error outside the known set is itself an error: a new failure mode must
// be named on both sides.
func ErrorKind(err error) (string, error) {
	var (
		stateSlotIsNewer   *statetransition.StateSlotIsNewerError
		slotMismatch       *statetransition.SlotMismatchError
		parentSlotIsNewer  *statetransition.ParentSlotIsNewerError
		invalidProposer    *statetransition.InvalidProposerError
		invalidParent      *statetransition.InvalidParentError
		stateRootMismatch  *statetransition.StateRootMismatchError
		slotGapTooLarge    *statetransition.SlotGapTooLargeError
		attesterOutOfRange *statetransition.AttesterIndexOutOfRangeError
		tooManyData        *statetransition.TooManyAttestationDataError
		justifiedOutOfRng  *statetransition.JustifiedSlotOutOfRangeError
	)
	switch {
	case errors.Is(err, ErrMalformedInput):
		return "malformed_input", nil
	case errors.As(err, &stateSlotIsNewer):
		return "state_slot_is_newer", nil
	case errors.As(err, &slotMismatch):
		return "slot_mismatch", nil
	case errors.As(err, &parentSlotIsNewer):
		return "parent_slot_is_newer", nil
	case errors.As(err, &invalidProposer):
		return "invalid_proposer", nil
	case errors.As(err, &invalidParent):
		return "invalid_parent", nil
	case errors.As(err, &stateRootMismatch):
		return "state_root_mismatch", nil
	case errors.As(err, &slotGapTooLarge):
		return "slot_gap_too_large", nil
	case errors.As(err, &attesterOutOfRange):
		return "attester_index_out_of_range", nil
	case errors.As(err, &tooManyData):
		return "too_many_attestation_data", nil
	case errors.As(err, &justifiedOutOfRng):
		return "justified_slot_out_of_range", nil
	case errors.Is(err, statetransition.ErrEmptyAggregationBits):
		return "empty_aggregation_bits", nil
	case errors.Is(err, statetransition.ErrNoValidators):
		return "no_validators", nil
	case errors.Is(err, statetransition.ErrZeroHashInJustificationRoots):
		return "zero_hash_in_justification_roots", nil
	case errors.Is(err, statetransition.ErrEmptyValidatorRegistry):
		return "empty_validator_registry", nil
	case errors.Is(err, statetransition.ErrJustificationVotesLengthMismatch):
		return "justification_votes_length_mismatch", nil
	case errors.Is(err, ssz.ErrListTooBig), errors.Is(err, ssz.ErrEmptyBitlist),
		errors.Is(err, ssz.ErrIncorrectListSize), errors.Is(err, ssz.ErrBytesLength):
		// Hashing a state whose lists outgrew their SSZ limits.
		return "ssz_limit", nil
	}
	return "", fmt.Errorf("no kind for error: %w", err)
}
