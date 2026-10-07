# Don't-touch list

Match each changed path against these lists before analysis. When in doubt,
treat unfamiliar code as soft-skip.

## Hard skip — drop the file

`scripts/collect-diff.sh` mirrors this list in its `SKIP` pattern; keep them in sync.

- `types/*_encoding.go` and any file with a `// Code generated ... DO NOT EDIT.`
  header. A hand-written or hand-edited codec in the diff is still a finding
  (see `finding-categories.md`, `duplicates-existing-util`).
- `crypto/xmss/rust/` — Rust source, `Cargo.toml`, `Cargo.lock`; reviewed with
  `cargo fmt` and clippy (`make lint`), not this skill.
- `*.py` files.
- `internal/specfixtures/`, `internal/spectests/fixture.go` — mirror the leanSpec
  fixture JSON; "unused" fields are required for unmarshaling.
- `vendor/`, `third_party/`, `external/` (if present).

## Soft skip — report as `consensus-critical`, never apply without per-finding confirmation

- **Spec logic:** `consensus/statetransition/`, `consensus/forkchoice/`, and
  `types/` (non-generated). Names and structure mirror leanSpec so a
  reviewer can read gean's `ProcessAttestations` beside `process_attestations`.
  Constants such as `SecondsPerSlot` are network-wide. `ForkChoice.Prune` must
  remap vote indices (`consensus/forkchoice/votes.go`) together with pruning nodes.
- **Engine and duties:** `node/` (single-writer engine, tick loop,
  gossip, import, proposal), `pending/`, `dutygate/`,
  `role/`. Even inlining a helper can change scheduling under load.
- **Block and attestation processing:** `consensus/blockprocessor/`,
  `consensus/blockbuilder/`, `consensus/attestation/`, `consensus/attestationproof/`,
  `consensus/aggregation/`, `proving/`.
- **Consensus store:** `storage/store/` (write ordering and crash recovery).
- **P2P wire and gossip:** `net/p2p/host.go` (gossipsub parameters shared
  with other clients), `topics.go`, `msgid.go`, and req/resp in `protocol.go`,
  `handlers.go`, `status.go`, `roots.go`, `range.go`, `encoding.go`,
  `response.go`. Wire formats must match other clients exactly.
- **XMSS Go binding:** `crypto/xmss/ffi.go` (CGo signatures and buffer ownership),
  `crypto/xmss/keys.go` (attestation/proposal key split guards one-time-signature
  reuse), `crypto/xmss/proof_pool.go`, `crypto/xmss/pubkey_cache.go`.
- **Spec test harness:** `internal/spectests/*_test.go`. These exist to fail
  loudly on divergence; cleanup risks weakening them.

## Always review

Everything else, with normal risk classification: `cmd/`, `api/`,
`net/syncer/`, `net/checkpoint/`, `storage/db/`,
`consensus/genesis/`, `logger/`, `metrics/`, `shadow/`,
other `net/p2p/` files, and new tests outside `internal/spectests/`.
