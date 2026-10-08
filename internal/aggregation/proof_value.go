package aggregation

import (
	"bytes"
	"slices"

	"github.com/geanlabs/gean/internal/metrics"
	"github.com/geanlabs/gean/internal/statetransition"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

// voteState is the head state as the next block's header leaves it, which is
// the state that block counts its votes against: the head block joins the chain
// history and the justified-slot window reaches the head's slot. A vote naming
// the head block fails the chain check against the head state itself. Only
// those two fields differ, and the head state is not modified.
func voteState(head *types.State, headRoot [32]byte) *types.State {
	if head == nil || head.LatestBlockHeader == nil || head.LatestFinalized == nil {
		return head
	}
	headSlot := head.LatestBlockHeader.Slot
	if uint64(len(head.HistoricalBlockHashes)) != headSlot {
		return head
	}
	projected := *head
	projected.HistoricalBlockHashes = append(slices.Clone(head.HistoricalBlockHashes), slices.Clone(headRoot[:]))
	if headSlot > head.LatestFinalized.Slot {
		projected.JustifiedSlots = types.BitlistExtend(bytes.Clone(head.JustifiedSlots), headSlot-head.LatestFinalized.Slot)
	}
	return &projected
}

// proofValue says what a proof covering voters would do for justification if a
// block carried it on top of state (see voteState). held are voters already in aggregates this
// node holds for the same data, so a proof whose only new voters are held ones
// adds nothing. It uses the state transition's own vote filters, so it cannot
// disagree with how a block is processed.
func proofValue(state *types.State, data *types.AttestationData, voters []uint64, held map[uint64]bool) string {
	if state == nil || data == nil {
		return metrics.ProofValueIgnored
	}
	reason, err := statetransition.VoteInvalidReason(state, data.Source, data.Target)
	if err != nil || reason != "" || !statetransition.HeadMatchesChain(state, data.Head) {
		return metrics.ProofValueIgnored
	}
	validatorCount := len(state.Validators)
	tally := statetransition.JustificationVotes(state, data.Target.Root)
	counted := 0
	for _, voted := range tally {
		if voted {
			counted++
		}
	}
	added := 0
	for _, v := range voters {
		if v >= uint64(validatorCount) || (tally != nil && tally[v]) {
			continue
		}
		counted++
		if !held[v] {
			added++
		}
	}
	switch {
	case added == 0:
		return metrics.ProofValueNoNewVotes
	case 3*counted >= 2*validatorCount:
		return metrics.ProofValueJustifies
	default:
		return metrics.ProofValueAddsVotes
	}
}

// heldVoters are the participants of every aggregate this node holds for one
// data root, new or known.
func heldVoters(snap *Snapshot, dataRoot [32]byte) map[uint64]bool {
	held := make(map[uint64]bool)
	for _, entry := range []*store.PayloadEntry{snap.newEntries[dataRoot], snap.knownEntries[dataRoot]} {
		if entry == nil {
			continue
		}
		for _, proof := range entry.Proofs {
			for _, v := range types.BitlistIndices(proof.Participants) {
				held[v] = true
			}
		}
	}
	return held
}

// deferredValues classifies the groups a session stopped before proving, as if
// each had been proved with every signer on hand.
func deferredValues(snap *Snapshot, state *types.State, groups []aggregationGroup) map[string]int {
	values := make(map[string]int)
	for _, group := range groups {
		held := heldVoters(snap, group.dataRoot)
		seen := make(map[uint64]bool, len(held))
		voters := make([]uint64, 0, len(held))
		for v := range held {
			seen[v] = true
			voters = append(voters, v)
		}
		if entry := snap.attSigs[group.dataRoot]; entry != nil {
			for _, sig := range entry.Signatures {
				if !seen[sig.ValidatorID] {
					seen[sig.ValidatorID] = true
					voters = append(voters, sig.ValidatorID)
				}
			}
		}
		values[proofValue(state, attestationDataForRoot(snap, group.dataRoot), voters, held)]++
	}
	return values
}
