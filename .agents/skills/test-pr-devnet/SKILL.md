---
name: test-pr-devnet
description: Test gean PR changes in multi-client devnet. Use when users want to (1) Test a branch/PR with other Lean clients, (2) Validate BlocksByRoot or P2P protocol changes, (3) Test sync recovery with pause/unpause, (4) Verify cross-client interoperability, (5) Run integration tests before merging.
disable-model-invocation: true
---

# Test PR in Devnet

Test a gean branch in the 5-client lean-quickstart devnet. Clients, images,
and ports: [devnet-runner/references/clients.md](../devnet-runner/references/clients.md).

## Where to Run

Run on the user's devnet server, never on the local laptop. If the host is
unknown, ask for it. On the server, check out the branch in the gean repo and
run the scripts from the repo root; they are host-agnostic.

## Quick Start

```bash
git checkout my-feature-branch

# Basic interoperability (~60s observation, RUN_DURATION=N to change)
.agents/skills/test-pr-devnet/scripts/test-branch.sh

# With sync recovery (pauses zeam_0 and ream_0)
.agents/skills/test-pr-devnet/scripts/test-branch.sh --with-sync-test

# Status while running, then stop
.agents/skills/test-pr-devnet/scripts/check-status.sh
.agents/skills/test-pr-devnet/scripts/cleanup.sh
```

## What test-branch.sh Does

1. Requires HEAD to be on the requested branch (default: the current one) and
   prints the full commit SHA and `git status --porcelain` (dirty worktrees
   are built as-is and flagged).
2. Refuses to start if a previous run is still recorded or any of `zeam_0`,
   `ream_0`, `lantern_0`, `ethlambda_0`, `gean_0` already exists — those may
   belong to another devnet on the server.
3. Builds `gean:<branch>` (characters outside `[A-Za-z0-9_.-]`, such as `/`,
   become `-`) and prints the image ID.
4. Points `lean-quickstart/client-cmds/gean-cmd.sh` at that image, keeping a
   run-scoped backup that is restored when the script exits.
5. Starts the devnet with fresh genesis and records the containers it created
   in `${TMPDIR:-/tmp}/test-pr-devnet-<uid>/` (with the spin-node.sh log).
6. Optionally pauses/unpauses `zeam_0` and `ream_0` to exercise batched
   `blocks_by_root` sync.
7. Reports a verdict and leaves the devnet running for inspection.

`cleanup.sh` removes only the recorded containers and stops that run's
spin-node.sh. `check-status.sh` reports only those containers.

## Verdict

The finalized slot is the last `[forkchoice] finalized advanced slot=N` gean
logged (`internal/node/head.go`), sampled at the start and end of the
observation window.

| Verdict | Exit | Condition |
|---|---|---|
| PASS | 0 | gean_0 running the built image and its finalized slot advanced during the window |
| FAIL | 1 | gean_0 not running, or finalization stalled after having advanced |
| UNKNOWN | 2 | gean_0 runs a different image, or gean never logged finalization (run longer) |

Setup errors also exit 1. The report also lists blocks proposed, ERROR lines,
and (with `--with-sync-test`) `batched fetch starting`, `queueing missing
block`, and `fetch exhausted for root` counts from gean's logs.

## Reading gean Logs

gean colors its output and puts a reset code between `[component]` and the
message, so raw greps such as `"\[validator\] proposed block"` never match.
Strip ANSI first:

```bash
strip() { sed -E 's/\x1b\[[0-9;]*m//g'; }

docker logs gean_0 2>&1 | strip | grep "\[forkchoice\] finalized advanced" | tail -1
docker logs gean_0 2>&1 | strip | grep "Connected Peers:" | tail -1
docker logs gean_0 2>&1 | strip | grep -c "\[validator\] proposed block"
docker logs gean_0 2>&1 | strip | grep -c " ERROR "
docker logs gean_0 2>&1 | strip | grep -oE "proc_time=[^ ]+" | sort | uniq -c
docker logs gean_0 2>&1 | strip | grep -c "has_parent=false"
docker logs gean_0 2>&1 | strip | grep "\[sync\]"
docker logs gean_0 2>&1 | strip | grep "\[signature\] aggregate:" | head
```

Cross-client finalization:

```bash
for node in zeam_0 ream_0 lantern_0 ethlambda_0 gean_0; do
    echo "$node: $(docker logs "$node" 2>&1 | strip | grep -i finalized | tail -1)"
done
```

For a full analysis use the devnet-log-review skill
(`.agents/skills/devnet-log-review/scripts/analyze-logs.sh`).

## Manual Workflow

```bash
BRANCH=$(git symbolic-ref --short HEAD)
TAG=$(printf '%s' "$BRANCH" | tr -c 'A-Za-z0-9_.-' '-')
docker build --build-arg GIT_COMMIT=$(git rev-parse HEAD) \
    --build-arg GIT_BRANCH="$BRANCH" -t "gean:$TAG" .
# Set node_docker's image in lean-quickstart/client-cmds/gean-cmd.sh to gean:$TAG
cd lean-quickstart && NETWORK_DIR=local-devnet ./spin-node.sh --node all --cleanData --generateGenesis --metrics
```

Before starting, confirm no container already uses the node names
(`docker ps -a`); only remove containers you started.

## Troubleshooting

- **A node name already exists:** another devnet may own it. Ask the user
  before removing it, or rerun after it is gone.
- **"a previous run is recorded":** run `cleanup.sh`.
- **UNKNOWN, gean_0 on a different image:** check `grep node_docker=
  lean-quickstart/client-cmds/gean-cmd.sh` during the run; the image
  replacement assumes a `…gean:<tag>` reference in that file (unverified
  against the current lean-quickstart).
- **Build fails:** check `docker info` on the server.

## References

- [devnet-runner](../devnet-runner/SKILL.md) — devnet management, clients reference
- [devnet-log-review](../devnet-log-review/SKILL.md) — log analysis
