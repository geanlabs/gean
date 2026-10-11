//! justifications.go

use std::collections::BTreeMap;

use crate::types::{bitlist, get, Root, RootList, State};
use crate::Error;

/// reconstruct_justifications splits the flat vote bitlist into one tally per
/// tracked root; a repeated root keeps its last tally.
pub fn reconstruct_justifications(
    state: &State,
    validator_count: usize,
) -> BTreeMap<Root, Vec<bool>> {
    let bits = &state.justifications_validators;
    state
        .justifications_roots
        .iter()
        .enumerate()
        .map(|(i, root)| {
            let votes = (0..validator_count)
                .map(|v| get(bits, (i * validator_count + v) as u64))
                .collect();
            (*root, votes)
        })
        .collect()
}

/// serialize_justifications writes the tallies back in root order.
pub fn serialize_justifications(
    state: &mut State,
    justifications: &BTreeMap<Root, Vec<bool>>,
    validator_count: usize,
) -> Result<(), Error> {
    let roots: Vec<Root> = justifications.keys().copied().collect();
    let votes: Vec<&Vec<bool>> = justifications.values().collect();
    state.justifications_roots = RootList::new(roots).map_err(|_| Error::SszLimit)?;
    let total_bits = (votes.len() * validator_count) as u64;
    state.justifications_validators = bitlist(total_bits, |i| {
        votes[i / validator_count][i % validator_count]
    })?;
    Ok(())
}
