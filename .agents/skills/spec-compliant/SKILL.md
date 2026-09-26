---
name: spec-compliant
description: Check gean's compliance with leanSpec changes between two named leanSpec targets (devnet versions or a fetched commit). Use when checking if gean implements all spec changes, is spec-compliant, or needs to catch up to the spec.
---

# /spec-compliant - Spec Compliance Check

Check whether gean implements the leanSpec changes between two **named** targets and report,
per item, how strong the evidence is.

## Usage

- `/spec-compliant devnet2 devnet3` - changes from devnet 2 to devnet 3
- `/spec-compliant devnet3 origin/main` - changes from devnet 3 to a fetched upstream commit

A single argument is ambiguous: `make test-spec` checks leanSpec out at the pin, so local
`HEAD` is the pin and "compare to HEAD" always looks up to date. Ask the user for the target
(a devnet version, `origin/main` after they approve `git -C leanSpec fetch origin main`, or a
hash). Never use local `HEAD` as an implicit target.

## Steps

### 1. Resolve targets

If `leanSpec/.git` is missing, tell the user to run `make leanSpec/.git` (clone) or
`make test-spec` (clone, check out the pin, generate fixtures), and stop. Do not clone it yourself.

Resolve devnet names via `leanSpec/VERSIONS.md` (case-insensitive: `devnet3`, `Devnet 3`, `d3`).
If a name is not listed — VERSIONS.md often lags newer devnets — report it as an **unknown
version**, list the listed ones, and ask for a hash. Never guess a hash from branch names,
tags or dates.

### 2. Record the pin

Read `LEAN_SPEC_COMMIT_HASH` from gean's `Makefile`. Report its relation to each named target
(`git -C leanSpec merge-base --is-ancestor`, `git -C leanSpec rev-list --count`): equal, behind
by N, ahead, or diverged. "Up to date" is only ever relative to a named target.

If the pin is behind the to-target, ask whether it is deliberately held (e.g. matching a running
devnet) or an upgrade is intended. Never bump it silently.

### 3. Get the spec diff

Read `leanSpec/.claude/skills/spec-diff/SKILL.md` and follow its steps directly inside
`leanSpec/` with the resolved hashes. It yields items grouped by component, tagged `[New]`,
`[Modified]` or `[Removed]`, plus changed test vectors. Spec code lives under
`leanSpec/src/lean_spec/spec/` (forks in `spec/forks/<fork>/`, containers in
`spec/forks/<fork>/containers/`); fixtures under `leanSpec/tests/consensus/<fork>/`.

### 4. Locate each item in gean

gean mirrors spec names: `snake_case` functions become `PascalCase`/`camelCase`, containers keep
their name, `UPPER_SNAKE` constants become `PascalCase`. Grep the whole tree for the name
(`grep -rn --include='*.go' -i 'ProcessBlock\|process_block' internal/ xmss/`), skipping
generated `internal/types/*_encoding.go`. If the name is absent, grep for distinctive logic
before declaring it missing.

Starting points (see `ls internal/`): `internal/types` (containers, constants),
`internal/statetransition`, `internal/forkchoice`, `internal/store` (consensus store),
`internal/aggregation`, `internal/p2p`, `internal/spectests` (fixture runners, build tag
`spectests`) with parsers in `internal/specfixtures`.

### 5. Grade the evidence

Give every item exactly one level:

| Level | Meaning |
|---|---|
| **missing** | Searched; no counterpart found (say what was searched). |
| **located** | Counterpart found by name; not yet compared with the spec. |
| **inspected** | Code read and matches the described change (cite `file:line`); not proven by a test. |
| **tested** | A named test or fixture that exercises the change passed in this session. |
| **unknown** | Could not determine (e.g. spec intent unclear, fixtures not generated). |

Code presence is never compliance. `[Modified]` items need at least **inspected**; a
behavioral claim needs **tested**. A `[Removed]` item still present in gean is a gap.
New fixture fields need a matching parser change in `internal/specfixtures`; fixtures only
exist for the pin, so vectors beyond it stay **unknown** until the pin moves and
`make test-spec` passes.

### 6. Report

```markdown
## Spec Compliance: gean vs <from> -> <to>

### Spec Pin
- LEAN_SPEC_COMMIT_HASH: `<hash>` — <equal to | behind by N | ahead of | diverged from> <to> (`<hash>`)
- Pin intent: <held deliberately | upgrade intended | not asked yet>

### <Component>
- **inspected** `Item` — `internal/pkg/file.go:line`, matches <change>
- **missing** `Item` — searched <names/paths>; needs <what>
- ...

### Test Vectors
- <category>: <level> — <test name and result, or why unknown>

### Summary
| Level | Count |
|---|---|

**Gaps and next steps:** ordered by protocol risk. Remind to run `make sszgen` if SSZ types change.
```

Report only checks actually run. If `make test-spec` was not run this session, nothing is **tested**.
