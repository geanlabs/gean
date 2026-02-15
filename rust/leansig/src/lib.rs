use leansig::signature::SignatureScheme;
use leansig::signature::generalized_xmss::instantiations_poseidon_top_level::lifetime_2_to_the_32::hashing_optimized::SIGTopLevelTargetSumLifetime32Dim64Base8;
use rand::{SeedableRng,rngs::StdRng};
use std::ptr;

pub type LeanSignatureScheme = SIGTopLevelTargetSumLifetime32Dim64Base8;
pub type LeanPublicKey = <LeanSignatureScheme as SignatureScheme>::PublicKey;
pub type LeanSecretKey = <LeanSignatureScheme as SignatureScheme>::SecretKey;


pub struct SecretKey {
    pub inner: LeanSecretKey,
}

pub struct PublicKey {
    pub inner: LeanPublicKey,
}

pub struct Keypair {
    pub public_key: PublicKey,
    pub secret_key: SecretKey,
}

impl PublicKey {
    pub fn new(inner: LeanPublicKey) -> Self {
        Self { inner }
    }
}

impl SecretKey {
    pub fn new(inner: LeanSecretKey) -> Self {
        Self { inner }
    }
}

/// FFI: Exposed for Go (cgo) interoperability.
///
/// # Safety
/// - `ptr` must be a valid pointer to `len` bytes.
/// - Caller is responsible for freeing returned memory.

#[unsafe(no_mangle)]
pub unsafe extern "C" fn leansig_keypair_generate(
    seed: u64,
    activation_epoch: usize,
    num_active_epochs: usize,
) -> *mut Keypair {
    let mut rng = StdRng::seed_from_u64(seed);
    
    let (pk, sk) = <LeanSignatureScheme as SignatureScheme>::key_gen(&mut rng, activation_epoch, num_active_epochs);
    
    let public_key = PublicKey::new(pk);
    let secret_key = SecretKey::new(sk);
    
    let keypair = Box::new(Keypair {
        public_key,
        secret_key,
    });
    
    Box::into_raw(keypair)
}

// Get a pointer to the public key from a keypair
#[unsafe(no_mangle)]
pub unsafe extern "C" fn leansig_keypair_get_public_key(keypair: *const Keypair) -> *const PublicKey {
    if keypair.is_null() {
           return ptr::null();
    }
    
    unsafe {
         &(*keypair).public_key
    }
}

// Get a pointer to the secret key from a keypair
#[unsafe(no_mangle)]
pub unsafe extern "C" fn leansig_keypair_get_private_key(keypair: *const Keypair) -> *const SecretKey {
    if keypair.is_null() {
           return ptr::null();
    }
    
    unsafe {
         &(*keypair).secret_key
    }
}


/// FFI: Frees a heap-allocated XMSS `Keypair`.
///
/// # Safety
/// - `key_pair` must be a pointer previously returned by
///   `leansig_generate_keypair` (or any function that allocates a `Keypair` on the heap).
/// - Passing a null pointer is safe (function does nothing).
/// - After calling this function, the pointer must not be used again.
/// - Must only be called once per allocated `Keypair` to avoid double-free.
///
/// # Notes
/// - This function is intended for use from Go or other languages via FFI.
/// - It converts the raw pointer back into a `Box` and drops it, freeing the memory.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn leansig_keypair_free(key_pair: *mut Keypair) {
    if !key_pair.is_null() {
        unsafe {
            let _ = Box::from_raw(key_pair);
        }
    }
}