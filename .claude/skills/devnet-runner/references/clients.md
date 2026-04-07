# Client Reference

Supported lean consensus clients used in gean's multi-client devnet
configuration.

## Supported Clients (gean's default 5-client devnet)

| Client | Language | Description |
|---|---|---|
| gean | Go | This client. Per-validator latest-vote selection, fast aggregator. |
| zeam | Zig | Lean consensus client with the best logging design. **Watch for SSZ panics**. |
| ream | Rust | Active development. **Sync recovery is fragile**. |
| lantern | C | Most reliable client in the network. Smart attestation selection. |
| ethlambda | Rust | Best fork choice tree visualization. **Has known block bloat bug**. |

Other clients that exist but are NOT in gean's default 5-client setup:

| Client | Language | Why excluded |
|---|---|---|
| qlean | C++ | Unreliable: `listen_addrs=0` config bug, frequent disconnects, no log shipping |
| lighthouse | Rust | Heavyweight Ethereum client (lean fork) — overkill for our tests |
| grandine | Rust | Not always available; can be added when needed |

## Docker Images

Images are defined in `client-cmds/{client}-cmd.sh`. Edit the `node_docker`
variable to change image/tag.

| Client | Default Image |
|---|---|
| gean | `gean:dev` (built locally via `make docker-build`) |
| zeam | `blockblaz/zeam:devnet1` |
| ream | `ghcr.io/reamlabs/ream:latest` |
| lantern | `piertwo/lantern:v0.0.1` |
| ethlambda | `ghcr.io/lambdaclass/ethlambda:devnet3` |

## Default Ports

Ports are configured per-node in `validator-config.yaml`. Typical port
assignments for the 5-client devnet:

| Node | QUIC Port | Metrics Port | API Port |
|---|---|---|---|
| zeam_0 | 9001 | 8081 | n/a |
| ream_0 | 9002 | 8082 | n/a |
| lantern_0 | 9004 | 8084 | n/a |
| ethlambda_0 | 9007 | 8087 | 5052 |
| gean_0 | 9008 | 8088 | 5058 |

**Note:** Adjust ports to avoid conflicts when running multiple instances of
the same client.

**Dual-port clients (gean, ethlambda):** Both run separate API and metrics
HTTP servers. The `metricsPort` from `validator-config.yaml` maps to
`--metrics-port`. The API port must be configured separately in the
client-cmd script.

## Client Command Files

Each client's Docker configuration is in `client-cmds/{client}-cmd.sh` (e.g.,
`gean-cmd.sh`, `zeam-cmd.sh`, `ethlambda-cmd.sh`). Edit the `node_docker`
variable to change image/tag.

## Changing Docker Images

To use a different image or tag:

1. **Temporary (single run):** Use `--tag` flag:
   ```bash
   NETWORK_DIR=local-devnet ./spin-node.sh --node gean_0 --tag my-branch
   ```

2. **Permanent:** Edit `client-cmds/{client}-cmd.sh` and modify `node_docker`:
   ```bash
   node_docker="your-registry/image:tag"
   ```

## Known Issues & Compatibility

### gean

| Issue | Status | Description |
|---|---|---|
| Block bloat bug | **FIXED in commit 62454aa** | Per-validator latest-vote selection ensures bounded block size. Earlier versions produced ~12 MB blocks during stalls. |
| Slow catch-up | **FIXED in commit e7e752c** | Batched `blocks_by_root` (up to 10 roots per request) speeds up restart catch-up. |
| Checkpoint init slot | **FIXED in commit e7e752c** | Anchor convention now matches ethlambda. |

### zeam

| Issue | Image Tags Affected | Description |
|---|---|---|
| SSZ stack overflow | All known | Crashes with `thread panic: integer overflow` or stack overflow in `serializedSize__anon_*` / `process_block`. Adversarial input or even valid blocks can trigger it. **Detection: zeam_0.log unusually large (>1M lines).** |
| CLI flag change | devnet2+ | Uses `--api-port` instead of `--metrics_port` for metrics endpoint |
| XMSS prover crash | devnet2 | Missing prover setup files cause panic when producing blocks with signature aggregation |

### ream

| Issue | Status | Description |
|---|---|---|
| Sync recovery fragile | Known | After a peer dies, ream cannot reseed missing fork-choice target states. Stuck `Justified` value persists across hundreds of slots. |
| `Attestation too far in future` | Known | ream rejects attestations for slots far ahead of its head, even when those slots are valid. |
| `No common highest checkpoint` | Known | Backfill cannot select a sync target when peers diverge. |

### ethlambda

| Issue | Status | Description |
|---|---|---|
| Block bloat regression | **OPEN UPSTREAM** | `crates/blockchain/src/store.rs:1018` greedily accumulates attestations. Same bug gean had before commit 62454aa. Issue filed. |
| Manifest unknown warning | local | Docker shows "manifest unknown" but falls back to local image — can be ignored |
| NoPeersSubscribedToTopic | all | Expected warning when no peers are connected to gossipsub topics |

### lantern

No known issues — most reliable client in our network. If lantern reports
errors, take them seriously.

## Environment Variables Available to Clients

These are set by `spin-node.sh` and available in client command scripts:

| Variable | Description |
|---|---|
| `$item` | Node name (e.g., `gean_0`) |
| `$configDir` | Genesis config directory path |
| `$dataDir` | Data directory path |
| `$quicPort` | QUIC port from config |
| `$metricsPort` | Metrics port from config |
| `$privkey` | P2P private key |

## gean Image Build

Unlike other clients, gean is typically built locally rather than pulled from a
registry. From the gean repo root:

```bash
make docker-build
# Produces image: gean:dev
```

For testing a feature branch, build with a custom tag:

```bash
docker build -t gean:my-feature-branch .
# Then update lean-quickstart/client-cmds/gean-cmd.sh to use the tag
```
