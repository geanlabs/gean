# gean Claude Code Skills

This directory contains three skills for managing and testing gean in
multi-client devnets.

## Quick reference

| Skill | What it does | When to use |
|---|---|---|
| [`devnet-log-review`](devnet-log-review/SKILL.md) | Analyze logs from a devnet run (errors, blocks, consensus progress) | A devnet finished and you want to know what happened |
| [`devnet-runner`](devnet-runner/SKILL.md) | Start/stop the multi-client devnet, dump logs, manage configs | You want to run the devnet for a fixed time |
| [`test-pr-devnet`](test-pr-devnet/SKILL.md) | Build gean from your current branch and test it against other clients | You're working on a PR and want a quick safety check |

## Easiest way to use them: Make targets

The gean Makefile exposes each skill as a `make` target. From the gean repo
root:

```bash
make devnet-test          # Build current branch + run 5-client devnet test (most common)
make devnet-test-sync     # Same as devnet-test but also tests sync recovery
make devnet-status        # Check what's running right now
make devnet-cleanup       # Stop everything, restore configs, AND delete dumped logs
make devnet-run           # Just run the devnet for 120s and dump logs
make devnet-analyze       # Analyze .log files in the current directory
make devnet-clean-logs    # Just delete the dumped *.log files (without stopping devnet)
```

`make help` lists them all with descriptions.

## Direct script invocation (if you prefer)

```bash
.claude/skills/test-pr-devnet/scripts/test-branch.sh
.claude/skills/test-pr-devnet/scripts/test-branch.sh --with-sync-test
.claude/skills/test-pr-devnet/scripts/check-status.sh
.claude/skills/test-pr-devnet/scripts/cleanup.sh
.claude/skills/devnet-runner/scripts/run-devnet-with-timeout.sh 120
.claude/skills/devnet-log-review/scripts/analyze-logs.sh /path/to/logs
```

## The two main workflows

### Workflow A — Test your current PR

You're working on a branch and want to know "does this still work with the
other clients?"

```bash
make devnet-test
# Wait ~90 seconds...
# Look for "✓ PASSED" at the end
make devnet-cleanup
```

### Workflow B — Investigate a colleague's incident report

A teammate sends you a folder of `.log` files and says "the network stalled,
was it our fault?"

```bash
.claude/skills/devnet-log-review/scripts/analyze-logs.sh /path/to/their/logs
# Look at the Errors and Consensus Progress tables
# If gean has 0 errors and the same finalized slot as the majority, gean is clean.
```

## What gets checked automatically

The `devnet-test` workflow watches for the two regressions we care about most:

| Check | Threshold | Why it matters |
|---|---|---|
| Max attestations per block | > 30 → ⚠ regression | Block bloat bug (caused ~12 MB blocks before commit `62454aa`) |
| `MessageTooLarge` / `exceeds max` errors | > 0 → ✗ FAILED | Same regression, observed from the receiving side |

If either fires, the test exits with `✗ FAILED` so you can't accidentally push
a regression.

## Skill structure

Each skill follows the same layout:

```
.claude/skills/<skill-name>/
├── SKILL.md             ← The full guide; read this for the skill
├── scripts/             ← Bash helpers (called by Make targets above)
└── references/          ← Deep-dive docs (load when investigating)
```

You don't need to read the references unless you're hunting a specific kind of
bug. Open them on demand:

- `devnet-log-review/references/FORK_ANALYSIS.md` — when clients disagree on blocks
- `devnet-log-review/references/FINALIZATION_DEBUG.md` — when finalization stalls
- `devnet-log-review/references/ERROR_CLASSIFICATION.md` — when you can't tell if an error is real or noise
- `devnet-log-review/references/CLIENT_LOG_PATTERNS.md` — when you don't recognize a log line
- `devnet-runner/references/clients.md` — default images, ports, known issues per client
- `devnet-runner/references/validator-config.md` — full schema for `validator-config.yaml`
- `devnet-runner/references/long-lived-devnet.md` — running a persistent devnet with rolling restarts

## Default 5-client setup

The skills assume a 5-client devnet:

| Client | Language | Role |
|---|---|---|
| zeam | Zig | Peer validator. Watch for SSZ panics. |
| ream | Rust | Peer validator. Sync recovery is fragile. |
| lantern | C | Peer validator. Most reliable client. |
| ethlambda | Rust | Peer validator. Best fork-choice tree visualization. |
| gean | Go | The system under test. |

qlean is intentionally excluded — it has been historically unreliable
(`listen_addrs=0` config bug, frequent disconnects, no log shipping).
