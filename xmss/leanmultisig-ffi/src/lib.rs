use std::slice;
use std::sync::Once;

use leansig::serialization::Serializable;
use leansig::signature::generalized_xmss::instantiations_poseidon_top_level::lifetime_2_to_the_32::hashing_optimized::SIGTopLevelTargetSumLifetime32Dim64Base8 as SigScheme;
use leansig::signature::SignatureScheme;
use rec_aggregation::xmss_aggregate::{
    xmss_aggregate_signatures, xmss_setup_aggregation_program, xmss_verify_aggregated_signatures,
    Devnet2XmssAggregateSignature,
};
use ssz::{Decode, Encode};

type PublicKey = <SigScheme as SignatureScheme>::PublicKey;
type Signature = <SigScheme as SignatureScheme>::Signature;

const MESSAGE_HASH_LENGTH: usize = 32;

static PROVER_INIT: Once = Once::new();
static VERIFIER_INIT: Once = Once::new();

#[repr(C)]
pub enum LeanmultisigResult {
    Ok = 0,
    NullPointer = 1,
    InvalidLength = 2,
    LengthMismatch = 3,
    DeserializationFailed = 4,
    AggregationFailed = 5,
    VerificationFailed = 6,
}

#[repr(C)]
#[derive(Copy, Clone)]
pub struct LeanmultisigBytes {
    pub data: *const u8,
    pub len: usize,
}

fn setup_prover_once() {
    PROVER_INIT.call_once(|| {
        xmss_setup_aggregation_program();
    });
}

fn setup_verifier_once() {
    VERIFIER_INIT.call_once(|| {
        xmss_setup_aggregation_program();
    });
}

unsafe fn parse_message_hash(
    message_hash_ptr: *const u8,
    message_hash_len: usize,
) -> Result<[u8; MESSAGE_HASH_LENGTH], LeanmultisigResult> {
    if message_hash_ptr.is_null() {
        return Err(LeanmultisigResult::NullPointer);
    }
    if message_hash_len != MESSAGE_HASH_LENGTH {
        return Err(LeanmultisigResult::InvalidLength);
    }

    let mut message_hash = [0u8; MESSAGE_HASH_LENGTH];
    let hash_slice = unsafe { slice::from_raw_parts(message_hash_ptr, message_hash_len) };
    message_hash.copy_from_slice(hash_slice);
    Ok(message_hash)
}

unsafe fn parse_pubkeys(
    pubkeys: *const LeanmultisigBytes,
    num_pubkeys: usize,
) -> Result<Vec<PublicKey>, LeanmultisigResult> {
    if pubkeys.is_null() {
        return Err(LeanmultisigResult::NullPointer);
    }

    let pubkey_views = unsafe { slice::from_raw_parts(pubkeys, num_pubkeys) };
    let mut decoded_pubkeys = Vec::with_capacity(num_pubkeys);

    for view in pubkey_views {
        if view.data.is_null() || view.len == 0 {
            return Err(LeanmultisigResult::InvalidLength);
        }
        let bytes = unsafe { slice::from_raw_parts(view.data, view.len) };
        let decoded =
            PublicKey::from_bytes(bytes).map_err(|_| LeanmultisigResult::DeserializationFailed)?;
        decoded_pubkeys.push(decoded);
    }

    Ok(decoded_pubkeys)
}

unsafe fn parse_signatures(
    signatures: *const LeanmultisigBytes,
    num_signatures: usize,
) -> Result<Vec<Signature>, LeanmultisigResult> {
    if signatures.is_null() {
        return Err(LeanmultisigResult::NullPointer);
    }

    let signature_views = unsafe { slice::from_raw_parts(signatures, num_signatures) };
    let mut decoded_signatures = Vec::with_capacity(num_signatures);

    for view in signature_views {
        if view.data.is_null() || view.len == 0 {
            return Err(LeanmultisigResult::InvalidLength);
        }
        let bytes = unsafe { slice::from_raw_parts(view.data, view.len) };
        let decoded =
            Signature::from_bytes(bytes).map_err(|_| LeanmultisigResult::DeserializationFailed)?;
        decoded_signatures.push(decoded);
    }

    Ok(decoded_signatures)
}

/// Initialize prover-side aggregation setup. Idempotent.
#[unsafe(no_mangle)]
pub extern "C" fn leanmultisig_setup_prover() {
    setup_prover_once();
}

/// Initialize verifier-side setup. Idempotent.
#[unsafe(no_mangle)]
pub extern "C" fn leanmultisig_setup_verifier() {
    setup_verifier_once();
}

/// Aggregate XMSS signatures into a devnet-2 leanMultisig proof.
///
/// The caller owns the returned buffer and must free it via `leanmultisig_bytes_free`.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn leanmultisig_aggregate(
    pubkeys: *const LeanmultisigBytes,
    num_pubkeys: usize,
    signatures: *const LeanmultisigBytes,
    num_signatures: usize,
    message_hash_ptr: *const u8,
    message_hash_len: usize,
    epoch: u32,
    out_data: *mut *mut u8,
    out_len: *mut usize,
) -> LeanmultisigResult {
    if out_data.is_null() || out_len.is_null() {
        return LeanmultisigResult::NullPointer;
    }

    if num_pubkeys == 0 || num_signatures == 0 {
        return LeanmultisigResult::InvalidLength;
    }
    if num_pubkeys != num_signatures {
        return LeanmultisigResult::LengthMismatch;
    }

    let message_hash = match unsafe { parse_message_hash(message_hash_ptr, message_hash_len) } {
        Ok(hash) => hash,
        Err(err) => return err,
    };

    let decoded_pubkeys = match unsafe { parse_pubkeys(pubkeys, num_pubkeys) } {
        Ok(pks) => pks,
        Err(err) => return err,
    };

    let decoded_signatures = match unsafe { parse_signatures(signatures, num_signatures) } {
        Ok(sigs) => sigs,
        Err(err) => return err,
    };

    setup_prover_once();

    let aggregated_proof = match xmss_aggregate_signatures(
        &decoded_pubkeys,
        &decoded_signatures,
        &message_hash,
        epoch,
    ) {
        Ok(sig) => sig,
        Err(_) => return LeanmultisigResult::AggregationFailed,
    };

    let proof_bytes = aggregated_proof.as_ssz_bytes();
    let proof_len = proof_bytes.len();
    let proof_ptr = proof_bytes.leak().as_mut_ptr();

    unsafe {
        *out_data = proof_ptr;
        *out_len = proof_len;
    }
    LeanmultisigResult::Ok
}

/// Verify a devnet-2 leanMultisig proof.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn leanmultisig_verify_aggregated(
    pubkeys: *const LeanmultisigBytes,
    num_pubkeys: usize,
    message_hash_ptr: *const u8,
    message_hash_len: usize,
    proof_data: *const u8,
    proof_len: usize,
    epoch: u32,
) -> LeanmultisigResult {
    if num_pubkeys == 0 {
        return LeanmultisigResult::InvalidLength;
    }
    if proof_data.is_null() || proof_len == 0 {
        return LeanmultisigResult::InvalidLength;
    }

    let message_hash = match unsafe { parse_message_hash(message_hash_ptr, message_hash_len) } {
        Ok(hash) => hash,
        Err(err) => return err,
    };

    let decoded_pubkeys = match unsafe { parse_pubkeys(pubkeys, num_pubkeys) } {
        Ok(pks) => pks,
        Err(err) => return err,
    };

    let proof_slice = unsafe { slice::from_raw_parts(proof_data, proof_len) };
    let aggregated_proof = match Devnet2XmssAggregateSignature::from_ssz_bytes(proof_slice) {
        Ok(proof) => proof,
        Err(_) => return LeanmultisigResult::DeserializationFailed,
    };

    setup_verifier_once();

    match xmss_verify_aggregated_signatures(&decoded_pubkeys, &message_hash, &aggregated_proof, epoch)
    {
        Ok(_) => LeanmultisigResult::Ok,
        Err(_) => LeanmultisigResult::VerificationFailed,
    }
}

/// Free a buffer allocated by `leanmultisig_aggregate`.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn leanmultisig_bytes_free(data: *mut u8, len: usize) {
    if !data.is_null() && len > 0 {
        unsafe {
            drop(Vec::from_raw_parts(data, len, len));
        }
    }
}
