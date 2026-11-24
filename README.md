# gean Ethereum consensus client

<p align="center">
  <img src="docs/assets/gean-banner.svg" alt="gean: an Ethereum consensus client in Go" width="100%">
</p>

## Overview

gean is an Ethereum consensus client written in Go, becoming a client for the post-quantum beacon
chain. It has no runnable node yet.

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

## License

MIT, see [LICENSE](LICENSE). To report a vulnerability, see [SECURITY.md](SECURITY.md).
