use rec_aggregation::xmss_aggregate::{
    config::{LeanSigPubKey, LeanSigSignature},
    xmss_aggregate_signatures, xmss_setup_aggregation_program, xmss_verify_aggregated_signatures,
    Devnet2XmssAggregateSignature,
};
use ssz::{Decode, Encode};
use std::slice;
use std::sync::Once;

use leansig::signature::generalized_xmss::instantiations_poseidon_top_level::lifetime_2_to_the_32::hashing_optimized::SIGTopLevelTargetSumLifetime32Dim64Base8;
use leansig::signature::SignatureScheme;

type HashSigScheme = SIGTopLevelTargetSumLifetime32Dim64Base8;
type HashSigPublicKey = <HashSigScheme as SignatureScheme>::PublicKey;
type HashSigSignature = <HashSigScheme as SignatureScheme>::Signature;

static PROVER_INIT: Once = Once::new();
static VERIFIER_INIT: Once = Once::new();

// Must match hashsig-glue's struct layout exactly.
#[repr(C)]
pub struct PublicKey {
    pub inner: HashSigPublicKey,
}

#[repr(C)]
pub struct Signature {
    pub inner: HashSigSignature,
}

pub fn to_ssz_bytes(agg_sig: &Devnet2XmssAggregateSignature) -> Vec<u8> {
    agg_sig.as_ssz_bytes()
}

pub fn from_ssz_bytes(bytes: &[u8]) -> Result<Devnet2XmssAggregateSignature, ssz::DecodeError> {
    Devnet2XmssAggregateSignature::from_ssz_bytes(bytes)
}

#[no_mangle]
pub extern "C" fn xmss_setup_prover() {
    PROVER_INIT.call_once(|| {
        xmss_setup_aggregation_program();
        whir_p3::precompute_dft_twiddles::<p3_koala_bear::KoalaBear>(1 << 24);
    });
}

#[no_mangle]
pub extern "C" fn xmss_setup_verifier() {
    VERIFIER_INIT.call_once(|| {
        xmss_setup_aggregation_program();
    });
}

#[no_mangle]
pub unsafe extern "C" fn xmss_aggregate(
    public_keys: *const *const PublicKey,
    num_keys: usize,
    signatures: *const *const Signature,
    num_sigs: usize,
    message_hash_ptr: *const u8,
    epoch: u32,
) -> *const Devnet2XmssAggregateSignature {
    if public_keys.is_null() || signatures.is_null() || message_hash_ptr.is_null() {
        return std::ptr::null();
    }
    if num_keys != num_sigs {
        return std::ptr::null();
    }

    let message_hash_slice = slice::from_raw_parts(message_hash_ptr, 32);
    let message_hash: &[u8; 32] = match message_hash_slice.try_into() {
        Ok(arr) => arr,
        Err(_) => return std::ptr::null_mut(),
    };

    let pub_key_ptrs = slice::from_raw_parts(public_keys, num_keys);
    let mut pub_keys: Vec<LeanSigPubKey> = Vec::with_capacity(num_keys);
    for &pk_ptr in pub_key_ptrs {
        if pk_ptr.is_null() {
            return std::ptr::null();
        }
        pub_keys.push((*pk_ptr).inner.clone());
    }

    let sig_ptrs = slice::from_raw_parts(signatures, num_sigs);
    let mut lean_signatures: Vec<LeanSigSignature> = Vec::with_capacity(num_sigs);
    for &sig_ptr in sig_ptrs {
        if sig_ptr.is_null() {
            return std::ptr::null();
        }
        lean_signatures.push((*sig_ptr).inner.clone());
    }

    // Run inline with catch_unwind to prevent CGo crash from Rust panics.
    // Panics can occur when SSZ-round-tripped signatures have corrupted rho fields.
    match std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| {
        xmss_aggregate_signatures(&pub_keys, &lean_signatures, message_hash, epoch)
    })) {
        Ok(Ok(sig)) => Box::into_raw(Box::new(sig)),
        Ok(Err(_)) => std::ptr::null(),
        Err(_) => std::ptr::null(), // panic caught
    }
}

#[no_mangle]
pub unsafe extern "C" fn xmss_verify_aggregated(
    public_keys: *const *const PublicKey,
    num_keys: usize,
    message_hash_ptr: *const u8,
    agg_sig: *const Devnet2XmssAggregateSignature,
    epoch: u32,
) -> bool {
    if public_keys.is_null() || message_hash_ptr.is_null() || agg_sig.is_null() {
        return false;
    }

    let message_hash_slice = slice::from_raw_parts(message_hash_ptr, 32);
    let message_hash: &[u8; 32] = match message_hash_slice.try_into() {
        Ok(arr) => arr,
        Err(_) => return false,
    };

    let pub_key_ptrs = slice::from_raw_parts(public_keys, num_keys);
    let mut pub_keys: Vec<LeanSigPubKey> = Vec::with_capacity(num_keys);
    for &pk_ptr in pub_key_ptrs {
        if pk_ptr.is_null() {
            return false;
        }
        pub_keys.push((*pk_ptr).inner.clone());
    }

    let agg_sig_ref = &*agg_sig;
    let message_owned = *message_hash;
    let epoch_owned = epoch;
    // Wrap in catch_unwind for CGo safety.
    std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| {
        xmss_verify_aggregated_signatures(&pub_keys, &message_owned, agg_sig_ref, epoch_owned)
            .is_ok()
    }))
    .unwrap_or_default()
}

#[no_mangle]
pub unsafe extern "C" fn xmss_free_aggregate_signature(
    agg_sig: *mut Devnet2XmssAggregateSignature,
) {
    if !agg_sig.is_null() {
        drop(Box::from_raw(agg_sig));
    }
}

#[no_mangle]
pub unsafe extern "C" fn xmss_aggregate_signature_to_bytes(
    agg_sig: *const Devnet2XmssAggregateSignature,
    buffer: *mut u8,
    buffer_len: usize,
) -> usize {
    if agg_sig.is_null() || buffer.is_null() {
        return 0;
    }
    let agg_sig_ref = &*agg_sig;
    let ssz_bytes = to_ssz_bytes(agg_sig_ref);
    if ssz_bytes.len() > buffer_len {
        return 0;
    }
    let output_slice = slice::from_raw_parts_mut(buffer, buffer_len);
    output_slice[..ssz_bytes.len()].copy_from_slice(&ssz_bytes);
    ssz_bytes.len()
}

#[no_mangle]
pub unsafe extern "C" fn xmss_aggregate_signature_from_bytes(
    bytes: *const u8,
    bytes_len: usize,
) -> *mut Devnet2XmssAggregateSignature {
    if bytes.is_null() || bytes_len == 0 {
        return std::ptr::null_mut();
    }
    let input_slice = slice::from_raw_parts(bytes, bytes_len);
    match from_ssz_bytes(input_slice) {
        Ok(agg_sig) => Box::into_raw(Box::new(agg_sig)),
        Err(_) => std::ptr::null_mut(),
    }
}
