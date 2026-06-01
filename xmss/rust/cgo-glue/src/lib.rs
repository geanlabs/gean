//! Unified FFI shim for the gean Go side.
//!
//! The `extern crate` bindings below force Cargo/rustc to link
//! `hashsig-glue` and `multisig-glue` into this `staticlib` even though
//! no Rust-level item from them is referenced here — the only things we
//! care about are the `#[no_mangle] pub extern "C"` functions defined
//! in those crates, which rustc preserves in the final archive as long
//! as the rlib is part of the link set.
//!
//! The Cargo.toml above documents the duplicate-symbol failure mode this avoids.

extern crate hashsig_glue;
extern crate multisig_glue;
