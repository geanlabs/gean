# gean Ethereum consensus client

[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Go version](https://img.shields.io/github/go-mod/go-version/geanlabs/gean)](go.mod)
[![Chat on Telegram](https://img.shields.io/badge/chat-Telegram-26A5E4?logo=telegram)][telegram]

<p align="center">
  <img src="docs/assets/gean-banner.svg" alt="gean: a Lean Ethereum consensus client in Go" width="100%">
</p>

## Overview

gean is an Ethereum consensus client written in Go. Today it runs **Lean Ethereum consensus**: it
follows the chain, imports and verifies blocks, runs fork choice, and performs validator duties
with post-quantum XMSS signatures. Ethereum beacon chain support is on the way, in the same
binary: `gean lean` for Lean Ethereum, `gean beacon` for the beacon chain.

Lean Ethereum is at the devnet stage, and so is gean. Expect breaking changes.

### Supported networks

| Chain | Status |
|---|---|
| **Lean Ethereum** | Working. Runs on Lean devnets and is tested against the leanSpec fixtures. |
| **Ethereum beacon chain** | Planned. |

## Capabilities

gean joins a Lean network over libp2p, keeps up with the chain through gossip and request/response
sync, and serves its state over an HTTP API and Prometheus metrics.

- **Post-quantum signatures:** validators sign with XMSS, a hash-based scheme, and aggregators
  combine signatures into leanVM proofs. The cryptography comes from leanVM through a small Rust
  library gean links against.

- **Spec conformance:** the state transition, fork choice, SSZ types and API are tested against
  the [leanSpec](https://github.com/leanEthereum/leanSpec) fixtures, pinned to one leanSpec commit.

- **Modularity:** networking, storage, the proto-array, the HTTP server and the other chain-agnostic
  parts live in `internal/core`, and each chain's rules live in their own packages. Import rules
  checked by `make lint` keep `core` from depending on any chain, so beacon support can be added
  beside Lean without touching it.

- **Networking:** QUIC transport, gossipsub, status and block-by-root/range sync, checkpoint sync
  from another node's API, and optional discv5 discovery.

- **Tested as a network:** an end-to-end test runs three full nodes in one process on an in-memory
  network, checks they finalize, stops one, and checks it catches up after a restart.

## Getting started

gean is built from source; there are no prebuilt releases. Once built (see
[Building from source](#building-from-source)), start a three-node local network:

```sh
make lean-testnet     # once: genesis config + XMSS keys for 5 validators across 3 nodes, in ./testnet
make lean-node N=0    # terminal 1: node0, the aggregator
make lean-node N=1    # terminal 2
make lean-node N=2    # terminal 3
```

`make lean-testnet` sets genesis 30 seconds in the future, so start the nodes soon after it. Node N
uses P2P port 9000+N, API port 5052+N and metrics port 8080+N, and `lean-node` wipes its data
directory (`data/nodeN`) on every start. Check that node0 is healthy and finalizing:

```sh
curl -s localhost:5052/lean/v0/health
curl -s localhost:8080/metrics | grep -E '^lean_(head_slot|latest_finalized_slot|connected_peers) '
```

To run a single node by hand:

```sh
bin/gean lean \
  --custom-network-config-dir ./testnet \
  --node-key ./testnet/node0.key \
  --node-id node0 \
  --data-dir ./data/node0
```

| Flag | Default | Meaning |
|---|---|---|
| `--custom-network-config-dir` | (required) | Directory with the network's config and keys, as written by `gean lean keygen` |
| `--node-key` | (required) | Hex-encoded secp256k1 key for the node's P2P identity |
| `--node-id` | (required) | Node name, e.g. `node0` |
| `--data-dir` | `./data` | Database directory |
| `--gossipsub-port` | `9000` | P2P listen port (QUIC over UDP) |
| `--http-address` | `127.0.0.1` | Bind address for the API and metrics servers |
| `--api-port` | `5052` | API port |
| `--metrics-port` | `5054` | Metrics port |
| `--checkpoint-sync-url` | (none) | Start from a finalized state fetched from another node's API |
| `--is-aggregator` | `false` | Aggregate attestation signatures |
| `--aggregate-subnet-ids` | (all) | Comma-separated subnets to aggregate; needs `--is-aggregator` |
| `--attestation-committee-count` | from config | Number of attestation subnets |
| `--discovery` | `false` | Find peers with discv5; no other Lean client runs it yet |
| `--discovery-port` | `9001` | discv5 UDP port; needs `--discovery` |
| `--discovery-advertise-ip` | (learned) | IP to put in this node's ENR; needs `--discovery` |
| `--prover-arena` | `false` | Faster proving, at the cost of memory that is never returned |

Run `gean --help` for the commands and `gean lean --help` for every flag.

### Multi-client devnet

To run gean against other Lean clients through
[lean-quickstart](https://github.com/blockblaz/lean-quickstart):

```sh
make lean-devnet
```

This builds the Docker image `ghcr.io/geanlabs/gean:local` (set `DOCKER_TAG` to change it) and
writes the devnet's output to `devnet.log`.

## Building from source

### Docker image

The quickest way to build gean, with no Go or Rust toolchain installed:

```sh
docker build https://github.com/geanlabs/gean.git -t gean
```

### Standalone binary

**Prerequisites**

| Tool | Version | Used for |
|---|---|---|
| Go | 1.26 | gean itself |
| Rust (via rustup) | 1.92.0, pinned in `internal/core/crypto/xmss/rust/rust-toolchain.toml` | the XMSS signature library |
| [golangci-lint](https://golangci-lint.run/) | v2 | `make lint` |
| [uv](https://docs.astral.sh/uv/) | any recent | optional: leanSpec fixtures for `make lean-spectest` |
| Docker | any recent | optional: images and multi-client devnets |

**Clone and build**

```sh
git clone https://github.com/geanlabs/gean.git
cd gean
make build
```

This builds the Rust XMSS library, then `bin/gean`. Run `make help` to list every target.

**Test**

| Command | Runs |
|---|---|
| `make test` | Unit tests |
| `make lean-spectest` | leanSpec fixture tests; the first run clones leanSpec at its pinned commit and generates the fixtures with `uv` |
| `make lean-e2e` | The end-to-end multi-node test, on fake XMSS (no Rust needed) |
| `make lean-test-ffi` | The XMSS library's tests |
| `make test-all` | All of the above but `lean-e2e`, plus a multi-node simulation with real XMSS (slow, about 1 GB) |
| `make lint` | `go vet`, golangci-lint (import rules fail the build) and the Rust `fmt`/`clippy` checks |

## Getting help

- [Telegram][telegram] for questions and discussion
- [GitHub Issues](https://github.com/geanlabs/gean/issues) to report a bug or request a feature

## Contributing

Before starting a feature or a larger fix, open an issue or ask on [Telegram][telegram], so the
work isn't wasted or duplicated.

## Security

If you believe you have found a security vulnerability, report it privately as described in our
[security policy](SECURITY.md).

## License

gean is open-source software licensed under the [MIT License](LICENSE).

[telegram]: https://t.me/geanclient
