//! attestations.go

use std::collections::BTreeSet;

use tree_hash::TreeHash;

use crate::finality::{build_root_to_slot, try_finalize};
use crate::justifications::{reconstruct_justifications, serialize_justifications};
use crate::types::{AggregatedAttestation, State, MAX_ATTESTATIONS_DATA, ZERO_ROOT};
use crate::votes::{head_matches_chain, set_slot_justified, vote_is_valid};
use crate::Error;

pub fn process_attestations(
    state: &mut State,
    attestations: &[AggregatedAttestation],
) -> Result<(), Error> {
    // The transition bounds the distinct votes a block may carry: each one
    // builds a tally sized to the validator set.
    let distinct: BTreeSet<_> = attestations
        .iter()
        .map(|agg| agg.data.tree_hash_root())
        .collect();
    if distinct.len() > MAX_ATTESTATIONS_DATA {
        return Err(Error::TooManyAttestationData {
            count: distinct.len() as u64,
            max: MAX_ATTESTATIONS_DATA as u64,
        });
    }

    let validator_count = state.validators.len();
    if validator_count == 0 {
        return Err(Error::EmptyValidatorRegistry);
    }

    // The flat vote bitlist holds one segment of validator_count bits per
    // tracked root.
    if state.justifications_validators.len() != state.justifications_roots.len() * validator_count {
        return Err(Error::JustificationVotesLengthMismatch);
    }

    if state.justifications_roots.contains(&ZERO_ROOT) {
        return Err(Error::ZeroHashInJustificationRoots);
    }

    let mut justifications = reconstruct_justifications(state, validator_count);
    let root_to_slot = build_root_to_slot(state);

    for agg in attestations {
        let (source, target) = (agg.data.source, agg.data.target);

        // An out-of-range justified-slot query rejects the whole block; any
        // other invalid vote is only filtered out.
        if !vote_is_valid(state, &source, &target)? || !head_matches_chain(state, &agg.data.head) {
            continue;
        }

        // A counted vote must name at least one voter, all in the registry.
        let voters: Vec<usize> = agg
            .aggregation_bits
            .iter()
            .enumerate()
            .filter_map(|(i, set)| set.then_some(i))
            .collect();
        if voters.is_empty() {
            return Err(Error::EmptyAggregationBits);
        }
        if let Some(&voter) = voters.iter().find(|&&voter| voter >= validator_count) {
            return Err(Error::AttesterIndexOutOfRange {
                index: voter as u64,
                validators: validator_count as u64,
            });
        }

        let votes = justifications
            .entry(target.root)
            .or_insert_with(|| vec![false; validator_count]);
        for &voter in &voters {
            votes[voter] = true;
        }

        let vote_count = votes.iter().filter(|&&v| v).count();
        if 3 * vote_count >= 2 * validator_count {
            if target.slot >= state.latest_justified.slot {
                state.latest_justified = target;
            }
            set_slot_justified(state, state.latest_finalized.slot, target.slot)?;

            justifications.remove(&target.root);
            try_finalize(state, &source, &target, &mut justifications, &root_to_slot)?;
        }
    }

    serialize_justifications(state, &justifications, validator_count)
}
