<!-- Adapted from ethlambda (github.com/lambdaclass/ethlambda), MIT License, Copyright (c) 2025 Lambdaclass. -->

# Client Reference

Supported lean consensus clients and their configurations.

## Supported Clients

| Client | Description |
|--------|-------------|
| zeam | Zig-based lean consensus client |
| ream | Rust-based lean consensus client |
| qlean | QEMU/Kagome lean implementation |
| lantern | PierTwo's lean consensus client |
| lighthouse | Rust Ethereum consensus client (lean fork) |
| grandine | High-performance consensus client |
| ethlambda | LambdaClass Rust implementation |
| gean | Go implementation (this repository) |

## Docker Images

Images are defined in `client-cmds/{client}-cmd.sh`. Edit the `node_docker` variable to change image/tag.

| Client | Default Image |
|--------|---------------|
| zeam | `blockblaz/zeam:devnet1` |
| ream | `ghcr.io/reamlabs/ream:latest` |
| qlean | `qdrvm/qlean-mini:3a96a1f` |
| lantern | `piertwo/lantern:v0.0.1` |
| lighthouse | `hopinheimer/lighthouse:latest` |
| grandine | `sifrai/lean:unstable` |
| ethlambda | `ghcr.io/lambdaclass/ethlambda:local` |
| gean | `ghcr.io/geanlabs/gean:devnetX` (`make lean-devnet` switches it to `:local`) |

## Default Ports

Ports are configured per-node in `validator-config.yaml`. Typical port assignments:

| Node | QUIC Port | Metrics Port |
|------|-----------|--------------|
| *_0 | 9001 | 8081 |
| *_1 | 9002 | 8082 |
| *_2 | 9003 | 8083 |

**Note:** Adjust ports to avoid conflicts when running multiple nodes.

**ethlambda dual-port note:** ethlambda runs separate API (`--api-port`, default 5052) and metrics (`--metrics-port`, default 5054) HTTP servers. Both share a bind address (`--http-address`, default `127.0.0.1`). The `metricsPort` from `validator-config.yaml` maps to `--metrics-port`. The API port must be configured separately in `ethlambda-cmd.sh`.

**gean note:** gean also runs separate API (`--api-port`) and metrics (`--metrics-port`) servers;
`gean-cmd.sh` passes both from the node's config. gean's binary needs the `lean` subcommand
(`gean lean --custom-network-config-dir ...`). Upstream `gean-cmd.sh` predates it, so
`make lean-devnet` rewrites the image line of the local clone to `ghcr.io/geanlabs/gean:local lean`.

## Client Command Files

Each client's Docker configuration is in `client-cmds/{client}-cmd.sh` (e.g., `zeam-cmd.sh`, `ream-cmd.sh`, `ethlambda-cmd.sh`). Edit the `node_docker` variable to change image/tag.

## Changing Docker Images

To use a different image or tag:

1. **Temporary (single run):** Use `--tag` flag:
   ```bash
   NETWORK_DIR=local-devnet ./spin-node.sh --node zeam_0 --tag dev
   ```

2. **Permanent:** Edit `client-cmds/{client}-cmd.sh` and modify `node_docker`:
   ```bash
   node_docker="your-registry/image:tag"
   ```

## Known Issues & Compatibility

### zeam

| Issue | Image Tags Affected | Description |
|-------|---------------------|-------------|
| CLI flag change | devnet2+ | Uses `--api-port` instead of `--metrics_port` for metrics endpoint |
| XMSS prover crash | devnet2 | Missing prover setup files cause panic when producing blocks with signature aggregation |
| Block format incompatibility | devnet2 ↔ ethlambda:local | Cannot deserialize blocks from ethlambda - OutOfMemory error |

### gean

| Issue | Image Tags Affected | Description |
|-------|---------------------|-------------|
| `lean` subcommand | devnet5 and later builds of main | Upstream `gean-cmd.sh` runs the image without `lean`; use `make lean-devnet`, which patches the local clone |

### ethlambda

| Issue | Image Tags Affected | Description |
|-------|---------------------|-------------|
| Separate API and metrics ports | PR #210+ | ethlambda now uses `--http-address`, `--api-port`, and `--metrics-port` instead of the old single `--metrics-address`/`--metrics-port`. `ethlambda-cmd.sh` in lean-quickstart must pass both `--api-port` and `--metrics-port` |
| Manifest unknown warning | local | Docker shows "manifest unknown" but falls back to local image - can be ignored |
| NoPeersSubscribedToTopic | all | Expected warning when no peers are connected to gossipsub topics |

## Environment Variables Available to Clients

These are set by `spin-node.sh` and available in client command scripts:

| Variable | Description |
|----------|-------------|
| `$item` | Node name (e.g., `zeam_0`) |
| `$configDir` | Genesis config directory path |
| `$dataDir` | Data directory path |
| `$quicPort` | QUIC port from config |
| `$metricsPort` | Metrics port from config. For ethlambda, maps to `--metrics-port`; API server needs separate `--api-port` |
| `$privkey` | P2P private key |
