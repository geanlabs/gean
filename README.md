# Gean: Lean Ethereum consensus client

An open-source Lean Ethereum consensus client, written in Go and maintained by Gean Labs.

[Documentation](https://github.com/geanlabs/gean)

![Gean banner](docs/assets/gean-banner.png)

## Overview

Gean is:

- A consensus client for **[Lean Ethereum](https://github.com/leanEthereum)** — the successor to today's Beacon Chain, designed for fast finality, quantum-resistant security, and a simpler core protocol.
- Currently running on the **Lean Consensus devnet-4** alongside other independent client implementations built by separate teams.
- Fully open-source under the MIT license. Anyone can read, audit, or build on the code.
- Built for **clarity and auditability**, with a deliberately small codebase that researchers, security reviewers, and contributors can read end-to-end.
- **Quantum-resistant by design** — uses [XMSS](https://en.wikipedia.org/wiki/Hash-based_cryptography), a post-quantum signature scheme, in place of the BLS signatures used in today's Ethereum.
- Built and maintained by [Gean Labs](https://github.com/geanlabs) as part of the multi-client Lean Ethereum effort to bring the next chapter of Ethereum consensus to production.

## Getting started

### Prerequisites

- [Go](https://go.dev/doc/install) 1.25+
- [Rust](https://www.rust-lang.org/tools/install) 1.90.0, for building the XMSS FFI
- [uv](https://docs.astral.sh/uv/), for generating leanSpec fixtures
- [Docker](https://www.docker.com/get-started), for devnet workflows

### Building and testing

Gean uses `make` as a thin wrapper around Go, Rust FFI, Docker, and fixture-generation commands. Common targets:

```sh
# Build the Rust FFI libraries and the gean and keygen binaries
make build

# Run Go unit tests (excludes spec and FFI tests)
make test

# Build the FFI and run XMSS tests
make test-ffi

# Generate leanSpec fixtures and run spec-conformance tests
make leanSpec/fixtures
make test-spec

# Format Go and Rust sources
make fmt

# Run go vet, cargo fmt --check, and clippy
make lint

# Build the Docker image
make docker-build
```

Run `make help` or browse the [`Makefile`](Makefile) for the full set of targets.

### Running in a devnet

To run Gean in a local multi-client devnet through [`lean-quickstart`](https://github.com/blockblaz/lean-quickstart):

```sh
# Clones lean-quickstart, builds the docker image, and starts a local devnet
make run-devnet
```

For a small Gean-only local network, generate keys and start three nodes in separate terminals:

```sh
make run-setup
make run        # node 0 (aggregator)
make run-node1  # node 1
make run-node2  # node 2
```

> **Important:** When running nodes manually, at least one node must be started as an aggregator so attestations are aggregated and included in blocks. Without an aggregator the network will produce blocks but never finalize.

## Current status

Gean currently tracks **Lean Consensus devnet-4**, pinned to `leanSpec@70fc774`.

### Devnet support

- **devnet-4** — currently tracked. The pin is set by `LEAN_SPEC_COMMIT_HASH` in the [Makefile](Makefile); changes to the commit propagate through `make leanSpec/fixtures` and `make test-spec`.

Support for older devnet versions is discontinued when the next devnet version is adopted.

## Philosophy

Gean treats reviewability as a consensus-safety property. A client that can be read end-to-end by a single contributor is a client where consensus-critical behavior has fewer places to hide and fewer layers to mislead an auditor. The same property is what makes multi-client interoperability checks credible: when Gean and another Lean client disagree on a fork choice or a state root, the divergence should be locatable in a small number of files.

The Lean Ethereum specification is early and fast-moving. Gean optimizes for iteration speed against `leanSpec` rather than for long-term backward compatibility — interfaces, types, and even package shapes are expected to evolve as the spec does. Annotating non-obvious behavior with the `leanSpec PR #NNN` that justifies it is how that velocity stays compatible with auditability: a future reviewer can reconstruct intent without re-reading every spec discussion that led to a given branch.

## Branches

Gean uses `main` for active development. Devnet-specific branches may be cut for stable testing against a particular `leanSpec` version; the active devnet pin lives in the [Makefile](Makefile).

## References and acknowledgements

Gean is part of the multi-client Lean Ethereum ecosystem and learns from the work of other client teams:

- [ethlambda](https://github.com/lambdaclass/ethlambda)
- [zeam](https://github.com/blockblaz/zeam)


## License

Gean is open-source software released under the MIT license.
