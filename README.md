# Gean

A Go implementation of the Lean Ethereum consensus protocol that is simple enough to last.

## Getting started

```bash
# Build
make build

# Run a single validator node
./bin/gean --validators=4 --validator-index=0 --log-level=debug
```

## Philosophy

> *"Even if a protocol is super decentralized with hundreds of thousands of nodes... if the protocol is an unwieldy mess of hundreds of thousands of lines of code, ultimately that protocol fails."* — Vitalik Buterin

Our goal is to build a consensus client that is simple and readable yet elegant and resilient; code that anyone can read, understand, and maintain for decades to come. A codebase developers actually enjoy contributing to. It's why we chose Go.

## Acknowledgements

- [leanSpec](https://github.com/leanEthereum/leanSpec) — Python reference specification
- [ethlambda](https://github.com/lambdaclass/ethlambda) — Rust implementation by LambdaClass

## Status

### Devnet 0 — Complete

Spec: [leanSpec @ `4b750f2`](https://github.com/leanEthereum/leanSpec/tree/4b750f2748a3718fe3e1e9cdb3c65e3a7ddabff5)

- 3SF-mini consensus (per-attestation justification, consecutive-slot finalization)
- LMD-GHOST fork choice with safe-target tracking
- SSZ serialization via [fastssz](https://github.com/ferranbt/fastssz)
- libp2p networking (QUIC transport, gossipsub)
- Snappy-compressed block and vote gossip
- Round-robin block proposer
- 4-interval slot timing (propose, vote, safe-target update, accept)
- Chain sync via req/resp protocol
- State root validation on received blocks

### Devnet 1 — In Progress

Spec: [leanSpec @ `050fa4a`](https://github.com/leanEthereum/leanSpec/tree/050fa4a) | Devnet info: [pq-devnet-1](https://github.com/leanEthereum/pm/blob/main/breakout-rooms/leanConsensus/pq-interop/pq-devnet-1.md)

Devnet 1 upgrades gean with post-quantum signatures, supermajority consensus, and multi-client interoperability.

**What's changing:**

| Area | Devnet 0 | Devnet 1 |
|------|----------|----------|
| Signatures | None (trusted) | XMSS post-quantum (KoalaBear + Poseidon2) |
| Justification | Single attestation justifies | 2/3 supermajority required |
| Fork choice | Target-based LMD-GHOST | Head-based LMD-GHOST |
| Block format | `SignedBlock` | `SignedBlockWithAttestation` (sigs in envelope) |
| Attestations | `Vote` / `SignedVote` | `Attestation` / `SignedAttestation` |
| Validator identity | Index only | Registry with pubkeys (`State.Validators`) |
| Interop | Standalone | Multi-client via lean-quickstart |

**Roadmap:**

| Phase | Description | Status |
|-------|-------------|--------|
| 1 — Containers | SSZ type restructuring | Planned |
| 2 — State & Genesis | Validator registry, genesis updates | Planned |
| 3 — XMSS | Post-quantum signature scheme | Planned |
| 4 — Consensus | 2/3 supermajority state transition | Planned |
| 5 — Fork Choice | Head-based LMD-GHOST, store rewrite | Planned |
| 6 — Production | Block & attestation production with signing | Planned |
| 7 — Networking | Gossipsub, req/resp, Snappy fixes | Planned |
| 8 — Node & CLI | Key management, node orchestration | Planned |
| 9 — Testing | Spec fixtures, XMSS cross-validation | Planned |
| 10 — Interop | lean-quickstart multi-client integration | Planned |

## License

MIT