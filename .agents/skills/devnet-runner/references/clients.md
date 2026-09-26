# Client Reference

Canonical client reference for gean's multi-client devnets. Other skills link
here instead of keeping their own client tables.

**Applies to:** devnet5 — gean `main`, whose `make docker-build` tags
`ghcr.io/geanlabs/gean:devnet5`. Peer-client images are whatever the checked-out
lean-quickstart pins; re-check them when the devnet changes.

## Default 5-Client Set

| Client | Language |
|---|---|
| gean | Go (system under test) |
| zeam | Zig |
| ream | Rust |
| lantern | C |
| ethlambda | Rust |

qlean, lighthouse (lean fork), and grandine exist but are not in the default
set; add them to `validator-config.yaml` when needed.

## Docker Images

Each client's image is the `node_docker` variable in
`lean-quickstart/client-cmds/<client>-cmd.sh`. List the pinned images with:

```bash
grep -H 'node_docker=' lean-quickstart/client-cmds/*-cmd.sh
```

To change an image or tag, edit `node_docker` in that file.

## gean Image Build

From the gean repo root on the devnet server:

```bash
make docker-build
# Tags: gean:$(VERSION)  (git describe --tags --always --dirty)
#       ghcr.io/geanlabs/gean:devnet5
```

For a feature branch, build with an explicit tag and point `gean-cmd.sh` at it:

```bash
docker build --build-arg GIT_COMMIT=$(git rev-parse HEAD) -t gean:my-feature .
```

The image entrypoint is the `gean` binary (`Dockerfile`).

## Ports

Ports are configured per node in `validator-config.yaml` (`enrFields.quic`,
`metricsPort`); gean's `--api-port` is set in `gean-cmd.sh`. Typical
assignments in the 5-client config:

| Node | QUIC Port | Metrics Port | API Port |
|---|---|---|---|
| zeam_0 | 9001 | 8081 | n/a |
| ream_0 | 9002 | 8082 | n/a |
| lantern_0 | 9004 | 8084 | n/a |
| ethlambda_0 | 9007 | 8087 | 5052 |
| gean_0 | 9008 | 8088 | 5058 |

gean and ethlambda run separate API and metrics HTTP servers: `metricsPort`
maps to `--metrics-port`, and the API port is configured in the client-cmd
script. gean's own flag defaults (`cmd/gean/flags.go`) are gossipsub 9000,
API 5052, metrics 5054, bound to `--http-address` (default `127.0.0.1`).

## Environment Variables Available to Client Scripts

Set by `spin-node.sh` for `client-cmds/*.sh`:

| Variable | Description |
|---|---|
| `$item` | Node name (e.g., `gean_0`) |
| `$configDir` | Genesis config directory path |
| `$dataDir` | Data directory path |
| `$quicPort` | QUIC port from config |
| `$metricsPort` | Metrics port from config |
| `$privkey` | P2P private key |
