# Don't-touch list

Match each changed path against these lists before analysis. When in doubt,
treat unfamiliar code as soft-skip.

## Hard skip — drop the file

`scripts/collect-diff.sh` mirrors this list in its `SKIP` pattern; keep them in sync.

- `lean/consensus/types/*_encoding.go` and any file with a `// Code generated ... DO NOT EDIT.`
  header. A hand-written or hand-edited codec in the diff is still a finding
  (see `finding-categories.md`, `duplicates-existing-util`).
- `lean/crypto/xmss/rust/` — Rust source, `Cargo.toml`, `Cargo.lock`; reviewed with
  `cargo fmt` and clippy (`make lint`), not this skill.
- `*.py` files.
- `lean/internal/specfixtures/`, `lean/spectests/fixture.go` — mirror the leanSpec
  fixture JSON; "unused" fields are required for unmarshaling.
- `vendor/`, `third_party/`, `external/` (if present).

## Soft skip — report as `consensus-critical`, never apply without per-finding confirmation

- **Spec logic:** `lean/consensus/statetransition/`, `lean/consensus/forkchoice/`, and
  `lean/consensus/types/` (non-generated). Names and structure mirror leanSpec so a
  reviewer can read gean's `ProcessAttestations` beside `process_attestations`.
  Constants such as `SecondsPerSlot` are network-wide. `ForkChoice.Prune` must
  remap vote indices (`lean/consensus/forkchoice/votes.go`) together with pruning nodes.
- **Engine and duties:** `lean/internal/chain/` (single-writer engine, tick loop,
  gossip, import, proposal), `lean/internal/chain/pending/`, `lean/internal/validator/dutygate/`,
  `lean/internal/aggregation/role/`. Even inlining a helper can change scheduling under load.
- **Block and attestation processing:** `lean/internal/chain/blockprocessor/`,
  `lean/internal/blockbuilder/`, `lean/internal/chain/attestation/`, `lean/internal/blockbuilder/attestationproof/`,
  `lean/internal/aggregation/`, `lean/internal/prover/`.
- **Consensus store:** `lean/internal/store/` (write ordering and crash recovery).
- **P2P wire and gossip:** `lean/internal/network/host.go` (gossipsub parameters shared
  with other clients), `topics.go`, `msgid.go`, and req/resp in `protocol.go`,
  `handlers.go`, `status.go`, `roots.go`, `range.go`, `encoding.go`,
  `response.go`. Wire formats must match other clients exactly.
- **XMSS Go binding:** `lean/crypto/xmss/ffi.go` (CGo signatures and buffer ownership),
  `lean/crypto/xmss/keys.go` (attestation/proposal key split guards one-time-signature
  reuse), `lean/crypto/xmss/proof_pool.go`, `lean/crypto/xmss/pubkey_cache.go`.
- **Spec test harness:** `lean/spectests/*_test.go`. These exist to fail
  loudly on divergence; cleanup risks weakening them.

## Always review

Everything else, with normal risk classification: `cmd/`, `lean/internal/api/`,
`lean/internal/network/syncer/`, `lean/internal/network/checkpoint/`, `internal/storage/`,
`lean/consensus/genesis/`, `internal/logger/`, `internal/metrics/`, `lean/internal/prover/shadow/`,
other `lean/internal/network/` files, and new tests outside `lean/spectests/`.
