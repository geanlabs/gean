use leansig::{signature::SignatureScheme, MESSAGE_LENGTH};
use rand::rngs::StdRng;
use rand::Rng;
use rand::SeedableRng;
use sha2::{Digest, Sha256};
use std::ffi::CStr;
use std::os::raw::c_char;
use std::ptr;
use std::slice;

// Devnet-4 XMSS parameters: Dim46 Base8 Aborting (matches leanMultisig V=46).
// Production config (default)
#[cfg(not(feature = "test-config"))]
mod config {
    pub use leansig::signature::generalized_xmss::instantiations_aborting::lifetime_2_to_the_32::{
        PubKeyAbortingTargetSumLifetime32Dim46Base8 as XmssPublicKey,
        SchemeAbortingTargetSumLifetime32Dim46Base8 as XmssScheme,
        SecretKeyAbortingTargetSumLifetime32Dim46Base8 as XmssSecretKey,
        SigAbortingTargetSumLifetime32Dim46Base8 as XmssSignature,
    };
}

// Test config
#[cfg(feature = "test-config")]
mod config {
    pub use leansig::signature::generalized_xmss::instantiations_aborting::lifetime_2_to_the_8::{
        PubKeyAbortingTargetSumLifetime8Dim46Base8 as XmssPublicKey,
        SchemeAbortingTargetSumLifetime8Dim46Base8 as XmssScheme,
        SecretKeyAbortingTargetSumLifetime8Dim46Base8 as XmssSecretKey,
        SigAbortingTargetSumLifetime8Dim46Base8 as XmssSignature,
    };
}

pub type HashSigScheme = config::XmssScheme;
pub type HashSigPrivateKey = config::XmssSecretKey;
pub type HashSigPublicKey = config::XmssPublicKey;
pub type HashSigSignature = config::XmssSignature;

#[repr(C)]
pub struct PrivateKey {
    inner: HashSigPrivateKey,
}

#[repr(C)]
pub struct PublicKey {
    pub inner: HashSigPublicKey,
}

#[repr(C)]
pub struct Signature {
    pub inner: HashSigSignature,
}

#[repr(C)]
pub struct KeyPair {
    pub public_key: PublicKey,
    pub private_key: PrivateKey,
}

impl PrivateKey {
    pub fn new(inner: HashSigPrivateKey) -> Self {
        Self { inner }
    }

    pub fn generate<R: Rng>(
        rng: &mut R,
        activation_epoch: usize,
        num_active_epochs: usize,
    ) -> (PublicKey, Self) {
        let (public_key, private_key) =
            <HashSigScheme as SignatureScheme>::key_gen(rng, activation_epoch, num_active_epochs);
        (PublicKey::new(public_key), Self::new(private_key))
    }

    pub fn sign(
        &self,
        message: &[u8; MESSAGE_LENGTH],
        epoch: u32,
    ) -> Result<Signature, leansig::signature::SigningError> {
        Ok(Signature::new(<HashSigScheme as SignatureScheme>::sign(
            &self.inner,
            epoch,
            message,
        )?))
    }
}

impl PublicKey {
    pub fn new(inner: HashSigPublicKey) -> Self {
        Self { inner }
    }
}

impl Signature {
    pub fn new(inner: HashSigSignature) -> Self {
        Self { inner }
    }

    pub fn verify(
        &self,
        message: &[u8; MESSAGE_LENGTH],
        public_key: &PublicKey,
        epoch: u32,
    ) -> bool {
        <HashSigScheme as SignatureScheme>::verify(&public_key.inner, epoch, message, &self.inner)
    }
}

// --- FFI Functions ---

#[no_mangle]
pub unsafe extern "C" fn hashsig_keypair_generate(
    seed_phrase: *const c_char,
    activation_epoch: usize,
    num_active_epochs: usize,
) -> *mut KeyPair {
    let seed_phrase = unsafe { CStr::from_ptr(seed_phrase).to_string_lossy().into_owned() };
    let mut hasher = Sha256::new();
    hasher.update(seed_phrase.as_bytes());
    let seed = hasher.finalize().into();

    let (public_key, private_key) = PrivateKey::generate(
        &mut StdRng::from_seed(seed),
        activation_epoch,
        num_active_epochs,
    );

    Box::into_raw(Box::new(KeyPair {
        public_key,
        private_key,
    }))
}

#[no_mangle]
pub unsafe extern "C" fn hashsig_keypair_from_ssz(
    private_key_ptr: *const u8,
    private_key_len: usize,
    public_key_ptr: *const u8,
    public_key_len: usize,
) -> *mut KeyPair {
    if private_key_ptr.is_null() || public_key_ptr.is_null() {
        return ptr::null_mut();
    }
    unsafe {
        let sk_slice = slice::from_raw_parts(private_key_ptr, private_key_len);
        let pk_slice = slice::from_raw_parts(public_key_ptr, public_key_len);

        let private_key: HashSigPrivateKey = match HashSigPrivateKey::from_ssz_bytes(sk_slice) {
            Ok(key) => key,
            Err(_) => return ptr::null_mut(),
        };
        let public_key: HashSigPublicKey = match HashSigPublicKey::from_ssz_bytes(pk_slice) {
            Ok(key) => key,
            Err(_) => return ptr::null_mut(),
        };

        Box::into_raw(Box::new(KeyPair {
            public_key: PublicKey::new(public_key),
            private_key: PrivateKey::new(private_key),
        }))
    }
}

#[no_mangle]
pub unsafe extern "C" fn hashsig_keypair_free(keypair: *mut KeyPair) {
    if !keypair.is_null() {
        unsafe {
            let _ = Box::from_raw(keypair);
        }
    }
}

#[no_mangle]
pub unsafe extern "C" fn hashsig_keypair_get_public_key(
    keypair: *const KeyPair,
) -> *const PublicKey {
    if keypair.is_null() {
        return ptr::null();
    }
    &(*keypair).public_key
}

#[no_mangle]
pub unsafe extern "C" fn hashsig_keypair_get_private_key(
    keypair: *const KeyPair,
) -> *const PrivateKey {
    if keypair.is_null() {
        return ptr::null();
    }
    &(*keypair).private_key
}

#[no_mangle]
pub unsafe extern "C" fn hashsig_public_key_from_ssz(
    public_key_ptr: *const u8,
    public_key_len: usize,
) -> *mut PublicKey {
    if public_key_ptr.is_null() {
        return ptr::null_mut();
    }
    unsafe {
        let pk_slice = slice::from_raw_parts(public_key_ptr, public_key_len);
        let public_key: HashSigPublicKey = match HashSigPublicKey::from_ssz_bytes(pk_slice) {
            Ok(key) => key,
            Err(_) => return ptr::null_mut(),
        };
        Box::into_raw(Box::new(PublicKey::new(public_key)))
    }
}

#[no_mangle]
pub unsafe extern "C" fn hashsig_public_key_free(public_key: *mut PublicKey) {
    if !public_key.is_null() {
        unsafe {
            let _ = Box::from_raw(public_key);
        }
    }
}

#[no_mangle]
pub unsafe extern "C" fn hashsig_sign(
    private_key: *const PrivateKey,
    message_ptr: *const u8,
    epoch: u32,
) -> *mut Signature {
    if private_key.is_null() || message_ptr.is_null() {
        return ptr::null_mut();
    }
    unsafe {
        let private_key_ref = &*private_key;
        let message_slice = slice::from_raw_parts(message_ptr, MESSAGE_LENGTH);
        let message_array: &[u8; MESSAGE_LENGTH] = match message_slice.try_into() {
            Ok(arr) => arr,
            Err(_) => return ptr::null_mut(),
        };
        match private_key_ref.sign(message_array, epoch) {
            Ok(sig) => Box::into_raw(Box::new(sig)),
            Err(_) => ptr::null_mut(),
        }
    }
}

#[no_mangle]
pub unsafe extern "C" fn hashsig_signature_free(signature: *mut Signature) {
    if !signature.is_null() {
        unsafe {
            let _ = Box::from_raw(signature);
        }
    }
}

#[no_mangle]
pub unsafe extern "C" fn hashsig_signature_from_ssz(
    signature_ptr: *const u8,
    signature_len: usize,
) -> *mut Signature {
    if signature_ptr.is_null() || signature_len == 0 {
        return ptr::null_mut();
    }
    unsafe {
        let sig_slice = slice::from_raw_parts(signature_ptr, signature_len);
        let signature: HashSigSignature = match HashSigSignature::from_ssz_bytes(sig_slice) {
            Ok(sig) => sig,
            Err(_) => return ptr::null_mut(),
        };
        Box::into_raw(Box::new(Signature { inner: signature }))
    }
}

#[no_mangle]
pub unsafe extern "C" fn hashsig_verify(
    public_key: *const PublicKey,
    message_ptr: *const u8,
    epoch: u32,
    signature: *const Signature,
) -> i32 {
    if public_key.is_null() || message_ptr.is_null() || signature.is_null() {
        return -1;
    }
    unsafe {
        let public_key_ref = &*public_key;
        let signature_ref = &*signature;
        let message_slice = slice::from_raw_parts(message_ptr, MESSAGE_LENGTH);
        let message_array: &[u8; MESSAGE_LENGTH] = match message_slice.try_into() {
            Ok(arr) => arr,
            Err(_) => return -1,
        };
        if signature_ref.verify(message_array, public_key_ref, epoch) {
            1
        } else {
            0
        }
    }
}

#[no_mangle]
pub extern "C" fn hashsig_message_length() -> usize {
    MESSAGE_LENGTH
}

use ssz::{Decode, Encode};

#[no_mangle]
pub unsafe extern "C" fn hashsig_signature_to_bytes(
    signature: *const Signature,
    buffer: *mut u8,
    buffer_len: usize,
) -> usize {
    if signature.is_null() || buffer.is_null() {
        return 0;
    }
    unsafe {
        let sig_ref = &*signature;
        let ssz_bytes = sig_ref.inner.as_ssz_bytes();
        if ssz_bytes.len() > buffer_len {
            return 0;
        }
        let output_slice = slice::from_raw_parts_mut(buffer, buffer_len);
        output_slice[..ssz_bytes.len()].copy_from_slice(&ssz_bytes);
        ssz_bytes.len()
    }
}

#[no_mangle]
pub unsafe extern "C" fn hashsig_public_key_to_bytes(
    public_key: *const PublicKey,
    buffer: *mut u8,
    buffer_len: usize,
) -> usize {
    if public_key.is_null() || buffer.is_null() {
        return 0;
    }
    unsafe {
        let public_key_ref = &*public_key;
        let ssz_bytes = public_key_ref.inner.as_ssz_bytes();
        if ssz_bytes.len() > buffer_len {
            return 0;
        }
        let output_slice = slice::from_raw_parts_mut(buffer, buffer_len);
        output_slice[..ssz_bytes.len()].copy_from_slice(&ssz_bytes);
        ssz_bytes.len()
    }
}

#[no_mangle]
pub unsafe extern "C" fn hashsig_private_key_to_bytes(
    private_key: *const PrivateKey,
    buffer: *mut u8,
    buffer_len: usize,
) -> usize {
    if private_key.is_null() || buffer.is_null() {
        return 0;
    }
    unsafe {
        let private_key_ref = &*private_key;
        let ssz_bytes = private_key_ref.inner.as_ssz_bytes();
        if ssz_bytes.len() > buffer_len {
            return 0;
        }
        let output_slice = slice::from_raw_parts_mut(buffer, buffer_len);
        output_slice[..ssz_bytes.len()].copy_from_slice(&ssz_bytes);
        ssz_bytes.len()
    }
}

#[no_mangle]
pub unsafe extern "C" fn hashsig_verify_ssz(
    pubkey_bytes: *const u8,
    pubkey_len: usize,
    message: *const u8,
    epoch: u32,
    signature_bytes: *const u8,
    signature_len: usize,
) -> i32 {
    if pubkey_bytes.is_null() || message.is_null() || signature_bytes.is_null() {
        return -1;
    }
    unsafe {
        let pk_data = slice::from_raw_parts(pubkey_bytes, pubkey_len);
        let sig_data = slice::from_raw_parts(signature_bytes, signature_len);
        let msg_data = slice::from_raw_parts(message, MESSAGE_LENGTH);
        let message_array: &[u8; MESSAGE_LENGTH] = match msg_data.try_into() {
            Ok(arr) => arr,
            Err(_) => return -1,
        };
        let pk: HashSigPublicKey = match HashSigPublicKey::from_ssz_bytes(pk_data) {
            Ok(pk) => pk,
            Err(_) => return -1,
        };
        let sig: HashSigSignature = match HashSigSignature::from_ssz_bytes(sig_data) {
            Ok(sig) => sig,
            Err(_) => return -1,
        };
        if <HashSigScheme as SignatureScheme>::verify(&pk, epoch, message_array, &sig) {
            1
        } else {
            0
        }
    }
}
