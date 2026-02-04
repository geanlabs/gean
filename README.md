# Gean

A Go implementation of the Lean Ethereum consensus protocol.

## Quick Start

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

## Command Line Options

| Flag | Description | Default |
|------|-------------|---------|
| `--validators` | Total number of validators in the network | 8 |
| `--validator-index` | This node's validator index (omit for observer mode) | - |
| `--genesis-time` | Unix timestamp for genesis | now + 10s |
| `--listen` | libp2p listen address | /ip4/0.0.0.0/udp/9000/quic-v1 |
| `--bootnodes` | Comma-separated bootnode multiaddrs | - |
| `--log-level` | Log level (debug, info, warn, error) | info |

## Development

```bash
make test       # Run tests
make lint       # Run linters
make fmt        # Format code
make clean      # Remove build artifacts
```

## Status

**Target:** [leanSpec devnet0](https://github.com/leanEthereum/leanSpec)

Implements:
- 3SF-mini consensus (2/3 supermajority justification)
- LMD-GHOST fork choice
- SSZ serialization (fastssz)
- libp2p networking (QUIC, gossipsub)
- Round-robin block proposer
- Slot-based vote production

## Philosophy

> *"Even if a protocol is super decentralized... if the protocol is an unwieldy mess of hundreds of thousands of lines of code, ultimately that protocol fails."* — Vitalik Buterin

Simple, readable code that anyone can understand and maintain.

## Acknowledgements

- [leanSpec](https://github.com/leanEthereum/leanSpec) — Python reference specification
- [ethlambda](https://github.com/lambdaclass/ethlambda) — Rust implementation

## License

MIT
