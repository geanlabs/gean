# AGENTS.md

Guidance for AI coding agents working in gean. `CLAUDE.md` is a symlink to this file.

## Project

gean is an Ethereum consensus client in Go, becoming a client for the post-quantum beacon chain
(XMSS signatures, leanVM aggregation). It has no runnable node yet and runs on devnets only.

```
cmd/gean/              CLI: subcommands and flags
internal/core/         chain-agnostic: clock, crypto/xmss (Go + Rust FFI), httpapi, lifecycle,
                       logger, p2p, pending, protoarray, storage
internal/crypto/       stark, vrf, keys, kzg
internal/types/        config, presets, the fork table, containers
internal/state/        the beacon state
internal/transition/   slot, epoch and block processing
internal/forkchoice/   fork choice over core/protoarray
internal/store/        the chain's tables on core/storage
internal/execution/    the Engine API client
internal/da/           data availability: PeerDAS columns and sampling
internal/importer/     block, envelope and column import
internal/p2p/          topics, protocols and message handling
internal/syncer/       range and by-root sync
internal/aggregation/  attestation proofs: proving and merging
internal/api/          the Beacon API
internal/node/         wires the node together; only cmd/gean imports it
testing/spectest/      consensus-spec reference tests
testing/sim/           multi-node tests in one process on core/p2p/memnet
```

A package appears with its first code. Where a package's rules differ by fork, each fork is a
subpackage (`transition/pq`) and the parent package holds what forks share; the fork table in
`types` picks the fork for an epoch.

## Commands

```bash
make build            # bin/gean
make test             # unit tests
make lint             # go vet, golangci-lint, cargo fmt and clippy
make fmt              # gofmt and cargo fmt
make test-ffi         # the XMSS FFI tests (slow)
make loc              # lines of code per package; BASE=origin/pq compares
go test ./internal/core/protoarray/ -run TestName -count=1   # one test
```

## Rules

- **Correctness first, then the reviewer's time.** Keep diffs minimal and focused: no drive-by
  renames, reformatting or comment rewrites in code you aren't changing.
- **Reduce rather than add.** When several changes would work, prefer the one that removes code,
  then one that adds code without touching existing code, and only then one that rewrites it.
  Deleted code can't break. If a change leaves code unused, delete that too.
- **The spec is the reference.** A divergence from the pinned consensus spec (`pins.env`) is a
  bug, even if nothing crashes. When a spec test fails, check the fixtures and the spec before
  changing the client.
- **`internal/core` holds no chain-specific logic** and imports nothing from gean outside core.
  Above it, `types` and `crypto` import only core and each other, and `state`, `transition` and
  `forkchoice` add only each other. `.golangci.yml` enforces the imports; `make lint` fails on a
  violation.
- **Only `cmd/gean` starts `internal/node`.** Nothing else under `internal/` imports it; tests
  under `testing/` may.
- **No backward compatibility.** gean is devnet-only: remove old flags, formats and code paths
  instead of keeping aliases, default subcommands or fallbacks.
- **Every bug fix comes with a regression test**, written like the tests around it.
- **Comments explain why**, not what. No references to plans, phases or the current session in code,
  comments, commit messages or config.

## Area rules

Rules for each area live in `.agents/rules/`; read the ones a task touches.
`.claude/rules/<name>.md` is a relative symlink to each, never a copy, loaded by its `paths:`
frontmatter.

- [`consensus`](.agents/rules/consensus.md): state, transition, fork choice, import, aggregation,
  store.
- [`ssz-types`](.agents/rules/ssz-types.md): `types`: containers that match the spec.
- [`concurrency`](.agents/rules/concurrency.md): the node's dispatch loop, workers, goroutines.
- [`p2p`](.agents/rules/p2p.md): networking, the API and hostile peer input.
- [`crypto-ffi`](.agents/rules/crypto-ffi.md): XMSS and the Rust FFI.
- [`storage`](.agents/rules/storage.md): Pebble and the chain store.
- [`testing`](.agents/rules/testing.md): test style and build tags.
- [`ci-workflows`](.agents/rules/ci-workflows.md): `.github` workflows.

## Commits and pull requests

- Before every commit, run the [`prep-commit`](.agents/skills/prep-commit/SKILL.md) skill instead of
  the built-in `/simplify` (skip it when only Markdown changed).
- Commit and PR titles are `<type>: <subject>` with a lowercase subject: `feat`, `fix`, `perf`,
  `refactor`, `test`, `docs`, `build`, `deps` or `chore`; the `titles` workflow checks them. The
  body says what changed and why.
- Commits must be signed.
- When reviewing, comment; never block a PR with "request changes".

## Skills

Skills live in `.agents/skills/<name>/SKILL.md`. `.claude/skills/<name>` is a relative symlink to
each one (`../../.agents/skills/<name>`), never a copy.

- [`prep-commit`](.agents/skills/prep-commit/SKILL.md): before a commit, report cleanups (reuse,
  over-engineering, unused code, test bloat, AI slop, Go idioms), apply the ones approved, then
  format, lint, build and test the change.
- [`review`](.agents/skills/review/SKILL.md): a deep review of a change or pull request.
- [`spec-review`](.agents/skills/spec-review/SKILL.md): check a consensus change against the pinned
  consensus-specs, marker by marker.
- [`verify-report`](.agents/skills/verify-report/SKILL.md): re-check an earlier answer or review,
  claim by claim, and correct it.
- [`split-into-commits`](.agents/skills/split-into-commits/SKILL.md): split a large uncommitted
  change into small, independently passing units, staged one at a time.
