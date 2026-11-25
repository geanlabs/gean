---
name: spec-review
description: Check a consensus change against the pinned consensus-specs, marker by marker - nothing the spec requires missing, nothing it doesn't require added, every rule tested. Use on every pull request that touches consensus code, or when asked to check a change against the spec.
---

# Spec review

Check that a change implements the pinned spec. Review the change as the
[`review`](../review/SKILL.md) skill's Phase 1 gathers it, and report in its format; this skill
adds the spec side. Report only findings you score at least 80 out of 100: real, and tied to
quoted spec text and a `file:line`. A pre-existing issue, one in code the change didn't touch, or
a deviation `docs/spec_deviations.md` already lists on the base branch scores 0; an entry the
change adds is reviewed like the code it excuses.

## 1. Get the spec at the pin

The pin is `CONSENSUS_SPECS_COMMIT` in `pins.env`. Read the spec at that commit, never a branch
head, which moves ahead of the devnets. In a consensus-specs clone, check for the commit with
`git cat-file -e <commit>^{commit}` and `git fetch origin <commit>` only if it is missing; then
use `git show <commit>:<path>` and `git grep <commit>`. Without a clone, read
`https://raw.githubusercontent.com/ethereum/consensus-specs/<commit>/<path>`.

A fork has its own directory (PQ: `specs/_features/pq/`) and marks what it changes with
`[New in <Fork>]` and `[Modified in <Fork>]`. Its spec states only those changes, so for a
function it doesn't restate, read the latest earlier fork that defines it
(`git grep -n 'def <name>(' <commit> -- specs`), not every fork.

## 2. List what the spec requires

Get the change and the spec's markers in parallel. For the sections the change touches, list:

- the markers and the headings that start with New, Modified or Removed
  (`git grep -nE '\[(New|Modified) in <Fork>\]|^#+ (New|Modified|Removed)' <commit> -- <fork dir>`),
  counted yourself rather than taken from a plan or an earlier review;
- the bodies of the spec functions the change implements, and the constants, presets and config
  values they read.

## 3. Check the code against it

Map both ways. A spec item with no code and no stated reason to leave it out is a gap; code with
no spec item behind it is over-implementation, unless it is plumbing the spec leaves to clients
(storage, networking, metrics). For each function:

- **Name and link.** Named after the spec function, with a link at the pinned commit above it, as
  `.agents/rules/consensus.md` asks.
- **Constants.** Each value equals the spec's exactly: domains, preset values, leaf offsets,
  depths, limits. A literal where the spec names a constant is a finding.
- **Asserts.** Every spec `assert` has a branch that rejects.
- **Order and inputs.** Hash and concatenation inputs are in the spec's order, and each read sees
  the state the spec reads, before or after the write the spec makes.
- **Arithmetic.** Loop bounds, integer division, rounding direction and `uint64` overflow match.
- **Tests.** Each rule, and each `assert`'s failing case, is covered by a test or a spec
  reference-test vector; find them with `git grep -ln '<FuncName>'` rather than reading every
  test.

## 4. Report

Use `review`'s report format and its severity headings, scored this way:

- **Critical:** wrong state, or an invalid block, attestation or proof accepted.
- **Major:** a spec violation someone can trigger.
- **Minor:** required behaviour with no test.
- **Nits:** a deviation that doesn't affect consensus.

Critical and Major findings are fixed before the change merges. Each finding quotes the spec and
gives its confidence. Start the report with coverage:

```
### Coverage (consensus-specs <short commit>)
<n> markers and <n> headings in the touched sections: <n> implemented, <n> out of scope (each
with its reason), <n> missing.

### Critical
- **(95)** `internal/transition/pq/randao.go:42`. Spec (`specs/_features/pq/beacon-chain.md`,
  `process_randao`):
  > mix = sha256(prev_randao + body.randao_reveal[0] + DOMAIN_RANDAO)
  <what the code does instead>. <the failure it causes>. Check: <a concrete way to confirm it>.
```
