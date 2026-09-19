use leanvm::xmss::{Epoch, Message, XmssPublicKey, XmssSignature, MESSAGE_LEN};
use leanvm::{
    aggregate, setup_prover, setup_prover_without_arena, setup_verifier, AggregationError,
    ClaimSelection, EthereumProof, SignatureClaims, XmssClaimGroup,
};
use std::panic::AssertUnwindSafe;
use std::slice;
use std::sync::OnceLock;

#[repr(C)]
pub struct PublicKey {
    pub inner: XmssPublicKey,
}

#[repr(C)]
pub struct Signature {
    pub inner: XmssSignature,
}

static PROVER_READY: OnceLock<bool> = OnceLock::new();
static VERIFIER_READY: OnceLock<bool> = OnceLock::new();

macro_rules! ffi_guard {
    ($fallback:expr, $body:block) => {
        std::panic::catch_unwind(AssertUnwindSafe(|| $body)).unwrap_or($fallback)
    };
}

// leanVM allows one `aggregate` call at a time per process, and setup_prover's arena is a
// single shared region. The Go-side proving.Gate serializes all aggregate, merge and split
// work, so that holds. Verification is not affected and stays safe to run concurrently.
// Only one of the two prover setups is ever called (shared readiness latch), chosen once at
// startup.
#[no_mangle]
pub extern "C" fn xmss_setup_prover() -> i32 {
    let ready = PROVER_READY.get_or_init(|| std::panic::catch_unwind(setup_prover).is_ok());
    if *ready {
        0
    } else {
        -1
    }
}

#[no_mangle]
pub extern "C" fn xmss_setup_prover_without_arena() -> i32 {
    let ready =
        PROVER_READY.get_or_init(|| std::panic::catch_unwind(setup_prover_without_arena).is_ok());
    if *ready {
        0
    } else {
        -1
    }
}

#[no_mangle]
pub extern "C" fn xmss_setup_verifier() -> i32 {
    let ready = VERIFIER_READY.get_or_init(|| std::panic::catch_unwind(setup_verifier).is_ok());
    if *ready {
        0
    } else {
        -1
    }
}

unsafe fn write_out(src: &[u8], out: *mut u8, cap: usize, written: *mut usize) -> i32 {
    if written.is_null() {
        return -1;
    }
    *written = src.len();
    if src.len() > cap {
        return -2;
    }
    if !src.is_empty() {
        if out.is_null() {
            return -1;
        }
        std::ptr::copy_nonoverlapping(src.as_ptr(), out, src.len());
    }
    0
}

unsafe fn read_message(ptr: *const u8) -> Option<Message> {
    if ptr.is_null() {
        return None;
    }
    slice::from_raw_parts(ptr, MESSAGE_LEN).try_into().ok()
}

unsafe fn collect_pubkeys(
    ptrs: *const *const PublicKey,
    count: usize,
) -> Option<Vec<XmssPublicKey>> {
    if count == 0 {
        return Some(Vec::new());
    }
    if ptrs.is_null() {
        return None;
    }
    let mut keys = Vec::with_capacity(count);
    for &ptr in slice::from_raw_parts(ptrs, count) {
        if ptr.is_null() {
            return None;
        }
        keys.push((*ptr).inner.clone());
    }
    Some(keys)
}

/// Reads `count` claim groups: group `i` holds `pubkey_counts[i]` keys from the flattened
/// `pubkeys`, which signed the `i`th 32-byte message in `message_hashes` at `slots[i]`.
unsafe fn collect_groups(
    pubkeys: *const *const PublicKey,
    pubkey_counts: *const usize,
    message_hashes: *const u8,
    slots: *const u32,
    count: usize,
) -> Option<Vec<XmssClaimGroup>> {
    if count == 0
        || pubkeys.is_null()
        || pubkey_counts.is_null()
        || message_hashes.is_null()
        || slots.is_null()
    {
        return None;
    }
    let counts = slice::from_raw_parts(pubkey_counts, count);
    let slots = slice::from_raw_parts(slots, count);
    let mut groups = Vec::with_capacity(count);
    let mut offset = 0usize;
    for i in 0..count {
        groups.push(XmssClaimGroup {
            epoch: slots[i],
            message: read_message(message_hashes.add(i.checked_mul(MESSAGE_LEN)?))?,
            keys: collect_pubkeys(pubkeys.add(offset), counts[i])?,
        });
        offset = offset.checked_add(counts[i])?;
    }
    Some(groups)
}

/// Reads `count` raw signatures: signature `i` is by `keys[i]` over the `i`th 32-byte message
/// in `message_hashes`, at `slots[i]`.
unsafe fn collect_raw(
    keys: *const *const PublicKey,
    signatures: *const *const Signature,
    message_hashes: *const u8,
    slots: *const u32,
    count: usize,
) -> Option<Vec<(XmssPublicKey, Epoch, Message, XmssSignature)>> {
    if count == 0 {
        return Some(Vec::new());
    }
    if keys.is_null() || signatures.is_null() || message_hashes.is_null() || slots.is_null() {
        return None;
    }
    let keys = slice::from_raw_parts(keys, count);
    let signatures = slice::from_raw_parts(signatures, count);
    let slots = slice::from_raw_parts(slots, count);
    let mut raw = Vec::with_capacity(count);
    for i in 0..count {
        if keys[i].is_null() || signatures[i].is_null() {
            return None;
        }
        raw.push((
            (*keys[i]).inner.clone(),
            slots[i],
            read_message(message_hashes.add(i.checked_mul(MESSAGE_LEN)?))?,
            (*signatures[i]).inner.clone(),
        ));
    }
    Some(raw)
}

/// The signer set a proof over `groups` is bound to. leanVM requires keys strictly sorted
/// within a group and groups strictly increasing by epoch, and the prover and every verifier
/// must derive the identical set, so it is built here in one place: groups sharing an epoch
/// and message are merged, keys are sorted and deduplicated. Two different messages at one
/// epoch cannot be carried by a single proof, and yield None.
fn signature_claims(mut groups: Vec<XmssClaimGroup>) -> Option<SignatureClaims> {
    groups.sort_by_key(|group| group.epoch);
    let mut merged: Vec<XmssClaimGroup> = Vec::with_capacity(groups.len());
    for group in groups {
        match merged.last_mut() {
            Some(last) if last.epoch == group.epoch => {
                if last.message != group.message {
                    return None;
                }
                last.keys.extend(group.keys);
            }
            _ => merged.push(group),
        }
    }
    for group in &mut merged {
        group.keys.sort();
        group.keys.dedup();
    }
    Some(SignatureClaims {
        xmss: merged,
        sphincs: Vec::new(),
    })
}

fn single_group(
    epoch: Epoch,
    message: Message,
    keys: Vec<XmssPublicKey>,
) -> Option<SignatureClaims> {
    signature_claims(vec![XmssClaimGroup {
        epoch,
        message,
        keys,
    }])
}

/// Decodes proof bytes against the claims the caller expects them to prove. The bytes carry
/// no claims of their own, so a proof decoded against the wrong claims fails verification.
unsafe fn decode_proof(
    proof: *const u8,
    proof_len: usize,
    claims: SignatureClaims,
) -> Option<EthereumProof> {
    if proof.is_null() || proof_len == 0 {
        return None;
    }
    EthereumProof::from_bytes_without_pubkeys(slice::from_raw_parts(proof, proof_len), claims).ok()
}

/// Why a proving call failed, returned as its status so a failure names its cause. The
/// Go side maps each code to a message (xmss/ffi.go). -1 is an invalid argument and -2 an
/// output buffer too small.
#[derive(Clone, Copy)]
#[repr(i32)]
enum Failure {
    UndecodableInput = -3,
    ConflictingMessages = -4,
    SplitTargetMissing = -5,
    Panicked = -6,
    InvalidChild = -10,
    MalformedRawSignature = -11,
    TooManyEpochs = -12,
    TooLarge = -13,
    ChildOutOfRange = -14,
    NotCovered = -15,
    Empty = -16,
    InvalidRate = -17,
    InvalidBlob = -18,
}

impl From<AggregationError> for Failure {
    fn from(err: AggregationError) -> Self {
        match err {
            AggregationError::ConflictingMessages => Failure::ConflictingMessages,
            AggregationError::InvalidChild(_) => Failure::InvalidChild,
            AggregationError::MalformedRawSignature => Failure::MalformedRawSignature,
            AggregationError::TooManyEpochs => Failure::TooManyEpochs,
            AggregationError::TooLarge => Failure::TooLarge,
            AggregationError::ChildOutOfRange { .. } => Failure::ChildOutOfRange,
            AggregationError::NotCovered => Failure::NotCovered,
            AggregationError::Empty => Failure::Empty,
            AggregationError::InvalidRate { .. } => Failure::InvalidRate,
            AggregationError::InvalidBlobSize { .. } | AggregationError::BlobNotCovered => {
                Failure::InvalidBlob
            }
        }
    }
}

/// XMSS-only `aggregate`. It panics on an invalid raw signature, so every caller runs it
/// inside `ffi_guard`.
fn prove(
    children: &[EthereumProof],
    raw: Vec<(XmssPublicKey, Epoch, Message, XmssSignature)>,
    declare: Option<ClaimSelection<'_>>,
    log_inv_rate: usize,
) -> Result<EthereumProof, Failure> {
    aggregate(children, raw, Vec::new(), &[], declare, log_inv_rate).map_err(Failure::from)
}

#[no_mangle]
pub unsafe extern "C" fn xmss_aggregate_type_1(
    raw_pub_keys: *const *const PublicKey,
    raw_signatures: *const *const Signature,
    num_raw: usize,
    child_all_pub_keys: *const *const PublicKey,
    child_num_keys: *const usize,
    child_proof_ptrs: *const *const u8,
    child_proof_lens: *const usize,
    num_children: usize,
    message_hash: *const u8,
    slot: u32,
    log_inv_rate: usize,
    out: *mut u8,
    cap: usize,
    written: *mut usize,
) -> i32 {
    ffi_guard!(Failure::Panicked as i32, {
        if written.is_null()
            || (num_raw > 0 && (raw_pub_keys.is_null() || raw_signatures.is_null()))
            || (num_children > 0
                && (child_all_pub_keys.is_null()
                    || child_num_keys.is_null()
                    || child_proof_ptrs.is_null()
                    || child_proof_lens.is_null()))
        {
            return -1;
        }
        let Some(message) = read_message(message_hash) else {
            return -1;
        };

        let mut raw = Vec::with_capacity(num_raw);
        if num_raw > 0 {
            let keys = slice::from_raw_parts(raw_pub_keys, num_raw);
            let signatures = slice::from_raw_parts(raw_signatures, num_raw);
            for i in 0..num_raw {
                if keys[i].is_null() || signatures[i].is_null() {
                    return -1;
                }
                raw.push((
                    (*keys[i]).inner.clone(),
                    slot,
                    message,
                    (*signatures[i]).inner.clone(),
                ));
            }
        }

        let mut children = Vec::with_capacity(num_children);
        if num_children > 0 {
            let counts = slice::from_raw_parts(child_num_keys, num_children);
            let proofs = slice::from_raw_parts(child_proof_ptrs, num_children);
            let lengths = slice::from_raw_parts(child_proof_lens, num_children);
            let mut offset = 0usize;
            for i in 0..num_children {
                let Some(keys) = collect_pubkeys(child_all_pub_keys.add(offset), counts[i]) else {
                    return -1;
                };
                let Some(next) = offset.checked_add(counts[i]) else {
                    return -1;
                };
                offset = next;
                let Some(claims) = single_group(slot, message, keys) else {
                    return -1;
                };
                match decode_proof(proofs[i], lengths[i], claims) {
                    Some(proof) => children.push(proof),
                    None => return Failure::UndecodableInput as i32,
                }
            }
        }

        match prove(&children, raw, None, log_inv_rate) {
            Ok(proof) => write_out(&proof.to_bytes_without_pubkeys(), out, cap, written),
            Err(failure) => failure as i32,
        }
    })
}

#[no_mangle]
pub unsafe extern "C" fn xmss_verify_type_1(
    public_keys: *const *const PublicKey,
    num_keys: usize,
    message_hash: *const u8,
    slot: u32,
    proof: *const u8,
    proof_len: usize,
) -> bool {
    ffi_guard!(false, {
        let Some(message) = read_message(message_hash) else {
            return false;
        };
        let Some(claims) = collect_pubkeys(public_keys, num_keys)
            .and_then(|keys| single_group(slot, message, keys))
        else {
            return false;
        };
        decode_proof(proof, proof_len, claims).is_some_and(|proof| proof.verify().is_ok())
    })
}

/// Merges Type-1 proofs, and raw signatures folded in as they are, into one Type-2. Proving
/// a raw signature on its own first and merging that proof would cost a whole extra proof.
#[no_mangle]
pub unsafe extern "C" fn xmss_merge_type_1_to_type_2(
    proof_ptrs: *const *const u8,
    proof_lens: *const usize,
    pubkeys: *const *const PublicKey,
    pubkey_counts: *const usize,
    message_hashes: *const u8,
    message_slots: *const u32,
    count: usize,
    raw_pub_keys: *const *const PublicKey,
    raw_signatures: *const *const Signature,
    raw_message_hashes: *const u8,
    raw_slots: *const u32,
    num_raw: usize,
    log_inv_rate: usize,
    out: *mut u8,
    cap: usize,
    written: *mut usize,
) -> i32 {
    ffi_guard!(Failure::Panicked as i32, {
        if written.is_null() {
            return -1;
        }
        let mut children = Vec::with_capacity(count);
        if count > 0 {
            if proof_ptrs.is_null() || proof_lens.is_null() {
                return -1;
            }
            let Some(groups) =
                collect_groups(pubkeys, pubkey_counts, message_hashes, message_slots, count)
            else {
                return -1;
            };
            let proof_ptrs = slice::from_raw_parts(proof_ptrs, count);
            let proof_lens = slice::from_raw_parts(proof_lens, count);
            for (i, group) in groups.into_iter().enumerate() {
                let Some(claims) = signature_claims(vec![group]) else {
                    return -1;
                };
                match decode_proof(proof_ptrs[i], proof_lens[i], claims) {
                    Some(proof) => children.push(proof),
                    None => return Failure::UndecodableInput as i32,
                }
            }
        }
        let Some(raw) = collect_raw(
            raw_pub_keys,
            raw_signatures,
            raw_message_hashes,
            raw_slots,
            num_raw,
        ) else {
            return -1;
        };
        match prove(&children, raw, None, log_inv_rate) {
            Ok(proof) => write_out(&proof.to_bytes_without_pubkeys(), out, cap, written),
            Err(failure) => failure as i32,
        }
    })
}

/// Re-proves the one group of a Type-2 proof that signed `target_message` as a standalone
/// Type-1. leanVM has no cheaper split: the Type-2 is a child of a new proof that declares
/// only the kept group.
#[no_mangle]
pub unsafe extern "C" fn xmss_split_type_2_by_message(
    proof: *const u8,
    proof_len: usize,
    pubkeys: *const *const PublicKey,
    pubkey_counts: *const usize,
    message_hashes: *const u8,
    message_slots: *const u32,
    count: usize,
    target_message: *const u8,
    log_inv_rate: usize,
    out: *mut u8,
    cap: usize,
    written: *mut usize,
) -> i32 {
    ffi_guard!(Failure::Panicked as i32, {
        if written.is_null() {
            return -1;
        }
        let Some(target) = read_message(target_message) else {
            return -1;
        };
        let Some(groups) =
            collect_groups(pubkeys, pubkey_counts, message_hashes, message_slots, count)
        else {
            return -1;
        };
        let Some(claims) = signature_claims(groups) else {
            return Failure::ConflictingMessages as i32;
        };
        let mut kept = claims.xmss.iter().filter(|group| group.message == target);
        let (Some(group), None) = (kept.next(), kept.next()) else {
            return Failure::SplitTargetMissing as i32;
        };
        let kept = SignatureClaims {
            xmss: vec![group.clone()],
            sphincs: Vec::new(),
        };
        let Some(type_2) = decode_proof(proof, proof_len, claims) else {
            return Failure::UndecodableInput as i32;
        };
        let declare = ClaimSelection {
            signatures: &kept,
            da_commitments: &[],
        };
        match prove(&[type_2], Vec::new(), Some(declare), log_inv_rate) {
            Ok(proof) => write_out(&proof.to_bytes_without_pubkeys(), out, cap, written),
            Err(failure) => failure as i32,
        }
    })
}

#[no_mangle]
pub unsafe extern "C" fn xmss_verify_type_2(
    proof: *const u8,
    proof_len: usize,
    pubkeys: *const *const PublicKey,
    pubkey_counts: *const usize,
    count: usize,
    message_hashes: *const u8,
    message_slots: *const u32,
) -> bool {
    ffi_guard!(false, {
        let Some(claims) =
            collect_groups(pubkeys, pubkey_counts, message_hashes, message_slots, count)
                .and_then(signature_claims)
        else {
            return false;
        };
        decode_proof(proof, proof_len, claims).is_some_and(|proof| proof.verify().is_ok())
    })
}
