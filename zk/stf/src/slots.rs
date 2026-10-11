//! slots.go

use tree_hash::TreeHash;

use crate::types::{State, ZERO_ROOT};
use crate::Error;

pub fn process_slots(state: &mut State, target_slot: u64) -> Result<(), Error> {
    if state.slot >= target_slot {
        return Err(Error::StateSlotIsNewer {
            target_slot,
            current_slot: state.slot,
        });
    }

    if state.latest_block_header.state_root == ZERO_ROOT {
        state.latest_block_header.state_root = state.tree_hash_root().0;
    }

    state.slot = target_slot;
    Ok(())
}
