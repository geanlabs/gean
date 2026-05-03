# gean

A Go consensus client for [Lean Ethereum](https://github.com/leanEthereum/leanSpec), built around the idea that protocol simplicity is a security property.

## Philosophy

A consensus client should be something a developer can read, understand, and verify without needing to trust a small class of experts. If you can't inspect it end-to-end, it's not fully yours.

## Design approach

- **Readable over clever.** Code is written so that someone unfamiliar with the codebase can follow it. Naming is explicit. Control flow is linear where possible.
- **Minimal dependencies.** Fewer imports means fewer things that can break, fewer things to audit, and fewer things to understand.
- **No premature abstraction.** Interfaces and generics are introduced when the duplication is real, not when it's hypothetical. Concrete types until proven otherwise.
- **Flat and direct.** Avoid deep package hierarchies and layers of indirection. A function should do what its name says, and you should be able to find it quickly.
- **Concurrency only where necessary.** Go makes concurrency easy to write and hard to reason about. We use it at the boundaries (networking, event loops) and keep the core logic sequential and deterministic.

## Current status

gean targets **Lean Consensus devnet-4**.

| Network | Status | Spec pin |
|---------|--------|----------|
| devnet-4 | Active | `leanSpec@e845240` |
| devnet-3 | Superseded | `leanSpec@be85318` |

## Prerequisites

- **Go** 1.25+
- **Rust** 1.90.0 (for the XMSS FFI libraries under `xmss/rust/`)
- **uv** ([astral.sh/uv](https://docs.astral.sh/uv/)) — needed to generate leanSpec test fixtures
- **Docker** (for multi-client devnet)

## Build

```sh
# Build Rust FFI libraries + Go binary
make build

# Build Docker image
make docker-build
```

## Local Testnet (Self-Interop)

Run a 3-node local testnet with 5 validators:

```sh
# First-time setup: generate XMSS keys + config
make run-setup

# Terminal 1: node0 (aggregator)
make run

# Terminal 2: node1
make run-node1

# Terminal 3: node2
make run-node2
```

### Ports

| Node  | P2P (QUIC/UDP) | API  | Metrics |
|-------|----------------|------|---------|
| node0 | 9000           | 5052 | 8080    |
| node1 | 9001           | 5053 | 8081    |
| node2 | 9002           | 5054 | 8082    |

### Checkpoint Sync

Restart a node using checkpoint sync from another running node:

```sh
rm -rf data/node1
bin/gean \
  --custom-network-config-dir testnet \
  --node-key testnet/node1.key \
  --node-id node1 \
  --data-dir data/node1 \
  --gossipsub-port 9001 \
  --api-port 5053 \
  --metrics-port 8081 \
  --checkpoint-sync-url http://127.0.0.1:5052/lean/v0/states/finalized
```

## Multi-Client Devnet

gean is part of the [lean-quickstart](https://github.com/blockblaz/lean-quickstart) multi-client devnet tooling.

```sh
# Build Docker image and start devnet
make run-devnet
```

### Multi-client testing skills

For repeatable testing against zeam, ream, lantern, and ethlambda, gean ships
three Claude Code skills under [`.claude/skills/`](.claude/skills/README.md).
The most useful entry points are exposed as `make` targets:

```sh
# Build current branch + run a 5-client devnet test (most common)
make devnet-test

# Same as above plus a sync recovery test (pause peers, then resume)
make devnet-test-sync

# Inspect what's running right now
make devnet-status

# Stop the devnet and restore configs
make devnet-cleanup

# Run a one-off devnet for 120s and dump every client's logs to the repo root
make devnet-run

# Analyze .log files in the current directory (gean + peer clients)
make devnet-analyze
```

`make devnet-test` automatically watches for two regressions: oversized blocks
(`MessageTooLarge` / `exceeds max`) and excessive attestations per block
(> 30). If either fires, the test exits with `✗ FAILED`.

See [`.claude/skills/README.md`](.claude/skills/README.md) for the full
overview of each skill (`devnet-log-review`, `devnet-runner`, `test-pr-devnet`).

## API

gean exposes a lightweight HTTP API on two separate ports:

**API server** (default `:5052`):

| Endpoint | Description |
|----------|-------------|
| `GET /lean/v0/health` | Health check |
| `GET /lean/v0/states/finalized` | Latest finalized state (SSZ) |
| `GET /lean/v0/checkpoints/justified` | Justified checkpoint (JSON) |
| `GET /lean/v0/fork_choice` | Fork choice tree (JSON) |

**Metrics server** (default `:5054`):

| Endpoint | Description |
|----------|-------------|
| `GET /metrics` | Prometheus metrics |

## Tests

```sh
# Unit tests (no FFI required)
make test

# FFI/crypto tests (requires make ffi)
make ffi-test

# leanSpec fixture tests (requires fixtures)
make leanSpec/fixtures
make test-spec
```

### Spec Test Coverage

| Suite | Fixtures | Description |
|-------|----------|-------------|
| State Transition | 14 | Block processing, genesis validation |
| Fork Choice | 27 | Head selection, reorgs, tiebreakers, aggregation, finalization |
| Signature Verification | 8 | Proposer signatures, attestation aggregation, invalid cases |
| **Total** | **49** | **All passing** |

### leanSpec Fixtures

Consensus conformance tests use fixtures generated from the pinned leanSpec commit:

```sh
# Generate/update fixtures
make leanSpec/fixtures

# Verify pin
git -C leanSpec rev-parse HEAD
```

Fixtures are generated under `leanSpec/fixtures/`. The `leanSpec/` directory is local and gitignored.

## CLI Flags

```
--custom-network-config-dir   Config directory (required)
--gossipsub-port              P2P listen port, QUIC/UDP (default: 9000)
--http-address                Bind address for API + metrics (default: 127.0.0.1)
--api-port                    API server port (default: 5052)
--metrics-port                Metrics server port (default: 5054)
--node-key                    Path to hex-encoded secp256k1 private key (required)
--node-id                     Node identifier, e.g. gean_0 (required)
--checkpoint-sync-url         URL for checkpoint sync (optional)
--is-aggregator               Enable attestation aggregation
--attestation-committee-count Number of attestation subnets (default: 1)
--data-dir                    Pebble database directory (default: ./data)
```

## Architecture

- **Single-writer node** goroutine with select on tick + gossip channels
- **3SF-mini fork choice** with LMD GHOST head selection (proto-array)
- **XMSS post-quantum signatures** via Rust FFI (leansig/leanMultisig)
- **Pebble** (CockroachDB's Go-native LSM) for persistent storage
- **GossipSub v1.1** with anonymous message signing
- **Req-resp** protocols: Status + BlocksByRoot with snappy framed encoding
- **5-interval slot structure** (800ms each, 4s total): propose, attest, aggregate, safe-target, accept
- **43 Prometheus metrics** matching the [leanMetrics](https://github.com/leanEthereum/leanMetrics) standard

## Acknowledgements

- [Lean Ethereum](https://github.com/leanEthereum)
- [ethlambda](https://github.com/lambdaclass/ethlambda)
- [zeam](https://github.com/blockblaz/zeam)

## License

MIT
