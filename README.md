# gean

A Go implementation of the Lean Ethereum consensus protocol that is simple enough to last.

## Quick Start

**Prerequisites:** [Go](https://go.dev/dl/) 1.22+ and optionally [Docker](https://www.docker.com/get-started) for devnet.

```sh
make build          # compile
make test           # run all 86 tests
make run            # start a node with default config
make run-devnet     # spin up a multi-client devnet via lean-quickstart
make help           # see all targets
```

## Philosophy

> *"Even if a protocol is super decentralized with hundreds of thousands of nodes... if the protocol is an unwieldy mess of hundreds of thousands of lines of code, ultimately that protocol fails."* — Vitalik Buterin

Our goal is to build a consensus client that is simple and readable yet elegant and resilient; code that anyone can read, understand, and maintain for decades to come. A codebase developers actually enjoy contributing to. It's why we chose Go.

## What It Does

gean targets [pq-devnet-0](https://github.com/leanEthereum/pm/blob/main/breakout-rooms/leanConsensus/pq-interop/pq-devnet-0.md): 3SF-mini consensus, round-robin proposer selection, 4-second slots with 4 intervals each.

- Produces and gossips blocks (fixed-point attestation collection)
- Produces and gossips attestation votes
- Runs LMD GHOST fork choice with safe target tracking
- Syncs missing blocks from peers via BlocksByRoot
- Exports Prometheus metrics compatible with [leanMetrics](https://github.com/leanEthereum/leanMetrics)

Spec pin: leanSpec @ [`4b750f2`](https://github.com/leanEthereum/leanSpec/tree/4b750f2).

## Acknowledgements

- [leanSpec](https://github.com/leanEthereum/leanSpec) — Python reference specification
- [ethlambda](https://github.com/lambdaclass/ethlambda) — Rust implementation by LambdaClass

## License

MIT
