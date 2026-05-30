# Repository Guidelines

## Project Structure & Module Organization

`gean` is a Go consensus client for Lean Consensus devnet-4 with Rust XMSS FFI.

- `node/`: consensus event loop, store, block/attestation processing, gossip ingress, aggregation, pruning, validator duties, and metrics.
- `statetransition/`: deterministic state transition logic.
- `forkchoice/`: fork-choice implementation.
- `p2p/`: libp2p gossip, req/resp, discovery, and peer handling.
- `xmss/`: Go bindings and Rust FFI glue for hash-based signatures and aggregation.
- `types/`: SSZ containers and generated codecs.
- `storage/`, `genesis/`, `checkpoint/`, `api/`: persistence, genesis setup, checkpoint utilities, and beacon API.
- `spectests/` and `specfixtures/`: leanSpec fixture-based tests.

## Build, Test, and Development Commands

- `make build`: build Rust FFI libraries and Go binaries.
- `make test`: run the standard Go unit suite.
- `make test-ffi`: run XMSS FFI tests.
- `make test-spec`: run leanSpec fixture tests.
- `make test-all`: run the full test suite; this is slower.
- `make fmt`: run `gofmt`.
- `make lint`: run Go and Rust lint checks.
- `go test ./node -run TestName -v -count=1`: run one focused node test.

## Coding Style & Naming Conventions

Use `gofmt` for Go and `cargo fmt` for Rust FFI code. Keep packages flat, direct, and readable. Prefer explicit data flow over clever abstractions, and keep changes scoped to the affected package.

## Consensus Store Concurrency Rule

`Engine.Run` owns `ConsensusStore` mutation. Worker goroutines may verify, aggregate, fetch, or compute from detached snapshots, but must return results over channels for the event loop to apply. Do not pass live store maps, mutable slices, or FFI-owned handles to workers.

## Testing Guidelines

Place Go tests beside code as `*_test.go`. For `node/` concurrency, run `go test -race ./node` when touching goroutines, snapshots, store buffers, or FFI handle ownership. Add regression tests for snapshot detachment whenever worker inputs include maps, slices, pointers, or handles.

## Commit & Pull Request Guidelines

Use short imperative commit subjects, preferably scoped, such as `fix(node): keep aggregation store mutations on event loop`. PRs should include a concise behavior summary, linked issue when relevant, and exact verification commands.
