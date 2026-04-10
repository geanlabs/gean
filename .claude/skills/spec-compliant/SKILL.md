---
name: spec-compliant
description: Check gean's compliance with leanSpec changes since a devnet version. Use when checking if gean implements all spec changes, is spec-compliant, or needs to catch up to the spec.
---

# /spec-compliant - Spec Compliance Check

Check whether gean implements all leanSpec changes between two devnet versions (or
from a version to HEAD). Produces a structured compliance report with what's done,
what's missing, and what needs updating.

## Usage

- `/spec-compliant devnet3` - Check compliance for changes from devnet 3 to HEAD
- `/spec-compliant devnet2 devnet3` - Check compliance for changes from devnet 2 to devnet 3
- `/spec-compliant devnet0 devnet3` - Full compliance check across multiple devnets

## Steps

### 1. Parse arguments and validate versions

Accept one or two arguments:
- **One argument** (e.g., `devnet3`): check compliance from that version to HEAD
- **Two arguments** (e.g., `devnet2 devnet3`): check compliance from first to second

Check that `leanSpec/` exists inside the gean repo root. If it does not exist, tell the
user: "leanSpec is not cloned. Run `make leanSpec` to clone it, then try again." and
abort. Do NOT attempt to clone it yourself.

Read `leanSpec/VERSIONS.md` to resolve each version name to a commit hash. Match
flexibly and case-insensitively:
- `devnet3`, `Devnet 3`, `d3` should all resolve to the same entry

If any version is not found, list available versions from VERSIONS.md and stop.

### 2. Check LEAN_SPEC_COMMIT_HASH currency

Read gean's `Makefile` and extract the `LEAN_SPEC_COMMIT_HASH` value.

Compare it against:
- The commit hash for the **to-version** (the second argument, or HEAD if one argument)
- The current leanSpec HEAD (`git -C leanSpec rev-parse HEAD`)

Record the pin status for the report:
- **Up to date**: hash matches the to-version (or HEAD if one argument)
- **Pinned to from-version**: hash matches the from-version but not the to-version
- **Behind**: hash does not match either (report how many commits behind)

### 3. Run /spec-diff on leanSpec

Change working directory to the `leanSpec/` directory inside the gean repo and invoke `/spec-diff`
with the same arguments the user provided:

- One argument: `/spec-diff <version>` (compares version to HEAD)
- Two arguments: `/spec-diff <from-version> <to-version>` (compares between versions)

This produces a structured changelog grouped by component, with each item tagged
`[New]`, `[Modified]`, or `[Removed]`, plus a Test Vectors section and Summary.

Capture the full spec-diff output. Then return to the gean directory.

### 4. Cross-reference each spec change against gean

Use the mapping table in `.claude/skills/spec-compliant/references/spec-gean-mapping.md`
to locate where each spec concept lives in gean's codebase.

For **each item** in the spec-diff output:

#### Type/Container changes

Search `types/*.go` files (skip `*_encoding.go` which are generated).

- `[New]` type: grep for a Go struct with the matching name (apply name transforms:
  Python `PascalCase` usually maps directly to Go `PascalCase`). If not found, mark ❌.
- `[Modified]` type: find the Go struct, read its definition, and check whether the
  described change is reflected (e.g., new field added, field type changed, field removed).
  If the struct exists but the change is missing, mark ⚠️.
- `[Removed]` type: search for the old type name. If still present, mark ⚠️ (stale code).
  If already gone, mark ✅.

#### Function/Logic changes

Identify the target gean package from the mapping table, then search for the function.
Apply Python-to-Go name transformation (`snake_case` -> `PascalCase`).

- `[New]` function: search the mapped package for a function with matching or equivalent
  name. If not found, mark ❌.
- `[Modified]` function: find the function, read its signature and body, determine if
  the described change (new parameter, changed logic, renamed) is reflected. If the
  function exists but the change is missing, mark ⚠️.
- `[Removed]` function: check if the old function still exists. If gone, ✅. If still
  present, ⚠️.

#### Constant changes

Search `types/constants.go`. Python `UPPER_SNAKE_CASE` maps to Go `PascalCase`
(e.g., `SECONDS_PER_SLOT` -> `SecondsPerSlot`).

#### Test vector changes

For each test category in the spec-diff Test Vectors section:

1. Check the corresponding gean test file exists (see mapping table):
   - `state_transition` -> `spectests/stf_test.go`
   - `fork_choice` (fc) -> `spectests/forkchoice_test.go`
   - `verify_signatures` -> `spectests/signatures_test.go`
   - `ssz` -> check `spectests/` for SSZ tests

2. Check fixture parser (`spectests/fixture.go`) for new/modified fields in fixtures.
   If the spec added a new field to a test fixture type and gean's parser doesn't
   handle it, mark ⚠️.

3. If `LEAN_SPEC_COMMIT_HASH` is behind the target version, note that regenerating
   fixtures (`make leanSpec/fixtures`) is required to pick up new test vectors.

### 5. Produce compliance report

Output the report in this format:

```markdown
## Spec Compliance: gean vs [From Version] -> [To Version or HEAD]

### Spec Pin
- gean LEAN_SPEC_COMMIT_HASH: `<hash>`
- From ([From Version]): `<hash>`
- To ([To Version or HEAD]): `<hash>`
- Status: [Up to date | Pinned to from-version | Behind by N commits]

### [Component Name]

- ✅ `ItemName` -- implemented in `package/file.go:line`
- ❌ `ItemName` -- **missing**: [what needs to be added]
- ⚠️ `ItemName` -- **needs update**: [what's different]

### [Next Component]

...

### Test Vectors

**[Category]** (N fixture files changed)
- ✅ / ❌ / ⚠️  per relevant check

### Summary

| Status | Count |
|--------|-------|
| ✅ Implemented | N |
| ❌ Missing | N |
| ⚠️ Needs Update | N |

**Key gaps:**
- Bulleted list of most important missing/outdated items

**Recommended next steps:**
1. Numbered action items (e.g., "Update LEAN_SPEC_COMMIT_HASH to `<hash>`")
2. ...
```

Guidelines:
- Group by the same component names used in the spec-diff output
- For each ✅ item, include the gean file path and line number where it's implemented
- For each ❌ item, describe what specifically needs to be added
- For each ⚠️ item, describe the gap between spec and gean
- End with actionable next steps ordered by priority
- If LEAN_SPEC_COMMIT_HASH is behind, the first next step should always be updating it
- If SSZ types changed, remind to run `make sszgen` after updating types
