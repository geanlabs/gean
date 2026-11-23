# AGENTS.md

Guidance for AI coding agents working in gean. `CLAUDE.md` is a symlink to this file.

## Project

gean is an Ethereum consensus client in Go: one binary, a subcommand per chain. `gean lean` runs
Lean Ethereum consensus (post-quantum XMSS signatures, leanVM aggregation); `gean beacon`, for the
Ethereum Beacon Chain, is planned. It runs on devnets only.

```
cmd/gean/              CLI: subcommands and flags
internal/core/         chain-agnostic: clock, crypto/xmss (Go + Rust FFI), httpapi, lifecycle,
                       logger, p2p, pending, protoarray, storage
internal/lean/         Lean Ethereum: types, transition, forkchoice, store, importer, aggregation,
                       syncer, p2p, api, metrics, keys, node (the engine), and lean.Run (node startup)
testing/spectest/      leanSpec fixture tests (build tag spectests)
testing/sim/           multi-node tests in one process on an in-memory network
```

## Commands

```bash
make build            # Rust XMSS library, then bin/gean
make test             # unit tests
make lint             # go vet, golangci-lint, cargo fmt and clippy
make fmt              # gofmt and cargo fmt
make lean-e2e         # three nodes finalize; a restarted node catches up (fake XMSS)
make lean-spectest    # leanSpec fixtures (a pinned download on first run)
go test ./internal/lean/forkchoice/ -run TestName -count=1   # one test
```

## Rules

- **Correctness first, then the reviewer's time.** Keep diffs minimal and focused: no drive-by
  renames, reformatting or comment rewrites in code you aren't changing.
- **Reduce rather than add.** When several changes would work, prefer the one that removes code,
  then one that adds code without touching existing code, and only then one that rewrites it.
  Deleted code can't break. If a change leaves code unused, delete that too.
- **leanSpec is the reference.** A divergence from the pinned leanSpec is a bug, even if nothing
  crashes. When a spec test fails, check the fixtures and the spec before changing the client.
- **`internal/core` never imports a chain**, and `lean` and `beacon` never import each other.
  `.golangci.yml` enforces this; `make lint` fails on a violation.
- **No backward compatibility.** gean is devnet-only: remove old flags, formats and code paths
  instead of keeping aliases, default subcommands or fallbacks.
- **Every bug fix comes with a regression test**, written like the tests around it.
- **Comments explain why**, not what. No references to plans, phases or the current session in code,
  comments, commit messages or config.
- **Generated code:** don't edit `internal/lean/types/*_encoding.go`; change the struct tags and run
  `make lean-sszgen`.
- **Concurrency:** the lean engine is a single-writer dispatch loop. State it owns is touched only
  on that goroutine; other goroutines reach it through channels and hooks.
- **XMSS is expensive.** Real keygen takes tens of seconds per key and proving takes gigabytes.
  Multi-node tests use `-tags fakecrypto`; don't add real-crypto work to `make test`.

## Commits and pull requests

- Before every commit, run the [`simplify`](.agents/skills/simplify/SKILL.md) skill (skip it when
  only Markdown changed), then the [`precheck`](.agents/skills/precheck/SKILL.md) skill.
- Commit and PR titles are `<type>: <subject>` with a lowercase subject: `feat`, `fix`, `perf`,
  `refactor`, `test`, `docs`, `build`, `deps` or `chore`; the `titles` workflow checks them. The
  body says what changed and why.
- Commits must be signed.
- When reviewing, comment; never block a PR with "request changes".

## Skills

Skills live in `.agents/skills/<name>/SKILL.md`. `.claude/skills/<name>` is a relative symlink to
each one (`../../.agents/skills/<name>`), never a copy.

- [`precheck`](.agents/skills/precheck/SKILL.md): format, lint, build and test the changed packages
  before a commit.
- [`review`](.agents/skills/review/SKILL.md): a deep review of a change or pull request.
- [`recheck`](.agents/skills/recheck/SKILL.md): re-check an earlier answer or review, claim by
  claim, and correct it.
- [`simplify`](.agents/skills/simplify/SKILL.md): clean up a change: reuse, over-engineering,
  unused code, test bloat and AI slop, then apply the fixes.
- [`fix-spectest`](.agents/skills/fix-spectest/SKILL.md): debug and fix failing leanSpec fixture
  tests.
- [`devnet-runner`](.agents/skills/devnet-runner/SKILL.md): run a local multi-client lean devnet with
  this build through lean-quickstart, collect the logs, test node failures.
- [`devnet-log-review`](.agents/skills/devnet-log-review/SKILL.md): analyze a devnet run's logs:
  errors, block production, forks, finality.
- [`split-for-review`](.agents/skills/split-for-review/SKILL.md): split a large uncommitted change
  into small, independently passing units, staged one at a time.
