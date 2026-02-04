# Gean

A Go implementation of the Lean Ethereum consensus protocol that is simple enough to last.

## Getting started

```bash
# Build
make build

# Run a single validator node
./bin/gean --validators=2 --validator-index=0

# Run a two-node network (use the same genesis time for both)
GENESIS=$(date -d '+30 seconds' +%s)

# Terminal 1
./bin/gean --validators=2 --validator-index=0 \
  --listen=/ip4/127.0.0.1/udp/9000/quic-v1 \
  --genesis-time=$GENESIS

# Terminal 2 (copy peer_id from Terminal 1 output)
./bin/gean --validators=2 --validator-index=1 \
  --listen=/ip4/127.0.0.1/udp/9001/quic-v1 \
  --bootnodes=/ip4/127.0.0.1/udp/9000/quic-v1/p2p/<PEER_ID> \
  --genesis-time=$GENESIS
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

- 3SF-mini consensus (2/3 supermajority justification)
- LMD-GHOST fork choice
- SSZ serialization (fastssz)
- libp2p networking (QUIC,Implements:: gossipsub)
- Round-robin block proposer
- Slot-based vote production

### Next

- [pq-devnet-1](https://github.com/leanEthereum/pm/blob/main/breakout-rooms/leanConsensus/pq-interop/pq-devnet-1.md) — PQ signatures, lean-quickstart integration

## License

MIT