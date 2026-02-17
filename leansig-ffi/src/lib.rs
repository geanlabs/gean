//! C-compatible FFI wrapper around the leansig XMSS signature scheme.
//!
//! This crate provides a C API for the leansig library's generalized XMSS
//! signature scheme, targeted at the devnet-1 instantiation:
//! `SIGTopLevelTargetSumLifetime32Dim64Base8` (LOG_LIFETIME=32, DIM=64, BASE=8).
//! All types are passed as opaque pointers or SSZ-serialized byte buffers.
//! Memory management follows the "caller frees" pattern: every `_new` or
//! `_generate` function has a corresponding `_free` function.

use std::slice;

use rand::SeedableRng;

use leansig::serialization::Serializable;
use leansig::signature::generalized_xmss::instantiations_poseidon_top_level::lifetime_2_to_the_32::hashing_optimized::SIGTopLevelTargetSumLifetime32Dim64Base8 as SigScheme;
use leansig::signature::{SignatureScheme, SignatureSchemeSecretKey};

// Concrete type aliases for the devnet-1 instantiation.
type PublicKey = <SigScheme as SignatureScheme>::PublicKey;
type SecretKey = <SigScheme as SignatureScheme>::SecretKey;
type Signature = <SigScheme as SignatureScheme>::Signature;

/// Result codes returned by FFI functions.
#[repr(C)]
pub enum LeansigResult {
    /// Operation succeeded.
    Ok = 0,
    /// Null pointer argument.
    NullPointer = 1,
    /// Invalid buffer length.
    InvalidLength = 2,
    /// Signing failed (encoding attempts exceeded).
    SigningFailed = 3,
    /// Deserialization (from_bytes) failed.
    DeserializationFailed = 4,
    /// Signature verification failed.
    VerificationFailed = 5,
    /// Epoch outside prepared interval.
    EpochNotPrepared = 6,
}

/// Opaque keypair holding both public and secret keys.
pub struct LeansigKeypair {
    pk: PublicKey,
    sk: SecretKey,
}

// ---------------------------------------------------------------------------
// Key generation
// ---------------------------------------------------------------------------

/// Generate a new XMSS keypair.
///
/// # Arguments
/// * `seed` - Random seed for the RNG (will be used to seed a ChaCha RNG).
/// * `activation_epoch` - Starting epoch for which the key is active.
/// * `num_active_epochs` - Number of consecutive active epochs.
/// * `out_keypair` - Pointer to receive the opaque keypair handle.
///
/// # Returns
/// `LeansigResult::Ok` on success.
///
/// # Note
/// Key generation is performed on a dedicated thread with a large stack
/// (64 MB) to accommodate the deep recursion required by XMSS Merkle tree
/// construction with LOG_LIFETIME=32.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn leansig_keypair_generate(
    seed: u64,
    activation_epoch: u64,
    num_active_epochs: u64,
    out_keypair: *mut *mut LeansigKeypair,
) -> LeansigResult {
    if out_keypair.is_null() {
        return LeansigResult::NullPointer;
    }

    // Spawn key_gen on a thread with 64 MB stack to avoid stack overflow
    // from deep Merkle tree recursion in the LOG_LIFETIME=32 instantiation.
    const STACK_SIZE: usize = 64 * 1024 * 1024; // 64 MB
    let handle = std::thread::Builder::new()
        .stack_size(STACK_SIZE)
        .spawn(move || {
            let mut rng = rand::rngs::SmallRng::seed_from_u64(seed);
            SigScheme::key_gen(
                &mut rng,
                activation_epoch as usize,
                num_active_epochs as usize,
            )
        });

    match handle {
        Ok(join_handle) => match join_handle.join() {
            Ok((pk, sk)) => {
                let keypair = Box::new(LeansigKeypair { pk, sk });
                unsafe {
                    *out_keypair = Box::into_raw(keypair);
                }
                LeansigResult::Ok
            }
            Err(_) => LeansigResult::SigningFailed, // thread panicked
        },
        Err(_) => LeansigResult::SigningFailed, // couldn't spawn thread
    }
}

/// Restore a keypair from serialized public and secret key bytes.
///
/// # Arguments
/// * `pk_bytes` - Pointer to the serialized public key bytes.
/// * `pk_len` - Length of the public key bytes.
/// * `sk_bytes` - Pointer to the serialized secret key bytes.
/// * `sk_len` - Length of the secret key bytes.
/// * `out_keypair` - Pointer to receive the opaque keypair handle.
///
/// # Returns
/// `LeansigResult::Ok` on success, or `DeserializationFailed` if bytes are invalid.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn leansig_keypair_restore(
    pk_bytes: *const u8,
    pk_len: usize,
    sk_bytes: *const u8,
    sk_len: usize,
    out_keypair: *mut *mut LeansigKeypair,
) -> LeansigResult {
    if pk_bytes.is_null() || sk_bytes.is_null() || out_keypair.is_null() {
        return LeansigResult::NullPointer;
    }

    let pk_slice = unsafe { slice::from_raw_parts(pk_bytes, pk_len) };
    let sk_slice = unsafe { slice::from_raw_parts(sk_bytes, sk_len) };

    let pk = match PublicKey::from_bytes(pk_slice) {
        Ok(k) => k,
        Err(_) => return LeansigResult::DeserializationFailed,
    };

    let sk = match SecretKey::from_bytes(sk_slice) {
        Ok(k) => k,
        Err(_) => return LeansigResult::DeserializationFailed,
    };

    let keypair = Box::new(LeansigKeypair { pk, sk });
    unsafe {
        *out_keypair = Box::into_raw(keypair);
    }
    LeansigResult::Ok
}

/// Free a keypair allocated by `leansig_keypair_generate`.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn leansig_keypair_free(keypair: *mut LeansigKeypair) {
    if !keypair.is_null() {
        unsafe {
            drop(Box::from_raw(keypair));
        }
    }
}

// ---------------------------------------------------------------------------
// Public key serialization
// ---------------------------------------------------------------------------

/// Get the SSZ-serialized public key from a keypair.
///
/// The caller must free the returned buffer with `leansig_bytes_free`.
///
/// # Arguments
/// * `keypair` - Opaque keypair handle.
/// * `out_data` - Pointer to receive the byte buffer.
/// * `out_len` - Pointer to receive the buffer length.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn leansig_pubkey_serialize(
    keypair: *const LeansigKeypair,
    out_data: *mut *mut u8,
    out_len: *mut usize,
) -> LeansigResult {
    if keypair.is_null() || out_data.is_null() || out_len.is_null() {
        return LeansigResult::NullPointer;
    }

    let keypair = unsafe { &*keypair };
    let bytes = keypair.pk.to_bytes();

    let len = bytes.len();
    let ptr = bytes.leak().as_mut_ptr();

    unsafe {
        *out_data = ptr;
        *out_len = len;
    }
    LeansigResult::Ok
}

/// Get the SSZ-serialized secret key from a keypair.
///
/// The caller must free the returned buffer with `leansig_bytes_free`.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn leansig_seckey_serialize(
    keypair: *const LeansigKeypair,
    out_data: *mut *mut u8,
    out_len: *mut usize,
) -> LeansigResult {
    if keypair.is_null() || out_data.is_null() || out_len.is_null() {
        return LeansigResult::NullPointer;
    }

    let keypair = unsafe { &*keypair };
    let bytes = keypair.sk.to_bytes();

    let len = bytes.len();
    let ptr = bytes.leak().as_mut_ptr();

    unsafe {
        *out_data = ptr;
        *out_len = len;
    }
    LeansigResult::Ok
}

/// Free a byte buffer returned by any `leansig_*_serialize` function.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn leansig_bytes_free(data: *mut u8, len: usize) {
    if !data.is_null() && len > 0 {
        unsafe {
            drop(Vec::from_raw_parts(data, len, len));
        }
    }
}

// ---------------------------------------------------------------------------
// Secret key operations
// ---------------------------------------------------------------------------

/// Get the start of the activation interval for this secret key.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn leansig_sk_activation_start(keypair: *const LeansigKeypair) -> u64 {
    if keypair.is_null() {
        return 0;
    }
    let keypair = unsafe { &*keypair };
    keypair.sk.get_activation_interval().start
}

/// Get the end (exclusive) of the activation interval for this secret key.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn leansig_sk_activation_end(keypair: *const LeansigKeypair) -> u64 {
    if keypair.is_null() {
        return 0;
    }
    let keypair = unsafe { &*keypair };
    keypair.sk.get_activation_interval().end
}

/// Get the start of the currently prepared interval.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn leansig_sk_prepared_start(keypair: *const LeansigKeypair) -> u64 {
    if keypair.is_null() {
        return 0;
    }
    let keypair = unsafe { &*keypair };
    keypair.sk.get_prepared_interval().start
}

/// Get the end (exclusive) of the currently prepared interval.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn leansig_sk_prepared_end(keypair: *const LeansigKeypair) -> u64 {
    if keypair.is_null() {
        return 0;
    }
    let keypair = unsafe { &*keypair };
    keypair.sk.get_prepared_interval().end
}

/// Advance the secret key's prepared interval to the next window.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn leansig_sk_advance_preparation(
    keypair: *mut LeansigKeypair,
) -> LeansigResult {
    if keypair.is_null() {
        return LeansigResult::NullPointer;
    }
    let keypair = unsafe { &mut *keypair };
    keypair.sk.advance_preparation();
    LeansigResult::Ok
}

// ---------------------------------------------------------------------------
// Signing
// ---------------------------------------------------------------------------

/// Sign a 32-byte message at a given epoch.
///
/// The caller must free the returned signature buffer with `leansig_bytes_free`.
///
/// # Arguments
/// * `keypair` - Opaque keypair handle (secret key is used).
/// * `epoch` - The epoch to sign at (must be in the prepared interval).
/// * `message` - Pointer to 32-byte message.
/// * `out_sig_data` - Pointer to receive the SSZ-serialized signature bytes.
/// * `out_sig_len` - Pointer to receive the signature length.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn leansig_sign(
    keypair: *const LeansigKeypair,
    epoch: u32,
    message: *const u8,
    out_sig_data: *mut *mut u8,
    out_sig_len: *mut usize,
) -> LeansigResult {
    if keypair.is_null() || message.is_null() || out_sig_data.is_null() || out_sig_len.is_null() {
        return LeansigResult::NullPointer;
    }

    let keypair = unsafe { &*keypair };
    let msg: &[u8; 32] = unsafe { &*(message as *const [u8; 32]) };

    // Check epoch is in prepared interval
    if !keypair.sk.get_prepared_interval().contains(&(epoch as u64)) {
        return LeansigResult::EpochNotPrepared;
    }

    match SigScheme::sign(&keypair.sk, epoch, msg) {
        Ok(sig) => {
            let bytes = sig.to_bytes();
            let len = bytes.len();
            let ptr = bytes.leak().as_mut_ptr();
            unsafe {
                *out_sig_data = ptr;
                *out_sig_len = len;
            }
            LeansigResult::Ok
        }
        Err(_) => LeansigResult::SigningFailed,
    }
}

// ---------------------------------------------------------------------------
// Verification
// ---------------------------------------------------------------------------

/// Verify a signature against a public key, epoch, and message.
///
/// # Arguments
/// * `pk_data` - SSZ-serialized public key bytes.
/// * `pk_len` - Length of public key bytes.
/// * `epoch` - The epoch the signature was created at.
/// * `message` - Pointer to 32-byte message.
/// * `sig_data` - SSZ-serialized signature bytes.
/// * `sig_len` - Length of signature bytes.
///
/// # Returns
/// `LeansigResult::Ok` if verification succeeds, `LeansigResult::VerificationFailed` otherwise.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn leansig_verify(
    pk_data: *const u8,
    pk_len: usize,
    epoch: u32,
    message: *const u8,
    sig_data: *const u8,
    sig_len: usize,
) -> LeansigResult {
    if pk_data.is_null() || message.is_null() || sig_data.is_null() {
        return LeansigResult::NullPointer;
    }

    let pk_bytes = unsafe { slice::from_raw_parts(pk_data, pk_len) };
    let sig_bytes = unsafe { slice::from_raw_parts(sig_data, sig_len) };
    let msg: &[u8; 32] = unsafe { &*(message as *const [u8; 32]) };

    let pk = match PublicKey::from_bytes(pk_bytes) {
        Ok(pk) => pk,
        Err(_) => return LeansigResult::DeserializationFailed,
    };

    let sig = match Signature::from_bytes(sig_bytes) {
        Ok(sig) => sig,
        Err(_) => return LeansigResult::DeserializationFailed,
    };

    if SigScheme::verify(&pk, epoch, msg, &sig) {
        LeansigResult::Ok
    } else {
        LeansigResult::VerificationFailed
    }
}

// ---------------------------------------------------------------------------
// Verify using keypair (convenience for testing)
// ---------------------------------------------------------------------------

/// Verify a signature using the public key from a keypair handle.
///
/// Convenience wrapper that avoids serialization/deserialization of the public key.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn leansig_verify_with_keypair(
    keypair: *const LeansigKeypair,
    epoch: u32,
    message: *const u8,
    sig_data: *const u8,
    sig_len: usize,
) -> LeansigResult {
    if keypair.is_null() || message.is_null() || sig_data.is_null() {
        return LeansigResult::NullPointer;
    }

    let keypair = unsafe { &*keypair };
    let sig_bytes = unsafe { slice::from_raw_parts(sig_data, sig_len) };
    let msg: &[u8; 32] = unsafe { &*(message as *const [u8; 32]) };

    let sig = match Signature::from_bytes(sig_bytes) {
        Ok(sig) => sig,
        Err(_) => return LeansigResult::DeserializationFailed,
    };

    if SigScheme::verify(&keypair.pk, epoch, msg, &sig) {
        LeansigResult::Ok
    } else {
        LeansigResult::VerificationFailed
    }
}
