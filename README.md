# gean

A Go implementation of the Lean Ethereum consensus protocol that is simple enough to last.

## Getting Started

### Prerequisites

- [Go](https://go.dev/dl/) 1.22+
- [Docker](https://www.docker.com/get-started)

### Building and testing

We use `go build` under the hood, but prefer `make` as a convenient wrapper for common tasks. These are some common targets:

```sh
# Build the gean binary
make build

# Run all tests
make test

# Run tests with race detector
make test-race

# Run linters (vet + staticcheck)
make lint

# Format code
make fmt

# Build a Docker image
make docker-build
```

Run `make help` or take a look at our [`Makefile`](./Makefile) for other useful commands.

### Local devnet

The quickest way to see gean running alongside other Lean Ethereum clients is with `make run-devnet`. This handles cloning [lean-quickstart](https://github.com/blockblaz/lean-quickstart), building the Docker image, and spinning up a local network in one step.

```sh
make run-devnet
```

If you'd rather run things manually or add gean to an existing devnet setup:

```sh
make docker-build
cd lean-quickstart
NETWORK_DIR=local-devnet ./spin-node.sh --node gean_0,ream_0,zeam_0 --generateGenesis --metrics
```

Validator keys and network parameters live in `lean-quickstart/local-devnet/genesis/validator-config.yaml`.

## Philosophy

> *"Even if a protocol is super decentralized with hundreds of thousands of nodes... if the protocol is an unwieldy mess of hundreds of thousands of lines of code, ultimately that protocol fails."* — Vitalik Buterin

Our goal is to build a consensus client that is simple and readable yet elegant and resilient; code that anyone can read, understand, and maintain for decades to come. A codebase developers actually enjoy contributing to. It's why we chose Go.

## Current Status

The client implements the core features of a Lean Ethereum consensus client:

- **Networking** — libp2p peer connections over QUIC, STATUS message handling, gossipsub for blocks and attestations
- **State management** — genesis state generation, state transition function, block processing
- **Fork choice** — 3SF-mini fork choice rule implementation with attestation-based head selection
- **Validator duties** — attestation production and broadcasting, block building with fixed-point attestation collection
- **Sync** — missing block retrieval from peers via BlocksByRoot
- **Observability** — [leanMetrics](https://github.com/leanEthereum/leanMetrics) compatible Prometheus metrics

### pq-devnet-0

Support for pq-devnet-0 is complete. All core consensus features are implemented and tested.

Spec pin: leanSpec @ [`4b750f2`](https://github.com/leanEthereum/leanSpec/tree/4b750f2)

### pq-devnet-1

We are working on adding support for the pq-devnet-1 spec. This includes XMSS post-quantum signature signing and verification with naive aggregation (concatenated individual signatures), and interoperability with [lean-quickstart](https://github.com/blockblaz/lean-quickstart) for multi-client devnet participation.

Spec pin: leanSpec @ [`050fa4a`](https://github.com/leanEthereum/leanSpec/tree/050fa4a) | leanSig @ [`f10dcbe`](https://github.com/leanEthereum/leanSig/tree/f10dcbe)

### Coming up next

- pq-devnet-1 type restructure (`SignedBlockWithAttestation`, `SignedAttestation`, validator registry)
- Full vote-tracking attestation processing with supermajority justification and finalization
- XMSS signature verification via [leanSig](https://github.com/leanEthereum/leanSig)
- Checkpoint sync for long-lived networks
- Multi-client devnet interop

## Acknowledgements

- [LeanEthereum](https://github.com/leanEthereum)
- [ethlambda](https://github.com/lambdaclass/ethlambda)

## License

MIT
