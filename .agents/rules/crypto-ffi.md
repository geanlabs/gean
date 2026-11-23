---
paths:
  - "internal/core/crypto/**"
  - "internal/lean/keys/**"
  - "internal/lean/keygen/**"
  - "internal/lean/proving/**"
---

# XMSS and the Rust FFI

- **Two builds, one API.** `ffi.go` calls the Rust library; `fake.go` (`-tags fakecrypto`) stands in
  for it. CI vets both builds, which catches a function a caller uses but one build lacks; keep
  exported functions in step by hand.
- **C memory is freed.** Every `ParsePublicKey` has a matching `FreePublicKey`, and every
  `ParseSignature` a `FreeSignature`, unless a cache owns the handle and frees it. A key pair is
  closed only after every proof using it has finished.
- **XMSS is expensive.** Real keygen takes tens of seconds per key and proving takes gigabytes of
  memory, so `make test` runs no real-crypto work beyond the scheme vector.
- **cgo pins an OS thread** per call: bound concurrent verifies with a fixed worker pool.
- **Rust glue** under `internal/core/crypto/xmss/rust` passes `cargo fmt` and `cargo clippy`
  (`make lint`). Rebuild the library with `make lean-ffi`, never `cargo build` alone: the target
  strips LLVM bitcode that Go's linker rejects.
