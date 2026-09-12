# leanVM system-info patch

Copied from `leanEthereum/leanVM` revision
`a5909d18647de6aed38640c098d9177fab2bf36a`, `crates/backend/system-info`.
The upstream license is included.

The only behavior change is a startup setter for the existing cached thread
count. `configure_num_threads(0)` retains CPU detection; a positive value must
not exceed available parallelism. A different value after resolution fails.
Cargo patches this crate for all leanVM consumers, so the pool and its callers
use the same count. Cache and memory reporting remain upstream code.

Remove this patch when the pinned upstream provides a thread-count setting.
