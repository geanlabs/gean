use leansig_wrapper::{XmssPublicKey, XmssSignature};
use rec_aggregation::{
    init_aggregation_bytecode, xmss_aggregate as rec_xmss_aggregate, xmss_verify_aggregation,
    AggregatedXMSS,
};
use std::slice;
use std::sync::Once;

// Mirror hashsig-glue's struct layout with #[repr(C)]
// These must match hashsig-glue/src/lib.rs exactly
#[repr(C)]
pub struct PublicKey {
    pub inner: XmssPublicKey,
}

#[repr(C)]
pub struct Signature {
    pub inner: XmssSignature,
}

static PROVER_INIT: Once = Once::new();
static VERIFIER_INIT: Once = Once::new();

#[no_mangle]
pub extern "C" fn xmss_setup_prover() {
    PROVER_INIT.call_once(|| {
        init_aggregation_bytecode();
        backend::precompute_dft_twiddles::<backend::KoalaBear>(1 << 24);
    });
}

#[no_mangle]
pub extern "C" fn xmss_setup_verifier() {
    VERIFIER_INIT.call_once(|| {
        init_aggregation_bytecode();
    });
}

/// Aggregate signatures with recursive child proof support.
/// Returns pointer to AggregatedXMSS on success, null on error.
///
/// # Safety
/// - `raw_pub_keys` must point to an array of `num_raw` valid pointers to `PublicKey`.
/// - `raw_signatures` must point to an array of `num_raw` valid pointers to `Signature`.
/// - When `num_children > 0`:
///   - `child_all_pub_keys` must point to a flat array of PublicKey pointers
///     with total length = sum of `child_num_keys[0..num_children]`.
///   - `child_num_keys` must point to an array of `num_children` elements.
///   - `child_proof_ptrs` must point to an array of `num_children` pointers to proof bytes.
///   - `child_proof_lens` must point to an array of `num_children` lengths.
/// - `message_hash_ptr` must point to at least 32 bytes.
#[no_mangle]
pub unsafe extern "C" fn xmss_aggregate(
    // Raw XMSS signatures
    raw_pub_keys: *const *const PublicKey,
    raw_signatures: *const *const Signature,
    num_raw: usize,
    // Children
    num_children: usize,
    child_all_pub_keys: *const *const PublicKey,
    child_num_keys: *const usize,
    child_proof_ptrs: *const *const u8,
    child_proof_lens: *const usize,
    // Common parameters
    message_hash_ptr: *const u8,
    slot: u32,
    log_inv_rate: usize,
) -> *const AggregatedXMSS {
    if message_hash_ptr.is_null() {
        return std::ptr::null();
    }
    if num_raw > 0 && (raw_pub_keys.is_null() || raw_signatures.is_null()) {
        return std::ptr::null();
    }
    if num_children > 0
        && (child_all_pub_keys.is_null()
            || child_num_keys.is_null()
            || child_proof_ptrs.is_null()
            || child_proof_lens.is_null())
    {
        return std::ptr::null();
    }

    let message_hash: &[u8; 32] = match slice::from_raw_parts(message_hash_ptr, 32).try_into() {
        Ok(arr) => arr,
        Err(_) => return std::ptr::null(),
    };

    // Build raw XMSS pairs: (XmssPublicKey, XmssSignature)
    let mut raw_xmss: Vec<(XmssPublicKey, XmssSignature)> = Vec::with_capacity(num_raw);
    if num_raw > 0 {
        let pk_ptrs = slice::from_raw_parts(raw_pub_keys, num_raw);
        let sig_ptrs = slice::from_raw_parts(raw_signatures, num_raw);
        for i in 0..num_raw {
            if pk_ptrs[i].is_null() || sig_ptrs[i].is_null() {
                return std::ptr::null();
            }
            raw_xmss.push(((*pk_ptrs[i]).inner.clone(), (*sig_ptrs[i]).inner.clone()));
        }
    }

    // Build children: Vec<(&[XmssPublicKey], AggregatedXMSS)>
    let mut children_pks: Vec<Vec<XmssPublicKey>> = Vec::with_capacity(num_children);
    let mut children_proofs: Vec<AggregatedXMSS> = Vec::with_capacity(num_children);

    if num_children > 0 {
        let num_keys_arr = slice::from_raw_parts(child_num_keys, num_children);
        let proof_ptrs = slice::from_raw_parts(child_proof_ptrs, num_children);
        let proof_lens = slice::from_raw_parts(child_proof_lens, num_children);

        let total_child_pks: usize = num_keys_arr.iter().sum();
        let all_pk_ptrs = slice::from_raw_parts(child_all_pub_keys, total_child_pks);

        let mut pk_offset: usize = 0;
        for i in 0..num_children {
            // Collect pub keys for this child
            let n = num_keys_arr[i];
            let mut pks = Vec::with_capacity(n);
            for j in 0..n {
                let pk_ptr = all_pk_ptrs[pk_offset + j];
                if pk_ptr.is_null() {
                    return std::ptr::null();
                }
                pks.push((*pk_ptr).inner.clone());
            }
            pk_offset += n;
            children_pks.push(pks);

            // Deserialize child proof
            if proof_ptrs[i].is_null() || proof_lens[i] == 0 {
                return std::ptr::null();
            }
            let proof_bytes = slice::from_raw_parts(proof_ptrs[i], proof_lens[i]);
            let proof = match AggregatedXMSS::deserialize(proof_bytes) {
                Some(p) => p,
                None => return std::ptr::null(),
            };
            children_proofs.push(proof);
        }
    }

    // Build children_with_keys: &[(&[XmssPublicKey], AggregatedXMSS)]
    let children_with_keys: Vec<(&[XmssPublicKey], AggregatedXMSS)> = children_pks
        .iter()
        .zip(children_proofs)
        .map(|(pks, proof)| (pks.as_slice(), proof))
        .collect();

    // Call recursive aggregation with catch_unwind for CGo safety.
    match std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| {
        rec_xmss_aggregate(
            &children_with_keys,
            raw_xmss,
            message_hash,
            slot,
            log_inv_rate,
        )
    })) {
        Ok((_pub_keys, agg_sig)) => Box::into_raw(Box::new(agg_sig)),
        Err(_) => std::ptr::null(), // panic caught
    }
}

/// Verify aggregated signatures.
#[no_mangle]
pub unsafe extern "C" fn xmss_verify_aggregated(
    public_keys: *const *const PublicKey,
    num_keys: usize,
    message_hash_ptr: *const u8,
    agg_sig_bytes: *const u8,
    agg_sig_len: usize,
    epoch: u32,
) -> bool {
    if public_keys.is_null() || message_hash_ptr.is_null() || agg_sig_bytes.is_null() {
        return false;
    }

    let message_hash: &[u8; 32] = match slice::from_raw_parts(message_hash_ptr, 32).try_into() {
        Ok(arr) => arr,
        Err(_) => return false,
    };

    let pub_key_ptrs = slice::from_raw_parts(public_keys, num_keys);
    let mut pub_keys: Vec<XmssPublicKey> = Vec::with_capacity(num_keys);
    for &pk_ptr in pub_key_ptrs {
        if pk_ptr.is_null() {
            return false;
        }
        pub_keys.push((*pk_ptr).inner.clone());
    }

    let proof_bytes = slice::from_raw_parts(agg_sig_bytes, agg_sig_len);
    let agg_sig = match AggregatedXMSS::deserialize(proof_bytes) {
        Some(sig) => sig,
        None => return false,
    };

    std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| {
        xmss_verify_aggregation(pub_keys, &agg_sig, message_hash, epoch).is_ok()
    }))
    .unwrap_or_default()
}

#[no_mangle]
pub unsafe extern "C" fn xmss_free_aggregate_signature(agg_sig: *mut AggregatedXMSS) {
    if !agg_sig.is_null() {
        drop(Box::from_raw(agg_sig));
    }
}

#[no_mangle]
pub unsafe extern "C" fn xmss_aggregate_signature_to_bytes(
    agg_sig: *const AggregatedXMSS,
    buffer: *mut u8,
    buffer_len: usize,
) -> usize {
    if agg_sig.is_null() || buffer.is_null() {
        return 0;
    }
    let agg_sig_ref = &*agg_sig;
    let ssz_bytes = agg_sig_ref.serialize();
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
) -> *mut AggregatedXMSS {
    if bytes.is_null() || bytes_len == 0 {
        return std::ptr::null_mut();
    }
    let input_slice = slice::from_raw_parts(bytes, bytes_len);
    match AggregatedXMSS::deserialize(input_slice) {
        Some(agg_sig) => Box::into_raw(Box::new(agg_sig)),
        None => std::ptr::null_mut(),
    }
}
