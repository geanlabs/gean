# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Gean is a Go consensus client for Lean Ethereum, implementing a simplified consensus layer with post-quantum signature support (XMSS via leanSig). It targets devnet-1 of the Lean Ethereum specification.

## Build & Development Commands

```bash
make build          # Build binary to bin/gean
make test           # Run all tests
make test-race      # Run tests with race detector
make lint           # go vet + staticcheck
make fmt            # go fmt
make run            # Build and run locally with config files
make run-devnet     # Spin up devnet node via lean-quickstart
make docker-build   # Build Docker image
```

Run a single test:
```bash
go test -run TestName ./path/to/package
```

The leansig CGo bindings require the Rust FFI library to be built first:
```bash
cd leansig-ffi && cargo build --release
```

## Toolchain Requirements

- **Go** 1.24.6+ (toolchain 1.24.12)
- **Rust** 1.87+ (for leansig-ffi)
- **staticcheck** (optional, for linting)

## Architecture

The node follows a reactor pattern with sequential core logic and concurrency only at boundaries (networking):

```
cmd/gean/main.go          Entry point, CLI flags, node init
        │
    node/                  Orchestrator: lifecycle, clock, sync, validator duties
        │
   ┌────┴──────────────┐
   │                    │
chain/                network/
├── forkchoice/       ├── host.go          libp2p host (QUIC, secp256k1)
│   ├── store.go      ├── gossipsub/       Block/attestation gossip
│   ├── block.go      │   ├── gossip.go    Topic pub/sub
│   ├── attestation.go│   ├── handler.go   Message handlers
│   ├── ghost.go      │   └── encoding.go  SSZ + Snappy encoding
│   ├── produce.go    └── reqresp/         /status, /blocks_by_root
│   └── time.go
└── statetransition/
    ├── transition.go  Main state transition function
    ├── genesis.go     Genesis state initialization
    └── proposer.go    Proposer duty helpers
```

**Other packages:**
- `types/` — SSZ-encoded core data structures (State, Block, Attestation) with `ssz-*` struct tags
- `storage/` — Storage interface with in-memory implementation (`storage/memory/`)
- `config/` — YAML config loaders for genesis, bootnodes, validator registry
- `leansig/` — Go CGo bindings for XMSS post-quantum signatures
- `leansig-ffi/` — Rust FFI library wrapping the leanSig crate
- `observability/` — Structured logging (slog) and Prometheus metrics

## Consensus Flow

**Block received:** validate parent state → state transition → store block+state → process attestations as votes → update head (LMD GHOST)

**Attestation received:** validate → store in fork choice → update head

Key entry points: `forkchoice.Store.ProcessBlock()`, `forkchoice.Store.ProcessAttestation()`, `statetransition.StateTransition()`

## Tests

Tests live in `test/unit/`, `test/interop/`, and `test/integration/`. Some packages also have colocated test files. Tests use standard `testing.T` with helper functions like `makeGenesisFC()` and `makeTestValidators()`.

## Code Conventions

- **Commit messages:** conventional commits — `feat(scope):`, `fix:`, `refactor:`, `test:`, `docs:`
- **Error wrapping:** `fmt.Errorf("context: %w", err)`
- **Logging:** structured slog via `logging.NewComponentLogger(logging.CompXxx)`
- **Receiver names:** single letter (`n *Node`, `s *Store`, `st *State`)
- **SSZ types:** generated encoding code lives in `*_encoding.go` files alongside type definitions

## Devnet-1 Metrics (leanMetrics@e077ac2)

Gean exposes Prometheus metrics on `--metrics-port` at `/metrics`. The 14 leanMetrics spec metrics are defined in `observability/metrics/metrics.go`:

**Fork-Choice:** `lean_head_slot`, `lean_fork_choice_block_processing_time_seconds`, `lean_attestations_valid_total`, `lean_attestations_invalid_total`, `lean_attestation_validation_time_seconds`

**State Transition:** `lean_latest_justified_slot`, `lean_latest_finalized_slot`, `lean_state_transition_time_seconds`, `lean_state_transition_slots_processed_total`, `lean_state_transition_slots_processing_time_seconds`, `lean_state_transition_block_processing_time_seconds`, `lean_state_transition_attestations_processed_total`, `lean_state_transition_attestations_processing_time_seconds`

**Validator:** `lean_validators_count`

## Design Philosophy

- Readable over clever — linear control flow, explicit naming
- Minimal dependencies — few external imports
- No premature abstraction — concrete types, interfaces only when duplication is real
- Flat and direct — shallow package hierarchy
