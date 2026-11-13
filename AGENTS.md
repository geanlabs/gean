# AGENTS.md

Guidance for AI coding agents working in this repository.

## Project Overview

Gean is an Ethereum lean consensus client written in Go, tracking the Python leanSpec. Do not mistake
it for Beacon Chain (Eth2) clients.

## Principles

Simplicity and minimalism are our core values: code should be easy to read and understand.

- Protocol correctness and safety take precedence over simplification. Among correct solutions, prefer
  the simplest to understand and review.
- Subtract before adding. When several solutions work, prefer in this order: one that removes code; then
  one that adds code without new exported API or touching existing code; last, one that modifies existing
  code. Removing code removes failure points, and additive changes rarely regress existing behavior. This
  ranks designs only: fix a bug in existing code in place, don't wrap it. Delete code a change leaves unused.
- Search before adding. Inspect existing helpers, types, interfaces, and callers; reuse only where the
  semantics fit, rather than forcing unrelated behavior together because it looks similar.
- No over-engineering: no single-implementation interfaces, generics or reflection without need, helpers
  used once, speculative flags, or validation that cannot fire. Limits stay package-level `const`s.
- Replace, don't accumulate: remove the old path when changing behavior. Leave no shims, dead branches,
  commented-out alternatives, unused options, or speculative TODOs.
- Keep state and control flow explicit. Avoid storing cheaply derived values without a reason and a clear
  invalidation strategy. Make mutation, resource ownership, and asynchronous work visible at call sites.
- Every new background task needs an owner, a cancellation path, and a defined completion policy.
  Before closing storage or freeing native resources, account for all tasks that may still use them.
- **No fallbacks.** Gean is pre-mainnet and runs on resettable devnets, so there is nothing to stay
  backward compatible with. Do not add fallback implementations, legacy code paths, compatibility shims,
  data migrations, or retries that mask a failure. Fail loudly and fix the cause. Not fallbacks:
  leanSpec-defined paths such as interval-2 aggregation, and bounded retries of transient failures where
  repeating is safe and exhaustion returns the error (e.g. `lean/internal/network/checkpoint`, peer retries).
- No defensive filler: no nil checks or defaults for impossible cases; never swallow errors.
- Prove a case is unreachable before removing its guard. Trace input boundaries, callers, and state
  construction; do not assume an invariant from a type or a successful lookup alone.
- Validate untrusted lengths, indices, encodings, and resource limits before allocation, mutation, or
  FFI calls. Boundary validation is not defensive filler.
- Comments describe the code, not the change, and do not restate it; history belongs in the commit message.
- No unrequested docs, logs, or metrics. Keep errors terse (no "failed to") and log or return, not both.
- When a change makes existing documentation, examples, or command instructions incorrect, update the
  affected material as part of that change.
- Test only the behavior the change adds or fixes, with the fewest cases that pin it down. No tests for
  trivial code (getters, constructors, constants) or the standard library, and do not retest the SSZ
  generator itself; keep tests of gean's serialization contracts, roots, and malformed input. No
  permutations that exercise the same path; no tests that only exercise mocks or chase coverage.
- Tests should distinguish correct behavior from a plausible defect, not repeat implementation logic.
  Regression tests must fail without the fix. Do not weaken assertions to make tests pass or use
  arbitrary sleeps for synchronization.
- Avoid duplication in tests. Before adding a test, check whether an existing table can take another case.
  Tests that differ only in inputs and expected outputs belong in one table-driven test with `t.Run`; move
  shared setup and assertions into helpers such as `helpers_test.go`.
- Preserve domain names and direct control flow; code that mirrors leanSpec structure is not duplication.
  Before combining similar protocol steps, check their ordering, validation, and failure semantics against
  leanSpec.
- Judge simplicity by the concepts a reader must hold, not by line count.
- Keep changes focused; do not touch unrelated code.
- Before handoff, justify every new abstraction, dependency, stored field, option, and fallback: what
  concrete requirement needs it, and what becomes simpler or safer because it exists?

## Agent change discipline

- Before a nontrivial edit, identify the required behavior, owning component, invariants, existing
  implementation/helpers, and covering tests. Use the Principles above as the design standard.
- Keep each change to one behavior or one ownership extraction. Keep unrelated fixes, dependency
  upgrades, public API changes, and protocol changes in separate changes.
- Every new package, abstraction, export, dependency, option, stored field, or worker needs a current
  caller and a concrete requirement. Explain these briefly in the handoff; no separate design file
  is needed for routine changes. Planned Beacon support alone does not justify generic scaffolding.
- After editing, inspect the full diff and search for old callers, duplicate implementations, and
  code made unused by the change. Remove the obsolete path in the same change. Do not add placeholder
  uses, empty implementations, or tests whose only purpose is to keep dead code reachable.
- Run `make check-architecture` and `make check-go`, plus the behavior-specific checks below. The
  architecture check is source-only and needs no native build; build FFI before `check-go`.
  Report failures and skips accurately. A skipped check is not a passing check.
- Use `.agents/skills/lean-review/SKILL.md` before handoff for nontrivial code changes. Its review is
  report-only; fixes follow the task's authorization and the skill's finding-specific rules.
- Do not weaken architecture rules, disable CI jobs, add lint suppressions, remove covering tests,
  or add allowlists to make a change pass. Policy changes require an explicit policy-changing task
  and a stated reason. This applies to this file and review instructions too.
- Static analysis cannot prove that exported APIs have real consumers or that similar protocol
  steps are interchangeable. Review those claims with callers, specifications, and ownership in view.

## Commands

```bash
make build
make fmt
make lint
make check-architecture
make check-go
make test
make test-ffi
make test-spec
go test ./lean/internal/chain -run TestName -v -count=1
```

## Architecture

The layout, its boundary rules and the remaining work are in [docs/architecture.md](docs/architecture.md).
The packages below describe the current implementation; migrate them in bounded, verified steps.

- `cmd/gean/`, `cmd/keygen/`: flag parsing and signals; testnet and key generation.
- `lean/`: everything Lean. `lean/consensus/` holds the pure rules (types, genesis, statetransition,
  forkchoice); `lean/crypto/xmss/` the XMSS bindings over Rust FFI crates; `lean/node/` builds, runs
  and stops the node; `lean/spectests/` runs leanSpec fixtures.
- `lean/internal/`: Lean node services. `chain/` is the engine loop (tick-driven duties in `tick.go`,
  event dispatch, block import, pending blocks); `validator/` attestation production, proposer
  selection and the proposal reservation; `store/` the consensus store; `network/` libp2p, gossip,
  req/resp, sync and checkpoint sync; `aggregation/`, `prover/`, `blockbuilder/`, `api/`.
- `internal/`: mechanics shared by every protocol (storage, lifecycle, logger, metrics); no protocol types.
- `beacon/`: reserved Beacon Chain layout, README files only until Beacon work starts.

## Testing

- Add regression coverage in existing package tests.
- Reuse test helpers (`helpers_test.go`) and isolate test databases and directories with `t.TempDir()`.
- Run affected packages first; broaden checks for changes across packages.
- Use existing benchmarks and metrics for performance changes and report measured results.
- Report the checks actually run, their results, and material skips or limitations. Missing fixtures,
  mocked cryptography, and unrun checks do not establish correctness for those paths.

## Commit and PR Style

Never run `git add`, `git commit`, or `git push` without the user's explicit prior approval.
Ask separately before each action.

Use conventional commits and PR titles: `type(scope): description`, e.g.
`fix(node): reserve proposal duties through signing and acceptance`. Keep subjects under 72 characters.
Explain what changed and why in one short paragraph. Fill in `.github/pull_request_template.md`; PRs to
`main` or `devnet-N` must link an issue. Include real measurements for performance claims.
Name branches `type/short-description` with the same types, e.g. `perf/jemalloc-allocator`.

Confirm with the user before force-pushing. When merging or rebasing, do not silently drop changes; ask
when the right resolution of a conflict is unclear.

## Notes

- **FFI**: Run `make ffi` before direct `go test`/`go vet`; `make test` and `make lint` do not build it.
- **Testing**: Run `make test-ffi` for `lean/crypto/xmss/` changes, `make test-spec` for consensus or SSZ changes, and
  `go test -race` for concurrency changes. Use fuzz tests for SSZ and wire decoding.
- **Spec**: leanSpec pinned at `LEAN_SPEC_COMMIT_HASH` in the Makefile is the source of truth; do not bump it silently.
- **Tick loop**: Keep proving and blocking FFI work off `Engine.onTick`; hand it to a worker. Preserve the
  head-update/proposal ordering.
- **Aggregation**: Interval 2 is the fallback; `maybeEarlyAggregate` may start it in interval 1 at quorum.
  It runs once per slot and is deliberately not behind the sync-lag duty gate.
- **Fork choice**: `ForkChoice.Prune` must remap vote indices together with pruning nodes.
- **Storage**: For `lean/internal/store` or `internal/storage` changes, check write ordering and what a crash
  mid-write leaves on restart.
- **Logging and metrics**: Use `internal/logger` component constants with `key=value` fields and `0x%x`
  roots. Add metrics in `internal/metrics` with the `lean_` prefix.
- **Generated code**: Do not edit `lean/consensus/types/*_encoding.go`; run `make sszgen`.
  When SSZ definitions change, inspect the regenerated diff and verify that a second run produces no
  further changes. Keep generator upgrades separate unless required by the task.
- **Build info**: `gitCommit` is injected via `-ldflags`; do not hardcode it.
- **Local runs**: `make run`, `run-node1`, and `run-node2` delete `data/nodeN` first.
- **Dependencies**: Run `make tidy` for Go changes; Rust builds use `--locked`.
  Add, remove, or upgrade dependencies only when required by the task. Inspect module and lockfile
  changes; avoid unrelated version churn.
- **Skills**: Task procedures live in `.agents/skills/`; read only those relevant to the task. Run
  `lean-review` before merge to find what can come out. Create and edit skills only there;
  `.claude/skills/<name>` is a relative symlink (`../../.agents/skills/<name>`), never a copy.

## Code Style

- Follow existing patterns.
- Explain non-obvious behavior, spec references, and safety requirements in comments, not PR history.
- Do not name or cite other clients in comments; a short leanSpec function reference is fine.
- Files use LF and end with a newline.
- Never expose secrets, node keys, or XMSS key material.

### Go

- `gofmt` all files. Keep package names short and lowercase.
- Group imports as stdlib, third-party, then `github.com/geanlabs/gean/...`, separated by blank lines.
- Start doc comments on exported identifiers with the identifier's name.
- Wrap errors with lowercase context: `fmt.Errorf("parse config.yaml: %w", err)`.
- Use package-level `Err*` sentinels for errors callers branch on; check them with `errors.Is`/`errors.As`.
- Do not `panic` outside tests; return an error.
- Put `ctx context.Context` first and propagate cancellation to workers.
- Put a mutex directly above the fields it guards and name it after them (`validatorKeysMu`).
- Tests use plain `testing` (`t.Fatalf`, `t.Helper()`), no assertion libraries.

### Rust

- `cargo fmt`; Clippy runs with `-D warnings`.
- Export FFI functions as `#[no_mangle] pub unsafe extern "C"` and wrap bodies that can panic in `ffi_guard!`.
- Free owned native objects exactly once through their matching `*_free`. Never free borrowed pointers
  (e.g. `hashsig_keypair_get_public_key`); keep their owner alive throughout use.
