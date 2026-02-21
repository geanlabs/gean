# gean

A Go consensus client for Lean Ethereum, built around the idea that protocol simplicity is a security property.

## Philosophy

A consensus client should be something a developer can read, understand, and verify without needing to trust a small class of experts. If you can't inspect it end-to-end, it's not fully yours.

## What is Lean Consensus

A complete redesign of Ethereum's consensus layer, hardened for security, decentralization, and finality in seconds.


## Design approach

- **Readable over clever.** Code is written so that someone unfamiliar with the codebase can follow it. Naming is explicit. Control flow is linear where possible.
- **Minimal dependencies.** Fewer imports means fewer things that can break, fewer things to audit, and fewer things to understand.
- **No premature abstraction.** Interfaces and generics are introduced when the duplication is real, not when it's hypothetical. Concrete types until proven otherwise.
- **Flat and direct.** Avoid deep package hierarchies and layers of indirection. A function should do what its name says, and you should be able to find it quickly.
- **Concurrency only where necessary.** Go makes concurrency easy to write and hard to reason about. We use it at the boundaries (networking, event loops) and keep the core logic sequential and deterministic.

## Current status

| Devnet | Status | Spec pin |
|--------|--------|----------|
| pq-devnet-0 | Complete | `leanSpec@4b750f2` |
| pq-devnet-1 | In progress | `leanSpec@050fa4a`, `leanSig@f10dcbe` |

devnet-1 progress:
- Done: consensus envelope pipeline (`SignedAttestation`, `SignedBlockWithAttestation`, proposer-attestation ordering, signed storage/sync path)
- Next: XMSS/leanSig integration (CGo bindings, key management, signing, verification), then cross-client interop

## Prerequisites

- **Go** 1.24.6+
- **Rust** 1.87+ (for the leanSig FFI library under `xmss/leansig-ffi/`)
- **uv** ([astral.sh/uv](https://docs.astral.sh/uv/)) — needed to generate leanSpec test fixtures

## Getting started

```sh
# Build (includes FFI library)
make build

# Run consensus spectests (builds FFI + fixtures)
make spec-test

# Run Go unit tests
make unit-test

# Lint
make lint

# Generate validator keys (XMSS)
./bin/keygen -validators 5 -keys-dir keys -print-yaml

# Generate node identity keys (libp2p/discv5)
go run ./scripts/gen_node_keys

# Run
make run
```

## leanSpec fixtures and spectests (devnet-1)

`make spec-test` is the primary consensus-conformance entry point. It bootstraps leanSpec fixtures and runs spectests in a signature-skip lane.

```sh
# Generate/update fixtures from pinned leanSpec commit
make leanSpec/fixtures

# Verify leanSpec pin used for fixtures
git -C leanSpec rev-parse HEAD
cat leanSpec/.fixtures-commit

# Run only consensus spectests (fork-choice + state-transition)
make spec-test

# Run Go unit tests across packages
make unit-test
```

Notes:
- Fixtures are generated under `leanSpec/fixtures`.
- `leanSpec/` is a local working directory and is gitignored.
- Devnet-1 fixture generation uses `uv run fill --fork=Devnet --layer=consensus --clean -o fixtures`.

## Metrics and Grafana

gean exposes Prometheus metrics at `/metrics` when `--metrics-port` is enabled.

```sh
./bin/gean \
  --genesis config.yaml \
  --bootnodes nodes.yaml \
  --validator-registry-path validators.yaml \
  --validator-keys keys \
  --node-id node0 \
  --metrics-port 8080
```

Grafana assets for gean are provided at:

- `observability/grafana/client-dashboard.json` (dashboard import)
- `observability/grafana/prometheus-scrape.example.yml` (scrape config example)

Dashboard notes:

- Datasource UID is hardcoded to `feyrb1q11ge0wa`.
- Panels filter targets using the `Gean Job` variable (`$gean_job`), populated from Prometheus `job` labels.

## Running in a devnet

gean is part of the [lean-quickstart](https://github.com/blockblaz/lean-quickstart) multi-client devnet tooling (integration in progress for devnet-1).


## Acknowledgements

- [Lean Ethereum](https://github.com/leanEthereum) 
- [ethlambda](https://github.com/lambdaclass/ethlambda) 


## License

MIT
