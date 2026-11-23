# gean Ethereum consensus client

<p align="center">
  <img src="docs/assets/gean-banner.svg" alt="gean: a Lean Ethereum consensus client in Go" width="100%">
</p>

## Overview

gean is an Ethereum consensus client written in Go: one binary, with a subcommand per chain.
[`gean lean`](#lean-ethereum--gean-lean) runs Lean Ethereum, on pq-devnet-5;
[`gean beacon`](#ethereum-beacon-chain--gean-beacon), for the Ethereum Beacon Chain, is planned.

## Building

You need Go 1.26 and Rust 1.92:

```sh
git clone https://github.com/geanlabs/gean.git
cd gean
make build
```

Or build a Docker image, with no toolchain installed:

```sh
docker build https://github.com/geanlabs/gean.git -t gean
```

Or pull one from `ghcr.io/geanlabs/gean`, for amd64 or arm64. `unstable` is the newest `main`,
`sha-abc1234` is that commit, and `devnet5`-style tags are the builds devnets run. To check which
commit and run built an image:

```sh
gh attestation verify oci://ghcr.io/geanlabs/gean:unstable --repo geanlabs/gean
```

Run `make help` for every target.

## Lean Ethereum — `gean lean`

gean implements a Lean Ethereum consensus client:

- **Networking:** libp2p over QUIC, gossipsub, status and block sync, checkpoint sync, optional discv5
- **Consensus:** state transition and 3SF-mini fork choice, tested against the
  [leanSpec](https://github.com/leanEthereum/leanSpec) fixtures
- **Validator duties:** attestations, block building, and post-quantum XMSS signatures aggregated
  into leanVM proofs
- **Operations:** Prometheus metrics and the `/lean/v0` API

Lean Ethereum is at the devnet stage, and so is gean. Expect breaking changes.

Start a three-node local network:

```sh
make lean-testnet     # once: genesis config and keys in ./testnet
make lean-node N=0    # terminal 1: node0, the aggregator
make lean-node N=1    # terminal 2
make lean-node N=2    # terminal 3
```

Run against other Lean clients with `make lean-devnet`, through
[lean-quickstart](https://github.com/blockblaz/lean-quickstart). Run `gean lean --help` for every
flag.

## Ethereum Beacon Chain — `gean beacon`

Planned. It will share gean's networking, storage and tooling, which live in `internal/core` and
depend on no chain.

## License

MIT, see [LICENSE](LICENSE). To report a vulnerability, see [SECURITY.md](SECURITY.md).
