# gean

Go implementation of the Lean Ethereum consensus protocol.

## What is gean

gean is a Lean consensus client focused on protocol simplicity, readability, and long-term maintainability.

We treat simplicity as a core security and decentralization property:
- trustlessness improves when more people can inspect and reason about the protocol
- the walkaway test improves when new teams can build clients without relying on a small expert class
- self-sovereignty improves when protocol behavior is understandable end-to-end

## What gean is building

gean targets Lean consensus (formerly beam chain): a next-generation Ethereum consensus direction focused on stronger security, decentralization, and faster finality.

## What are our goals

Build a Lean consensus client that is:
- Simple
- Contributor-friendly
- Interoperable
- Production-minded

And evolve it with explicit simplification principles:
- prefer subtraction and cleanup over feature bloat
- keep critical invariants clear and testable
- minimize unnecessary complexity and dependency surface

## Current status

- **pq-devnet-0:** complete
- **pq-devnet-1:** in progress
  - completed: consensus envelope pipeline updates (`SignedAttestation`, `SignedBlockWithAttestation`, proposer-attestation flow, signed storage path)
  - next: XMSS/leanSig integration (signing, verification, key loading), then interop hardening

Spec pins:
- devnet-0: `leanSpec@4b750f2`
- devnet-1: `leanSpec@050fa4a`, `leanSig@f10dcbe`

## Getting Started


```sh
make build
make test
make lint
make run
```


## Acknowledgements

- [LeanEthereum](https://github.com/leanEthereum)
- [ethlambda](https://github.com/lambdaclass/ethlambda)

## License

MIT
