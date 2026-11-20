# gean

A Go consensus client for Lean Ethereum, with Ethereum beacon chain support on the way.

## What is gean?

gean is a consensus client written in Go. Today it runs **Lean Ethereum consensus**: it follows
the chain, imports and verifies blocks, runs fork choice, and performs validator duties with
post-quantum XMSS signatures.

It is being restructured so that **one binary runs either chain**: `gean beacon` for the
Ethereum beacon chain and `gean lean` for Lean Ethereum.

## Goals

- **Two chains, one client.** Beacon and Lean share networking, storage and tooling, while each
  chain's rules live in their own packages and never depend on each other.
- **Modular.** Code is organised by protocol concept, so a change to one part of the protocol has
  an obvious home and doesn't ripple through the rest.
- **Easy to contribute to.** A newcomer should be able to find where a behaviour starts, follow it
  to the code that decides it, and find the test for it.
- **Fast enough for mainnet.** The beacon chain is built to keep up with Ethereum mainnet.

## Status

| Chain | Status |
|---|---|
| Lean Ethereum | Working. Runs on Lean devnets and is tested against the leanSpec fixtures. |
| Ethereum beacon chain | Planned. The codebase is being restructured to make room for it. |

Lean Ethereum is still at the devnet stage, and so is gean. Expect breaking changes.

## Quick start

Build gean (see [Requirements](#requirements) first), then start a three-node local network:

```sh
make build
make run-setup   # once: genesis config + XMSS keys for 5 validators across 3 nodes, in ./testnet
make run         # terminal 1: node0, the aggregator
make run-node1   # terminal 2
make run-node2   # terminal 3
```

`make run-setup` sets genesis 30 seconds in the future, so start the nodes soon after it; run it
again to refresh genesis. The `run` targets wipe their node's data directory (`data/nodeN`) on
every start.

## Check it works

Ask node0's API (port 5052):

```sh
curl -s localhost:5052/lean/v0/health                # {"status":"healthy",...}
curl -s localhost:5052/lean/v0/checkpoints/justified  # slot should rise every few slots
```

Or its Prometheus metrics (port 8080):

```sh
curl -s localhost:8080/metrics | grep -E '^lean_(head_slot|latest_justified_slot|latest_finalized_slot|connected_peers) '
```

`lean_head_slot` should advance every slot, `lean_latest_finalized_slot` should follow it, and
`lean_connected_peers` should be 2.

## Networks

### Local network

| Node | P2P port (UDP) | API | Metrics |
|---|---|---|---|
| node0 | 9000 | 127.0.0.1:5052 | 127.0.0.1:8080 |
| node1 | 9001 | 127.0.0.1:5053 | 127.0.0.1:8081 |
| node2 | 9002 | 127.0.0.1:5054 | 127.0.0.1:8082 |

Change the network's size with `NUM_VALIDATORS`, `NUM_NODES` and `TESTNET_DIR`, for example
`make run-setup NUM_VALIDATORS=8`.

### Multi-client devnet

```sh
make run-devnet
```

Builds a Docker image and starts a devnet with other Lean clients through
[lean-quickstart](https://github.com/blockblaz/lean-quickstart). Output goes to `devnet.log`.

### Checkpoint sync

Start from a recent finalized state instead of genesis with `--checkpoint-sync-url`, pointed at
another node's API.

## Flags

```sh
bin/gean \
  --custom-network-config-dir ./testnet \
  --node-key ./testnet/node0.key \
  --node-id node0 \
  --data-dir ./data/node0
```

| Flag | Default | Meaning |
|---|---|---|
| `--custom-network-config-dir` | (required) | Directory with the network's config and keys, as written by `keygen` |
| `--node-key` | (required) | Hex-encoded secp256k1 key for the node's P2P identity |
| `--node-id` | (required) | Node name, e.g. `node0` |
| `--data-dir` | `./data` | Database directory |
| `--gossipsub-port` | `9000` | P2P listen port (QUIC over UDP) |
| `--http-address` | `127.0.0.1` | Bind address for the API and metrics servers |
| `--api-port` | `5052` | API port |
| `--metrics-port` | `5054` | Metrics port |
| `--checkpoint-sync-url` | (none) | Start from a finalized state fetched from this URL |
| `--is-aggregator` | `false` | Aggregate attestation signatures |
| `--aggregate-subnet-ids` | (all) | Comma-separated subnets to aggregate; needs `--is-aggregator` |
| `--attestation-committee-count` | from config | Number of attestation subnets |
| `--prover-arena` | `false` | Faster proving, at the cost of memory that is never returned |

Run `bin/gean --help` for the full list.

## For developers

### Requirements

| Tool | Version | Used for |
|---|---|---|
| Go | 1.26 | gean itself |
| Rust (via rustup) | 1.92.0, pinned in `xmss/rust/rust-toolchain.toml` | the XMSS signature library, called from Go |
| [uv](https://docs.astral.sh/uv/) | any recent | generating leanSpec test fixtures (`make test-spec`) |
| [golangci-lint](https://golangci-lint.run/) | v2 | `make lint`; install with `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest` |
| Docker | any recent | optional: images and multi-client devnets |

### Build

```sh
make build
```

This builds the Rust XMSS libraries first, then two binaries:

- `bin/gean`: the consensus node
- `bin/keygen`: generates a local network's genesis config and validator keys

Run `make help` to list every target.

### Test

| Command | Runs |
|---|---|
| `make test` | Unit tests (no XMSS FFI, no spec fixtures) |
| `make test-ffi` | XMSS FFI tests |
| `make test-spec` | leanSpec fixture tests; the first run clones leanSpec at a pinned commit and generates fixtures with `uv` |
| `make test-all` | Everything above |
| `make lint` | `go vet` and golangci-lint (`.golangci.yml`, report-only for now), plus `cargo fmt --check` and `clippy` for the Rust code |
| `make fmt` | Format Go code |

## License

MIT. See [LICENSE](LICENSE). To report a vulnerability, see [SECURITY.md](SECURITY.md).
