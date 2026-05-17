# Gean: Lean Ethereum consensus client

An open-source Lean Ethereum consensus client, written in Go and maintained by Gean Labs.

[Documentation](https://github.com/geanlabs/gean)

![Gean banner](docs/assets/gean-banner.png)

## Overview

Gean is:

- A Go implementation of the [Lean Ethereum](https://github.com/leanEthereum/leanSpec) consensus specification.
- Built around a readable single-writer consensus core.
- Designed for devnet participation, spec testing, and multi-client interoperability.
- Integrated with XMSS post-quantum signatures through Rust FFI.
- Focused on small modules, direct control flow, and spec-linked implementation notes.

Gean currently targets **Lean Consensus devnet-4**.

## Getting Started

### Prerequisites

- [Go](https://go.dev/doc/install) 1.25+
- [Rust](https://www.rust-lang.org/tools/install) 1.90.0, for XMSS FFI
- [uv](https://docs.astral.sh/uv/), for generating leanSpec fixtures
- [Docker](https://www.docker.com/get-started), for devnet workflows

### Building and testing

Gean uses `make` as a wrapper around common Go, Rust FFI, Docker, and fixture-generation tasks.

```sh
# Build Rust FFI libraries and Go binaries
make build

# Run regular Go tests
make test

# Run XMSS FFI tests
make test-ffi

# Generate leanSpec fixtures and run spec tests
make leanSpec/fixtures
make test-spec

# Build the Docker image
make docker-build
```

Run `make help` or see the [Makefile](Makefile) for other commands.

### Running a devnet

To run Gean in a local multi-client devnet through [lean-quickstart](https://github.com/blockblaz/lean-quickstart):

```sh
make run-devnet
```

For a small Gean-only local network, generate keys/config and start three nodes in separate terminals:

```sh
make run-setup
make run
make run-node1
make run-node2
```

When running nodes manually, at least one node should be started as an aggregator so attestations are aggregated and included in blocks.

## Current Status

Gean currently tracks **Lean Consensus devnet-4**, pinned to `leanSpec@70fc774`.

## Features

Gean implements the core pieces of a Lean Ethereum consensus client:

- **Networking**: libp2p peer connections, gossipsub, status exchange, and block req/resp.
- **State management**: genesis loading, state transition, block processing, and checkpoint sync.
- **Fork choice**: 3SF-mini fork choice with LMD GHOST head selection over a proto-array.
- **Validator duties**: block proposal, attestation production, aggregation, and publishing.
- **Observability**: separate HTTP API and Prometheus metrics listeners.

## Documentation

Developer and contributor notes are available in:

- [AGENTS.md](AGENTS.md)
- [CLAUDE.md](CLAUDE.md)

Consensus behavior is anchored to the pinned [leanSpec](https://github.com/leanEthereum/leanSpec) commit used by the Makefile.

## Philosophy

Gean treats simplicity as part of consensus safety. The goal is a client that can be read, audited, and modified without forcing contributors through unnecessary layers of indirection.

### Design approach

- **Readable over clever**: code is written so that someone unfamiliar with the codebase can follow it. Naming is explicit, and control flow is linear where possible.
- **Minimal dependencies**: fewer imports mean fewer things that can break, fewer things to audit, and fewer things to understand.
- **No premature abstraction**: interfaces and generics are introduced when the duplication is real, not when it is hypothetical. Concrete types come first.
- **Flat and direct**: avoid deep package hierarchies and layers of indirection. A function should do what its name says, and it should be easy to find.
- **Concurrency only where necessary**: Go makes concurrency easy to write and hard to reason about. Gean uses it at the boundaries, while keeping core consensus logic sequential and deterministic.
- **Spec-linked behavior**: preserve `leanSpec PR #NNN` citations for behavior that is not obvious from the code alone.

## Branches

Gean uses `main` for active development unless a release or devnet branch is explicitly requested.

## Contributing

Contributions should keep the client simple, readable, and spec-grounded. Changes to consensus behavior should cite the relevant `leanSpec` PR or fixture evidence, and pull requests should list the tests or devnet checks that were run.

## References and Acknowledgements

Gean is part of the Lean Ethereum multi-client ecosystem and learns from the work of other client teams.

- [leanSpec](https://github.com/leanEthereum/leanSpec), the specification that anchors Gean's consensus behavior.
- [ethlambda](https://github.com/lambdaclass/ethlambda), for its minimalist Rust Lean client and practical devnet documentation.
- [zeam](https://github.com/blockblaz/zeam), for its Zig Lean client work, systems-level experimentation, and threading-design notes.

## License

MIT
