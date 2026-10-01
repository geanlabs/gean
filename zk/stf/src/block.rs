//! block.go

use tree_hash::TreeHash;

use crate::attestations::process_attestations;
use crate::types::{bitlist, get, Block, BlockHeader, State, HISTORICAL_ROOTS_LIMIT, ZERO_ROOT};
use crate::Error;

pub fn process_block(state: &mut State, block: &Block) -> Result<(), Error> {
    process_block_header(state, block)?;
    process_attestations(state, &block.body.attestations)
}

pub fn process_block_header(state: &mut State, block: &Block) -> Result<(), Error> {
    let num_validators = state.validators.len() as u64;
    if num_validators == 0 {
        return Err(Error::NoValidators);
    }

    if block.slot != state.slot {
        return Err(Error::SlotMismatch {
            state_slot: state.slot,
            block_slot: block.slot,
        });
    }

    let parent_header = &state.latest_block_header;
    if block.slot <= parent_header.slot {
        return Err(Error::ParentSlotIsNewer {
            parent_slot: parent_header.slot,
            block_slot: block.slot,
        });
    }

    let expected_proposer = block.slot % num_validators;
    if block.proposer_index != expected_proposer {
        return Err(Error::InvalidProposer {
            expected: expected_proposer,
            found: block.proposer_index,
        });
    }

    let parent_root = parent_header.tree_hash_root().0;
    if block.parent_root != parent_root {
        return Err(Error::InvalidParent {
            expected: parent_root,
            found: block.parent_root,
        });
    }

    let body_root = block.body.tree_hash_root().0;

    let parent_slot = parent_header.slot;
    let num_empty_slots = block.slot - parent_slot - 1;
    let new_entries = 1 + num_empty_slots;
    let current_historical_roots = state.historical_block_hashes.len() as u64;
    if current_historical_roots > HISTORICAL_ROOTS_LIMIT
        || new_entries > HISTORICAL_ROOTS_LIMIT - current_historical_roots
    {
        return Err(Error::SlotGapTooLarge {
            gap: new_entries,
            current: state.slot,
            max: HISTORICAL_ROOTS_LIMIT,
        });
    }

    if parent_slot == 0 {
        state.latest_justified.root = parent_root;
        state.latest_finalized.root = parent_root;
    }

    state
        .historical_block_hashes
        .push(parent_root)
        .map_err(|_| Error::SszLimit)?;
    for _ in 0..num_empty_slots {
        state
            .historical_block_hashes
            .push(ZERO_ROOT)
            .map_err(|_| Error::SszLimit)?;
    }

    let last_materialized_slot = block.slot - 1;
    if last_materialized_slot > state.latest_finalized.slot {
        let required_len = last_materialized_slot - state.latest_finalized.slot;
        if required_len > state.justified_slots.len() as u64 {
            let old = &state.justified_slots;
            state.justified_slots = bitlist(required_len, |i| get(old, i as u64))?;
        }
    }

    state.latest_block_header = BlockHeader {
        slot: block.slot,
        proposer_index: block.proposer_index,
        parent_root: block.parent_root,
        state_root: ZERO_ROOT,
        body_root,
    };
    Ok(())
}
