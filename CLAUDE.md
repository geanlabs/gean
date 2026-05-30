# CLAUDE.md

This file gives Claude Code guidance for working in `gean`, the Go Lean Consensus client.

## Project Map

- `node/`: core runtime. `Engine.Run` drives the event loop; `consensus_store.go` and `store_*.go` hold store behavior by concern; aggregation, validator duties, pruning, block production, gossip ingress, and metrics live here.
- `statetransition/`: pure state transition logic.
- `forkchoice/`: fork-choice implementation.
- `p2p/`: libp2p gossip, req/resp, discovery, and peer handling.
- `xmss/`: XMSS and aggregation FFI boundary. Rust glue lives under `xmss/rust/`.
- `types/`: SSZ types and generated codecs.
- `storage/`, `genesis/`, `checkpoint/`, `api/`: persistence, bootstrapping, checkpoint support, and beacon API.
- `spectests/` and `specfixtures/`: leanSpec fixture tests.

## Commands

```sh
make build        # build Rust FFI libs and Go binaries
make test         # standard unit tests
make test-ffi     # XMSS FFI tests
make test-spec    # leanSpec fixture tests
make test-all     # full suite, slow
make fmt          # gofmt
make lint         # Go and Rust lint checks
make sszgen       # regenerate SSZ codecs from struct tags
```

Run focused tests with `go test ./node -run TestName -v -count=1`.

## Architecture Rules

Keep the code flat, explicit, and local to the affected subsystem. Prefer readable control flow over abstractions that hide consensus behavior.

`Engine.Run` is the single writer for `ConsensusStore` and its owned buffers. Background goroutines may do expensive verification, fetching, or aggregation only from detached snapshots, then send results back over channels for the event loop to apply. Never share live store maps, mutable slices, or FFI-owned handles with worker goroutines.

FFI handle ownership must be explicit. If a worker needs signature handles, prefer giving it raw bytes and letting it parse/free its own temporary handles.

## Testing Expectations

Run targeted tests first, then the relevant package suite. When changing `node/` goroutines, snapshots, store buffers, or FFI handle ownership, run:

```sh
go test ./node
go test -race ./node
```

Add regression tests for any snapshot that crosses a goroutine boundary.
