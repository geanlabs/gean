# Don't-touch list

Match each changed path against these lists before analysis. When in doubt,
treat unfamiliar code as soft-skip.

## Hard skip — drop the file

`scripts/collect-diff.sh` mirrors this list in its `SKIP` pattern; keep them in sync.

- `internal/types/*_encoding.go` and any file with a `// Code generated ... DO NOT EDIT.`
  header. A hand-written or hand-edited codec in the diff is still a finding
  (see `finding-categories.md`, `duplicates-existing-util`).
- `xmss/rust/` — Rust source, `Cargo.toml`, `Cargo.lock`; reviewed with
  `cargo fmt` and clippy (`make lint`), not this skill.
- `*.py` files.
- `internal/specfixtures/`, `internal/spectests/fixture.go` — mirror the leanSpec
  fixture JSON; "unused" fields are required for unmarshaling.
- `vendor/`, `third_party/`, `external/` (if present).

## Soft skip — report as `consensus-critical`, never apply without per-finding confirmation

- **Spec logic:** `internal/statetransition/`, `internal/forkchoice/`, and
  `internal/types/` (non-generated). Names and structure mirror leanSpec so a
  reviewer can read gean's `ProcessAttestations` beside `process_attestations`.
  Constants such as `SecondsPerSlot` are network-wide. `ForkChoice.Prune` must
  remap vote indices (`internal/forkchoice/votes.go`) together with pruning nodes.
- **Engine and duties:** `internal/node/` (single-writer engine, tick loop,
  gossip, import, proposal), `internal/pending/`, `internal/dutygate/`,
  `internal/role/`. Even inlining a helper can change scheduling under load.
- **Block and attestation processing:** `internal/blockprocessor/`,
  `internal/blockbuilder/`, `internal/attestation/`, `internal/attestationproof/`,
  `internal/aggregation/`, `internal/proving/`.
- **Consensus store:** `internal/store/` (write ordering and crash recovery).
- **P2P wire and gossip:** `internal/p2p/host.go` (gossipsub parameters shared
  with other clients), `topics.go`, `msgid.go`, and req/resp in `protocol.go`,
  `handlers.go`, `status.go`, `roots.go`, `range.go`, `encoding.go`,
  `response.go`. Wire formats must match other clients exactly.
- **XMSS Go binding:** `xmss/ffi.go` (CGo signatures and buffer ownership),
  `xmss/keys.go` (attestation/proposal key split guards one-time-signature
  reuse), `xmss/proof_pool.go`, `xmss/pubkey_cache.go`.
- **Spec test harness:** `internal/spectests/*_test.go`. These exist to fail
  loudly on divergence; cleanup risks weakening them.

## Always review

Everything else, with normal risk classification: `cmd/`, `internal/api/`,
`internal/syncer/`, `internal/checkpoint/`, `internal/storage/`,
`internal/genesis/`, `internal/logger/`, `internal/metrics/`, `internal/shadow/`,
other `internal/p2p/` files, and new tests outside `internal/spectests/`.
