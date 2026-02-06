# Gean

A Go implementation of the Lean Ethereum consensus protocol that is simple enough to last.

## Getting started

```bash
# Build
make build

# Run a single validator node
./bin/gean --validators=4 --validator-index=0 --log-level=debug
```

### Multi-node local devnet

Set a shared genesis time and start each validator in its own terminal:

```bash
GENESIS=$(date -d '+30 seconds' +%s)

# Terminal 1 — Validator 0
./bin/gean --validators=4 --validator-index=0 \
  --listen=/ip4/127.0.0.1/udp/9000/quic-v1 \
  --genesis-time=$GENESIS --log-level=debug

# Terminal 2 — Validator 1 (copy PEER_ID from Terminal 1 output)
./bin/gean --validators=4 --validator-index=1 \
  --listen=/ip4/127.0.0.1/udp/9001/quic-v1 \
  --bootnodes=/ip4/127.0.0.1/udp/9000/quic-v1/p2p/<PEER_ID> \
  --genesis-time=$GENESIS --log-level=debug

# Terminal 3 — Validator 2
./bin/gean --validators=4 --validator-index=2 \
  --listen=/ip4/127.0.0.1/udp/9002/quic-v1 \
  --bootnodes=/ip4/127.0.0.1/udp/9000/quic-v1/p2p/<PEER_ID> \
  --genesis-time=$GENESIS --log-level=debug

# Terminal 4 — Validator 3
./bin/gean --validators=4 --validator-index=3 \
  --listen=/ip4/127.0.0.1/udp/9003/quic-v1 \
  --bootnodes=/ip4/127.0.0.1/udp/9000/quic-v1/p2p/<PEER_ID> \
  --genesis-time=$GENESIS --log-level=debug
```

### CLI flags

| Flag | Default | Description |
|------|---------|-------------|
| `--genesis-time` | now + 10s | Unix timestamp for genesis |
| `--validators` | `8` | Total number of validators |
| `--validator-index` | `0` | Index of this node's validator |
| `--listen` | `/ip4/0.0.0.0/udp/9000/quic-v1` | QUIC listen address |
| `--bootnodes` | (none) | Comma-separated bootnode multiaddrs |
| `--log-level` | `info` | Log level: debug, info, warn, error |

## Philosophy

> *"Even if a protocol is super decentralized with hundreds of thousands of nodes... if the protocol is an unwieldy mess of hundreds of thousands of lines of code, ultimately that protocol fails."* — Vitalik Buterin

Our goal is to build a consensus client that is simple and readable yet elegant and resilient; code that anyone can read, understand, and maintain for decades to come. A codebase developers actually enjoy contributing to. It's why we chose Go.

## Acknowledgements

- [leanSpec](https://github.com/leanEthereum/leanSpec) — Python reference specification
- [ethlambda](https://github.com/lambdaclass/ethlambda) — Rust implementation by LambdaClass

## Current status

Target: [leanSpec Devnet 0](https://github.com/leanEthereum/leanSpec/tree/4b750f2748a3718fe3e1e9cdb3c65e3a7ddabff5)

### Implemented

- 3SF-mini consensus (per-attestation justification, consecutive-slot finalization)
- LMD-GHOST fork choice with safe-target tracking
- SSZ serialization via [fastssz](https://github.com/ferranbt/fastssz)
- libp2p networking (QUIC transport, gossipsub)
- Snappy-compressed block and vote gossip
- Round-robin block proposer
- 4-interval slot timing (propose, vote, safe-target update, accept)
- Chain sync via req/resp protocol
- State root validation on received blocks

### Next

- [pq-devnet-1](https://github.com/leanEthereum/pm/blob/main/breakout-rooms/leanConsensus/pq-interop/pq-devnet-1.md) — PQ signatures, lean-quickstart integration

## License

MIT