//! The containers of internal/types that the transition reads, with SSZ
//! encoding and hash tree roots derived from the same field order and limits.

use ssz_derive::{Decode, Encode};
use ssz_types::typenum::{U1073741824, U262144, U4096};
use ssz_types::{BitList, VariableList};
use tree_hash_derive::TreeHash;

use crate::Error;

pub type Root = [u8; 32];

pub const ZERO_ROOT: Root = [0; 32];
pub const HISTORICAL_ROOTS_LIMIT: u64 = 1 << 18;
pub const MAX_ATTESTATIONS_DATA: usize = 8;

pub type JustifiedSlots = BitList<U262144>;
pub type JustificationsValidators = BitList<U1073741824>;
pub type RootList = VariableList<Root, U262144>;

#[derive(Clone, Encode, Decode, TreeHash)]
pub struct ChainConfig {
    pub genesis_time: u64,
}

#[derive(Clone, Copy, Encode, Decode, TreeHash)]
pub struct Checkpoint {
    pub root: Root,
    pub slot: u64,
}

#[derive(Clone, Encode, Decode, TreeHash)]
pub struct Validator {
    pub attestation_pubkey: [u8; 32],
    pub proposal_pubkey: [u8; 32],
    pub index: u64,
}

#[derive(Clone, Encode, Decode, TreeHash)]
pub struct BlockHeader {
    pub slot: u64,
    pub proposer_index: u64,
    pub parent_root: Root,
    pub state_root: Root,
    pub body_root: Root,
}

#[derive(Clone, Encode, Decode, TreeHash)]
pub struct AttestationData {
    pub slot: u64,
    pub head: Checkpoint,
    pub target: Checkpoint,
    pub source: Checkpoint,
}

#[derive(Clone, Encode, Decode, TreeHash)]
pub struct AggregatedAttestation {
    pub aggregation_bits: BitList<U4096>,
    pub data: AttestationData,
}

#[derive(Clone, Encode, Decode, TreeHash)]
pub struct BlockBody {
    pub attestations: VariableList<AggregatedAttestation, U4096>,
}

#[derive(Clone, Encode, Decode, TreeHash)]
pub struct Block {
    pub slot: u64,
    pub proposer_index: u64,
    pub parent_root: Root,
    pub state_root: Root,
    pub body: BlockBody,
}

#[derive(Clone, Encode, Decode, TreeHash)]
pub struct State {
    pub config: ChainConfig,
    pub slot: u64,
    pub latest_block_header: BlockHeader,
    pub latest_justified: Checkpoint,
    pub latest_finalized: Checkpoint,
    pub historical_block_hashes: RootList,
    pub justified_slots: JustifiedSlots,
    pub validators: VariableList<Validator, U4096>,
    pub justifications_roots: RootList,
    pub justifications_validators: JustificationsValidators,
}

/// bitlist builds a bitlist of `len` bits with the bits of `set` that fall
/// below it; a length over the type's limit is an SSZ limit error.
pub fn bitlist<N: ssz_types::typenum::Unsigned + Clone>(
    len: u64,
    set: impl Fn(usize) -> bool,
) -> Result<BitList<N>, Error> {
    let len = usize::try_from(len).map_err(|_| Error::SszLimit)?;
    let mut out = BitList::with_capacity(len).map_err(|_| Error::SszLimit)?;
    for i in (0..len).filter(|&i| set(i)) {
        out.set(i, true).map_err(|_| Error::SszLimit)?;
    }
    Ok(out)
}

/// get reads bit i, treating bits past the end as unset (types.BitlistGet).
pub fn get<N: ssz_types::typenum::Unsigned + Clone>(bits: &BitList<N>, i: u64) -> bool {
    usize::try_from(i).is_ok_and(|i| bits.get(i).unwrap_or(false))
}
