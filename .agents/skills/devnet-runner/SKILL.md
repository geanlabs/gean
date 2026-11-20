---
name: devnet-runner
description: Run local lean devnets with gean and other lean clients through lean-quickstart. Use when asked to run a devnet, start or stop devnet nodes, choose which nodes or clients run, change client image tags, regenerate genesis, collect node logs, troubleshoot a devnet, or test what happens when nodes go down.
---

<!-- Adapted from ethlambda's devnet-runner skill (github.com/lambdaclass/ethlambda), MIT License, Copyright (c) 2025 Lambdaclass. -->

# Devnet runner

Run local multi-client lean devnets with this gean build, through
[lean-quickstart](https://github.com/blockblaz/lean-quickstart). For a quick gean-only network
without Docker, use `make lean-testnet` and `make lean-node N=...` instead.

## How gean joins the devnet

`make lean-devnet` builds `ghcr.io/geanlabs/gean:local`, clones `lean-quickstart/` if missing, and
rewrites the gean image line in `lean-quickstart/client-cmds/gean-cmd.sh` to
`ghcr.io/geanlabs/gean:local lean`: upstream's script names a published image and runs it without
the `lean` subcommand gean requires. Run devnets through it (or the script below), not
`spin-node.sh` directly on a fresh clone.

## Default workflow

1. **Choose the nodes.** Edit `lean-quickstart/local-devnet/genesis/validator-config.yaml` to list
   only the nodes that will run. Validators are dealt to every node in the file, so a listed node
   that doesn't run misses its proposals; don't use `--node <name>` to pick nodes, it doesn't
   reassign validators. Make sure one node has `isAggregator: true`, or nothing finalizes. See
   [references/validator-config.md](references/validator-config.md).
2. **Choose image tags** for other clients in `lean-quickstart/client-cmds/<client>-cmd.sh`
   (`node_docker`). See [references/clients.md](references/clients.md).
3. **Run for a fixed time**, 20 slots unless asked otherwise:

   ```bash
   .agents/skills/devnet-runner/scripts/run-devnet-with-timeout.sh 120
   ```

   It builds the image first, then runs the devnet for the given seconds, dumps every node's log to
   `<node>.log` in the repository root, and stops and removes the containers. The whole run's
   output is in `devnet.log`. Then analyze the logs with the
   [`devnet-log-review`](../devnet-log-review/SKILL.md) skill.

**Timeout** = 10 s startup + 30 s until genesis + 4 s per slot: 20 slots is 120 s, 50 slots 240 s,
100 slots 440 s.

## Manual commands

```bash
make lean-devnet                     # run until Ctrl+C; output in devnet.log
cd lean-quickstart && NETWORK_DIR=local-devnet ./spin-node.sh --node all --stop
docker logs -f gean_0                # follow one node
docker logs gean_0 > gean_0.log 2>&1 # dump one node
```

`spin-node.sh` flags: `--node <name|all>` (required), `--generateGenesis` (fresh genesis, implies
`--cleanData`), `--cleanData`, `--stop`, `--forceKeyGen` (new validator keys),
`--validatorConfig <path>`, `--dockerWithSudo`. Node data lives in
`lean-quickstart/local-devnet/data/<node>/`.

## Troubleshooting

- **Nodes won't start:** containers from an earlier run may still exist:
  `docker ps -a | grep -E 'gean|zeam|ream|qlean|lantern|grandine|ethlambda'`, then stop them with
  `spin-node.sh --node all --stop` or `docker rm -f`.
- **gean exits at once with a usage error:** `gean-cmd.sh` is missing the `lean` subcommand. Run
  `make lean-devnet` once to patch it.
- **No peers, or genesis mismatch** (`UnknownSourceBlock`, deserialization errors): regenerate
  genesis for every node, `spin-node.sh --node all --generateGenesis --forceKeyGen`, and remove
  stale containers first.
- **Port in use:** `lsof -i :<port>` for the QUIC, API and metrics ports in `validator-config.yaml`.
- **Nothing finalizes:** check one node has `isAggregator: true` and that two thirds of the
  validators' nodes are running.
- **Docker needs root:** add `--dockerWithSudo`.

## Testing node failures

`docker pause` stops nodes without losing their state; `docker unpause` resumes them. Pausing
nodes holding more than a third of the validators stalls finality; the head keeps advancing.

```bash
docker pause gean_1 gean_2
docker unpause gean_1 gean_2
```

See [references/instability-testing.md](references/instability-testing.md) for scenarios and for
extracting per-block processing times from the logs.
