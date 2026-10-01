//! finality.go

use std::collections::BTreeMap;

use crate::justifiable::slot_is_justifiable_after;
use crate::types::{bitlist, get, Checkpoint, Root, State};
use crate::Error;

pub fn try_finalize(
    state: &mut State,
    source: &Checkpoint,
    target: &Checkpoint,
    justifications: &mut BTreeMap<Root, Vec<bool>>,
    root_to_slot: &BTreeMap<Root, u64>,
) -> Result<(), Error> {
    let finalized_slot = state.latest_finalized.slot;
    if source.slot <= finalized_slot {
        return Ok(());
    }

    if (source.slot + 1..target.slot).any(|slot| slot_is_justifiable_after(slot, finalized_slot)) {
        return Ok(());
    }

    state.latest_finalized = *source;
    shift_justified_slots(state, source.slot - finalized_slot)?;

    let finalized = state.latest_finalized.slot;
    justifications.retain(|root, _| {
        root_to_slot
            .get(root)
            .is_some_and(|&slot| slot >= finalized)
    });
    Ok(())
}

fn shift_justified_slots(state: &mut State, delta: u64) -> Result<(), Error> {
    if delta == 0 {
        return Ok(());
    }
    let old = &state.justified_slots;
    let new_len = (old.len() as u64).saturating_sub(delta);
    state.justified_slots = bitlist(new_len, |i| get(old, i as u64 + delta))?;
    Ok(())
}

/// build_root_to_slot maps each historical root past the finalized slot to the
/// highest slot holding it.
pub fn build_root_to_slot(state: &State) -> BTreeMap<Root, u64> {
    let first = state.latest_finalized.slot.wrapping_add(1);
    (first..state.historical_block_hashes.len() as u64)
        .map(|slot| (state.historical_block_hashes[slot as usize], slot))
        .collect()
}
