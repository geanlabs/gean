# Geany

Go implementation of the [Lean Ethereum Consensus Client](https://github.com/leanEthereum/leanSpec) targeting leanSpec devnet-3.

## Requirements

- Go 1.25+
- Rust 1.90.0 (for XMSS FFI libraries)
- Docker (for multi-client devnet)

## Build

```sh
# Build Rust FFI libraries + Go binaries
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

### Checkpoint Sync (Self-Interop)

Restart a node using checkpoint sync from another running node:

```sh
# Restart node1 using node0's finalized state
rm -rf data/node1
bin/geany \
  --custom-network-config-dir testnet \
  --node-key testnet/node1.key \
  --node-id node1 \
  --data-dir data/node1 \
  --gossipsub-port 9001 \
  --api-port 5053 \
  --metrics-port 8081 \
  --checkpoint-sync-url http://127.0.0.1:5052/lean/v0/states/finalized
```

## Multi-Client Devnet (lean-quickstart)

Run geany alongside zeam, ethlambda, ream, and lantern:

```sh
# Build Docker image and start devnet
make run-devnet
```

### Checkpoint Sync (Multi-Client)

Restart geany in the devnet using checkpoint sync from another client:

```sh
cd lean-quickstart
NETWORK_DIR=local-devnet ./spin-node.sh --restart-client geany_0 \
  --checkpoint-sync-url http://127.0.0.1:5060/lean/v0/states/finalized
```

## API Endpoints

| Endpoint | Description |
|----------|-------------|
| `GET /lean/v0/health` | Health check |
| `GET /lean/v0/states/finalized` | Latest finalized state (SSZ) |
| `GET /lean/v0/checkpoints/justified` | Justified checkpoint (JSON) |

## Tests

```sh
# Unit tests
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

## CLI Flags

```
--custom-network-config-dir   Config directory (required)
--gossipsub-port              P2P listen port, QUIC/UDP (default: 9000)
--http-address                Bind address for API + metrics (default: 127.0.0.1)
--api-port                    API server port (default: 5052)
--metrics-port                Metrics server port (default: 5054)
--node-key                    Path to hex-encoded secp256k1 private key (required)
--node-id                     Node identifier, e.g. geany_0 (required)
--checkpoint-sync-url         URL for checkpoint sync (optional)
--is-aggregator               Enable attestation aggregation
--attestation-committee-count Number of attestation subnets (default: 1)
--data-dir                    Pebble database directory (default: ./data)
```

## Architecture

Geany follows ethlambda's architecture:

- **Single-writer engine** goroutine with select on tick + gossip channels
- **3SF-mini fork choice** with LMD GHOST head selection (proto-array)
- **XMSS post-quantum signatures** via Rust FFI (leansig/leanMultisig)
- **Pebble** (CockroachDB's Go-native LSM) for persistent storage
- **GossipSub v1.1** with anonymous message signing
- **Req-resp** protocols: Status + BlocksByRoot with snappy framed encoding
- **5-interval slot structure** (800ms each, 4s total): propose, attest, aggregate, safe-target, accept

## License

MIT
