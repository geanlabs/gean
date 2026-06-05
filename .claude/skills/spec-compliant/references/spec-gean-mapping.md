# Spec-to-gean Mapping

Reference table for cross-referencing leanSpec changes against gean's Go codebase.

## Name Transformation Rules

| Spec (Python) | Go (gean) | Example |
|---|---|---|
| `snake_case` function | `PascalCase` exported function | `process_block` -> `ProcessBlock` |
| `PascalCase` class/container | `PascalCase` struct | `SignedBlock` -> `SignedBlock` |
| `UPPER_SNAKE_CASE` constant | `PascalCase` constant | `SECONDS_PER_SLOT` -> `SecondsPerSlot` |
| `List[X]` field | `[]X` or `[]*X` slice | `validators: List[Validator]` -> `Validators []*Validator` |
| `Optional[X]` field | `*X` pointer | `deposit_index: Optional[Uint64]` -> `DepositIndex *uint64` |
| `Bytes32` | `[32]byte` or `Root` | `parent_root: Bytes32` -> `ParentRoot [32]byte` |
| `Uint64` | `uint64` | `slot: Uint64` -> `Slot uint64` |

## Spec Area to gean Package Mapping

| Spec Area | Spec Path (under `src/lean_spec/`) | gean Package | Key Files |
|---|---|---|---|
| State container | `subspecs/containers/state/` | `internal/types/` | `state.go`, `state_encoding.go` |
| Block containers | `subspecs/containers/block/` | `internal/types/` | `block.go`, `block_encoding.go` |
| Attestation types | `subspecs/containers/attestation/` | `internal/types/` | `attestation.go`, `attestation_encoding.go` |
| Checkpoint | `subspecs/containers/checkpoint.py` | `internal/types/` | `checkpoint.go`, `checkpoint_encoding.go` |
| Validator | `subspecs/containers/validator.py` | `internal/types/` | `validator.go`, `validator_encoding.go` |
| Config | `subspecs/containers/config.py` | `internal/types/` | `config.go`, `config_encoding.go` |
| Constants | `subspecs/chain/config.py` | `internal/types/` | `constants.go` |
| Helpers | `subspecs/containers/` (various) | `internal/types/` | `helpers.go`, `bitlist.go` |
| SSZ encoding | `subspecs/ssz/` | `internal/types/` | `*_encoding.go` (generated via `make sszgen`) |
| State transition | `subspecs/containers/state/state.py` | `internal/statetransition/` | `transition.go`, `block.go`, `slots.go`, `attestations.go`, `justifiable.go` |
| Fork choice (LMD GHOST) | `subspecs/internal/forkchoice/` | `internal/forkchoice/` | `forkchoice.go`, `protoarray.go`, `votes.go`, `spec.go` |
| Consensus store | `subspecs/internal/forkchoice/store.py` | `internal/node/` | `consensus_store.go`, `store_block.go`, `store_payloads.go`, `store_tick.go` |
| On-block / on-attestation | `subspecs/internal/forkchoice/` | `internal/node/` | `store_block.go`, `store_payloads.go`, `block.go` |
| Aggregation | `subspecs/xmss/` + `subspecs/internal/forkchoice/` | `internal/node/` | `store_aggregate.go` |
| Genesis | `subspecs/internal/genesis/` | `internal/genesis/` | `config.go` |
| XMSS signatures | `subspecs/xmss/` | `xmss/` | Rust FFI (`xmss/rust/`) + Go bindings |
| P2P networking | `subspecs/networking/` | `internal/p2p/` | `host.go`, `gossip.go`, `topics.go`, `reqresp.go`, `encoding.go` |
| Storage | `subspecs/internal/storage/` | `internal/storage/` | `interface.go`, `memory.go`, `pebble.go` |

## Test Fixture Mapping

| Spec Test Category | Spec Path (under `tests/consensus/`) | gean Test File | Build Tag |
|---|---|---|---|
| State transition | `devnet/state_transition/` | `internal/spectests/stf_test.go` | `spectests` |
| Fork choice | `devnet/fc/` | `internal/spectests/forkchoice_test.go` | `spectests` |
| Signature verification | `devnet/verify_signatures/` | `internal/spectests/signatures_test.go` | `spectests` |
| SSZ containers | `devnet/ssz/` | `internal/spectests/` (if present) | `spectests` |
| Fixture parsing | (all) | `internal/spectests/fixture.go` | `spectests` |
