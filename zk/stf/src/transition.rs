//! transition.go

use tree_hash::TreeHash;

use crate::block::process_block;
use crate::slots::process_slots;
use crate::types::{Block, State};
use crate::Error;

pub fn state_transition(state: &mut State, block: &Block) -> Result<(), Error> {
    process_slots(state, block.slot)?;
    process_block(state, block)?;
    verify_state_root(state, block)
}

/// verify_state_root checks that the post-transition state root matches the
/// block's committed state root.
pub fn verify_state_root(state: &State, block: &Block) -> Result<(), Error> {
    let computed = state.tree_hash_root().0;
    if computed != block.state_root {
        return Err(Error::StateRootMismatch {
            expected: block.state_root,
            computed,
        });
    }
    Ok(())
}
