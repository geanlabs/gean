# Gean

A Go implementation of the Lean Ethereum consensus protocol that is simple enough to last.

## Getting started

```sh
# Build
make build

# Run standalone (requires genesis directory from lean-quickstart)
./bin/gean /path/to/genesis --node-id gean_0
```

## Running in a devnet

To run a local devnet with multiple clients using [lean-quickstart](https://github.com/blockblaz/lean-quickstart):

```sh
# Clone lean-quickstart
git clone https://github.com/blockblaz/lean-quickstart.git

# Generate genesis and start devnet
cd lean-quickstart
NETWORK_DIR=local-devnet ./spin-node.sh --node gean_0 --generateGenesis
```

## Philosophy

> *"Even if a protocol is super decentralized with hundreds of thousands of nodes... if the protocol is an unwieldy mess of hundreds of thousands of lines of code, ultimately that protocol fails."* — Vitalik Buterin

Our goal is to build a consensus client that is simple and readable yet elegant and resilient; code that anyone can read, understand, and maintain for decades to come. A codebase developers actually enjoy contributing to. It's why we chose Go.

## Acknowledgements

- [leanSpec](https://github.com/leanEthereum/leanSpec) — Python reference specification
- [ethlambda](https://github.com/lambdaclass/ethlambda) — Rust implementation by LambdaClass

## Current status

Target: [leanSpec devnet 0](https://github.com/leanEthereum/leanSpec/tree/4b750f2748a3718fe3e1e9cdb3c65e3a7ddabff5)

### Implemented

- **Types** — SSZ containers via fastssz (Block, State, Vote, Checkpoint, Config)
- **Consensus** — 3SF-mini justification rules, round-robin proposer selection
- **State transition** — slot processing, block header validation, attestation handling
- **Fork choice** — LMD-GHOST head selection, Store container
- **Networking** — libp2p host, gossipsub (block and attestation topics)
- **Node** — slot ticker, block and attestation production

### Remaining for Devnet 0

- Request-Response protocol (Status, BlocksByRoot)

## License

MIT
