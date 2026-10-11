//! The errors of internal/statetransition/errors.go.
//!
//! Go's malformed-state and malformed-block errors report nil fields. A
//! decoded SSZ container has every field, so those errors cannot occur here
//! and have no counterpart.

use std::fmt;

use crate::types::Root;

#[derive(Debug, PartialEq, Eq)]
pub enum Error {
    /// The input framing or its SSZ does not decode.
    MalformedInput,
    StateSlotIsNewer {
        target_slot: u64,
        current_slot: u64,
    },
    SlotMismatch {
        state_slot: u64,
        block_slot: u64,
    },
    ParentSlotIsNewer {
        parent_slot: u64,
        block_slot: u64,
    },
    InvalidProposer {
        expected: u64,
        found: u64,
    },
    InvalidParent {
        expected: Root,
        found: Root,
    },
    StateRootMismatch {
        expected: Root,
        computed: Root,
    },
    SlotGapTooLarge {
        gap: u64,
        current: u64,
        max: u64,
    },
    AttesterIndexOutOfRange {
        index: u64,
        validators: u64,
    },
    TooManyAttestationData {
        count: u64,
        max: u64,
    },
    JustifiedSlotOutOfRange {
        slot: u64,
        finalized_boundary: u64,
        tracked_length: u64,
    },
    EmptyAggregationBits,
    NoValidators,
    ZeroHashInJustificationRoots,
    EmptyValidatorRegistry,
    JustificationVotesLengthMismatch,
    /// A list or bitlist would outgrow its SSZ limit.
    SszLimit,
}

impl Error {
    /// kind is the name internal/zkstf.ErrorKind gives the same failure.
    pub fn kind(&self) -> &'static str {
        match self {
            Error::MalformedInput => "malformed_input",
            Error::StateSlotIsNewer { .. } => "state_slot_is_newer",
            Error::SlotMismatch { .. } => "slot_mismatch",
            Error::ParentSlotIsNewer { .. } => "parent_slot_is_newer",
            Error::InvalidProposer { .. } => "invalid_proposer",
            Error::InvalidParent { .. } => "invalid_parent",
            Error::StateRootMismatch { .. } => "state_root_mismatch",
            Error::SlotGapTooLarge { .. } => "slot_gap_too_large",
            Error::AttesterIndexOutOfRange { .. } => "attester_index_out_of_range",
            Error::TooManyAttestationData { .. } => "too_many_attestation_data",
            Error::JustifiedSlotOutOfRange { .. } => "justified_slot_out_of_range",
            Error::EmptyAggregationBits => "empty_aggregation_bits",
            Error::NoValidators => "no_validators",
            Error::ZeroHashInJustificationRoots => "zero_hash_in_justification_roots",
            Error::EmptyValidatorRegistry => "empty_validator_registry",
            Error::JustificationVotesLengthMismatch => "justification_votes_length_mismatch",
            Error::SszLimit => "ssz_limit",
        }
    }
}

impl fmt::Display for Error {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "{}: {self:?}", self.kind())
    }
}

impl std::error::Error for Error {}
