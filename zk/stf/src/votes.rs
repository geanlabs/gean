//! votes.go

use crate::justifiable::slot_is_justifiable_after;
use crate::types::{bitlist, get, Checkpoint, State, ZERO_ROOT};
use crate::Error;

/// vote_is_valid is VoteInvalidReason reporting only whether a reason exists.
/// The justification queries come first: an out-of-range source or target slot
/// rejects the block before the chain checks run.
pub fn vote_is_valid(
    state: &State,
    source: &Checkpoint,
    target: &Checkpoint,
) -> Result<bool, Error> {
    let finalized_slot = state.latest_finalized.slot;

    // source_not_justified
    if !is_slot_justified(state, finalized_slot, source.slot)? {
        return Ok(false);
    }
    // target_already_justified
    if is_slot_justified(state, finalized_slot, target.slot)? {
        return Ok(false);
    }
    // zero_root, chain_mismatch, target_not_after_source, target_not_justifiable
    Ok(source.root != ZERO_ROOT
        && target.root != ZERO_ROOT
        && checkpoint_exists(state, source)
        && checkpoint_exists(state, target)
        && target.slot > source.slot
        && slot_is_justifiable_after(target.slot, finalized_slot))
}

/// head_matches_chain reports whether the attestation head sits on the
/// canonical chain at its slot.
pub fn head_matches_chain(state: &State, head: &Checkpoint) -> bool {
    head.root != ZERO_ROOT && checkpoint_exists(state, head)
}

/// is_slot_justified reports whether a slot is justified. Slots at or below the
/// finalized boundary are justified by definition; a slot past the tracked
/// bitfield rejects the block.
pub fn is_slot_justified(state: &State, finalized_slot: u64, slot: u64) -> Result<bool, Error> {
    if slot <= finalized_slot {
        return Ok(true);
    }
    let rel_index = slot - finalized_slot - 1;
    let tracked_len = state.justified_slots.len() as u64;
    if rel_index >= tracked_len {
        return Err(Error::JustifiedSlotOutOfRange {
            slot,
            finalized_boundary: finalized_slot,
            tracked_length: tracked_len,
        });
    }
    Ok(get(&state.justified_slots, rel_index))
}

pub fn set_slot_justified(state: &mut State, finalized_slot: u64, slot: u64) -> Result<(), Error> {
    if slot <= finalized_slot {
        return Ok(());
    }
    let rel_index = slot - finalized_slot - 1;
    let old = &state.justified_slots;
    let len = (old.len() as u64).max(rel_index + 1);
    state.justified_slots = bitlist(len, |i| i as u64 == rel_index || get(old, i as u64))?;
    Ok(())
}

fn checkpoint_exists(state: &State, cp: &Checkpoint) -> bool {
    usize::try_from(cp.slot)
        .ok()
        .and_then(|slot| state.historical_block_hashes.get(slot))
        == Some(&cp.root)
}
