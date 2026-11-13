// Package validator holds the Lean validator's duties: which attestation data
// to sign, and the reservation that lets each proposal duty be signed at most
// once. The chain loop decides when duties run and calls into this package.
package validator

import "github.com/geanlabs/gean/lean/consensus/types"

// ProposalDuty is one proposal this node may make: the slot and the validator
// that proposes it.
type ProposalDuty struct {
	Slot        uint64
	ValidatorID uint64
	// Early marks a duty taken before its slot started, which makes the
	// interval-0 trigger skip the slot.
	Early bool
}

// ProposalReservation lets each proposal duty reach signing at most once. A
// duty is reserved when it is handed to the prover and stays reserved through
// signing, because a signing attempt spends the slot's one-time key; only a
// failure before signing may release it. The chain loop owns it, so it is not
// safe for concurrent use.
type ProposalReservation struct {
	last     ProposalDuty
	reserved bool
}

// Available reports whether a proposal for slot may start: slot is not older
// than the last reserved duty, and a duty for the same slot has been released.
func (r *ProposalReservation) Available(slot uint64) bool {
	return slot > r.last.Slot || (slot == r.last.Slot && !r.reserved)
}

// Reserve records duty as handed to the prover.
func (r *ProposalReservation) Reserve(duty ProposalDuty) {
	r.last = duty
	r.reserved = true
}

// Release frees duty after a failure before signing and reports whether it did.
// A duty other than the last reserved one changes nothing, so a stale worker
// result cannot free a newer reservation.
func (r *ProposalReservation) Release(duty ProposalDuty) bool {
	if duty != r.last {
		return false
	}
	r.reserved = false
	return true
}

// ProposerAmong returns the validator in ids that proposes at slot in a
// registry of numValidators, if any.
func ProposerAmong(ids []uint64, slot, numValidators uint64) (uint64, bool) {
	for _, id := range ids {
		if types.IsProposer(slot, id, numValidators) {
			return id, true
		}
	}
	return 0, false
}
