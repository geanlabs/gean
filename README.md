# gean Ethereum consensus client

<p align="center">
  <img src="docs/assets/gean-banner.svg" alt="gean: a Lean Ethereum consensus client in Go" width="100%">
</p>

## Overview

gean is an Ethereum consensus client written in Go. Today it runs **Lean Ethereum consensus**,
with post-quantum XMSS signatures; Ethereum beacon chain support is on the way, in the same binary.

Lean Ethereum is at the devnet stage, and so is gean. Expect breaking changes.

- **Post-quantum:** validators sign with XMSS, and aggregators combine signatures into leanVM proofs.
- **Spec-tested:** consensus logic is tested against the [leanSpec](https://github.com/leanEthereum/leanSpec) fixtures.
- **One client, two chains:** shared code in `internal/core` never depends on a chain, so the beacon
  chain can be added beside Lean.

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

## Running

Start a three-node local network:

```sh
make lean-testnet     # once: genesis config and keys in ./testnet
make lean-node N=0    # terminal 1: node0, the aggregator
make lean-node N=1    # terminal 2
make lean-node N=2    # terminal 3
```

Run against other Lean clients with `make lean-devnet`, through
[lean-quickstart](https://github.com/blockblaz/lean-quickstart).

Run `gean lean --help` for every flag, and `make help` for every target.

## License

MIT, see [LICENSE](LICENSE). To report a vulnerability, see [SECURITY.md](SECURITY.md).
