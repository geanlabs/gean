//! The subset of the `ethereum_hashing` API that `tree_hash` and `ssz_types`
//! use, backed by the `sha2` crate or, for OpenVM guests, `openvm-sha2`.

use sha2::Digest;
use std::sync::LazyLock;

#[cfg(feature = "openvm")]
use openvm_sha2::Sha256;
#[cfg(not(feature = "openvm"))]
use sha2::Sha256;

pub const HASH_LEN: usize = 32;

pub fn hash(input: &[u8]) -> Vec<u8> {
    hash_fixed(input).to_vec()
}

pub fn hash_fixed(input: &[u8]) -> [u8; HASH_LEN] {
    Sha256::digest(input).into()
}

pub fn hash32_concat(h1: &[u8], h2: &[u8]) -> [u8; HASH_LEN] {
    let mut ctx = Context::new();
    ctx.update(h1);
    ctx.update(h2);
    ctx.finalize()
}

pub trait Sha256Context {
    fn new() -> Self;
    fn update(&mut self, bytes: &[u8]);
    fn finalize(self) -> [u8; HASH_LEN];
}

pub struct Context(Sha256);

impl Sha256Context for Context {
    fn new() -> Self {
        Context(Sha256::new())
    }

    fn update(&mut self, bytes: &[u8]) {
        self.0.update(bytes);
    }

    fn finalize(self) -> [u8; HASH_LEN] {
        self.0.finalize().into()
    }
}

/// The largest index of `ZERO_HASHES`.
pub const ZERO_HASHES_MAX_INDEX: usize = 48;

/// `ZERO_HASHES[i]` is the root of a Merkle tree with 2^i zero leaves.
pub static ZERO_HASHES: LazyLock<Vec<[u8; HASH_LEN]>> = LazyLock::new(|| {
    let mut hashes = vec![[0; HASH_LEN]; ZERO_HASHES_MAX_INDEX + 1];
    for i in 0..ZERO_HASHES_MAX_INDEX {
        hashes[i + 1] = hash32_concat(&hashes[i], &hashes[i]);
    }
    hashes
});
