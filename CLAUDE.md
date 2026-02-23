# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Gean is a Go consensus client for Lean Ethereum — a complete redesign of Ethereum's consensus layer focused on security, decentralization, and finality in seconds. It implements LMD GHOST fork-choice, state transitions, and uses XMSS post-quantum signatures via Rust FFI.

## Prerequisites

- **Go** 1.24.6+
- **Rust** 1.87+ (for XMSS FFI library in `xmss/leansig-ffi/`)
- **uv** (astral.sh/uv) — only needed to generate leanSpec test fixtures

## Build & Test Commands

```sh
make build          # Build FFI library + gean binary + keygen → bin/
make spec-test      # Consensus spectests (clones leanSpec, generates fixtures, skips sig verify)
make unit-test      # All Go unit tests with signature verification
make test-race      # Race condition detection
make lint           # go vet + staticcheck
make fmt            # go fmt ./...
```

Run a single test or package:
```sh
go test -count=1 ./chain/forkchoice/...
go test -count=1 -run TestName ./package/...
```

Spectests use the build tag `skip_sig_verify` to bypass XMSS signature verification for speed. The FFI library (`make ffi`) must be built before running any tests.

## Architecture

The node starts at `cmd/gean/main.go`, which loads genesis config, bootnodes, and validator assignments, then delegates to `node/lifecycle.go` for initialization.

**Consensus (`chain/`)**
- `forkchoice/` — LMD GHOST fork-choice: block processing, attestation weighting, canonical head selection
- `statetransition/` — State machine that processes blocks and attestations, advances epochs

**Node orchestration (`node/`)**
- `lifecycle.go` — Initialization: genesis state, P2P host, gossipsub, discovery, validator keys, metrics
- `ticker.go` — Main event loop: slot ticker fires 4 intervals per slot (1.5s each, 6s slots). Advances fork-choice time, syncs peers, dispatches validator duties
- `validator.go` — Validator duties by interval: propose (0), attest (1), aggregate (2)
- `handler.go` — Gossip subscription and request/response handler registration
- `sync.go` — Peer sync protocol
- `clock.go` — Slot and interval timing relative to genesis

**Networking (`network/`)**
- `host.go` — libp2p host with QUIC transport
- `gossipsub/` — Pub/sub for blocks and attestations; SSZ-encoded messages
- `p2p/` — Peer discovery via discv5, ENR parsing
- `reqresp/` — Request/response protocols (status, block sync) using Snappy framing

**Cryptography (`xmss/`)**
- `leansig/` — CGo bindings for XMSS post-quantum signatures
- `leansig-ffi/` — Rust FFI library wrapping leanSig
- Devnet-1 instantiation: `SIGTargetSumLifetime18W1NoOff`

**Data types (`types/`)** — Consensus state, blocks, attestations, checkpoints. All types implement SSZ encoding.

**Storage (`storage/`)** — Interface with in-memory implementation (`memory/`). Thread-safe block and state storage.

**Config (`config/`)** — Genesis state initialization, validator registry loading, bootnode configuration. Loaded from `config.yaml`, `validators.yaml`, `nodes.yaml`.

**Observability (`observability/`)** — Structured logging with component tags and color output. Prometheus metrics for fork-choice, attestations, state transitions, validators, and network.

## Design Principles

- Readable over clever — explicit naming, linear control flow
- Minimal dependencies — fewer imports to audit
- No premature abstraction — concrete types until duplication is real
- Flat and direct — avoid deep package hierarchies
- Concurrency only at boundaries (networking, event loops); core consensus logic is sequential and deterministic
