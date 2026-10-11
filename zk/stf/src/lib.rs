//! gean's state transition, ported function for function from the Go
//! implementation in internal/statetransition so zkVM guests can prove it.
//! Each module names the Go file it mirrors; internal/zkstf generates the
//! vectors that hold the two implementations equal (tests/vectors.rs).

mod attestations;
mod block;
mod errors;
mod finality;
mod justifiable;
mod justifications;
mod slots;
mod transition;
pub mod types;
mod votes;

use ssz::Decode;
use tree_hash::TreeHash;

pub use errors::Error;
pub use transition::state_transition;
use types::{Block, State};

/// The size of the committed public values: pre-state root, block root and
/// post-state root.
pub const PUBLIC_VALUES_SIZE: usize = 96;

/// Input framing of internal/zkstf/input.go:
/// "GSTF" | u32 version | u64 len | state SSZ | u64 len | block SSZ.
const INPUT_MAGIC: &[u8; 4] = b"GSTF";
const INPUT_VERSION: u32 = 1;
const MAX_STATE_SSZ: u64 = 64 << 20;
const MAX_BLOCK_SSZ: u64 = 8 << 20;

/// apply decodes a framed input, runs the state transition and returns the
/// public values, as internal/zkstf.Apply does. A guest must not commit
/// anything when it fails, so no proof exists for a rejected input.
pub fn apply(input: &[u8]) -> Result<[u8; PUBLIC_VALUES_SIZE], Error> {
    let (state_ssz, block_ssz) = decode_input(input)?;
    let mut state = State::from_ssz_bytes(state_ssz).map_err(|_| Error::MalformedInput)?;
    let block = Block::from_ssz_bytes(block_ssz).map_err(|_| Error::MalformedInput)?;

    let pre_state_root = state.tree_hash_root();
    let block_root = block.tree_hash_root();
    state_transition(&mut state, &block)?;

    // state_transition has checked HTR(post-state) == block.state_root.
    let mut pv = [0; PUBLIC_VALUES_SIZE];
    pv[..32].copy_from_slice(pre_state_root.as_slice());
    pv[32..64].copy_from_slice(block_root.as_slice());
    pv[64..].copy_from_slice(&block.state_root);
    Ok(pv)
}

fn decode_input(input: &[u8]) -> Result<(&[u8], &[u8]), Error> {
    let rest = input
        .strip_prefix(INPUT_MAGIC)
        .ok_or(Error::MalformedInput)?;
    let (version, rest) = rest.split_first_chunk::<4>().ok_or(Error::MalformedInput)?;
    if u32::from_le_bytes(*version) != INPUT_VERSION {
        return Err(Error::MalformedInput);
    }
    let (state, rest) = read_section(rest, MAX_STATE_SSZ)?;
    let (block, rest) = read_section(rest, MAX_BLOCK_SSZ)?;
    if !rest.is_empty() {
        return Err(Error::MalformedInput);
    }
    Ok((state, block))
}

fn read_section(input: &[u8], limit: u64) -> Result<(&[u8], &[u8]), Error> {
    let (len, rest) = input
        .split_first_chunk::<8>()
        .ok_or(Error::MalformedInput)?;
    let len = u64::from_le_bytes(*len);
    if len > limit || len > rest.len() as u64 {
        return Err(Error::MalformedInput);
    }
    Ok(rest.split_at(len as usize))
}
