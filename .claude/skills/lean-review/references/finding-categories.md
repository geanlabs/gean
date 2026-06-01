# Finding categories — examples and counter-examples

Six categories. Each entry shows what the pattern looks like, what proof you
need to call it a finding, and what would make the same code *not* a finding.

---

## 1. `dead-code`

Code that has no callers, no readers, or runs only on a branch that can't be
reached.

**Looks like:**
- Exported function with no callers in the repo and no consumers (gean is
  not imported by other Go projects, so unused exports are dead).
- Unexported helper whose call site was removed in an earlier commit but the
  helper survived.
- Variable assigned in a struct that nothing reads.
- `if err != nil` after a call that the type system guarantees never returns
  an error (e.g. SSZ encode on a fixed-size primitive).
- Imported package whose symbols aren't used.

**Proof required:**
- `grep -r '<symbol>' --include='*.go'` returns only the definition and (for
  test-only callers) its `_test.go`.
- For an unreachable branch: trace the control flow above the branch and
  show why the condition can't fire.

**Not a finding when:**
- Symbol is part of a public interface implementation (used via interface
  dispatch, won't show in grep).
- Function is referenced by reflection, build tags, or codegen.
- It's a fixture parser field — fixture types must mirror JSON shape even
  if gean doesn't use every field.

---

## 2. `premature-abstraction`

A layer that adds no behavior beyond what's already at the call site, and
exists "just in case" the layer becomes useful later.

**Looks like:**
- Helper called exactly once, whose body is shorter than its call site would
  be if inlined.
- Interface with a single concrete implementation, where no test ever swaps
  it out.
- Wrapper struct whose only method delegates to the wrapped value.
- Generic function instantiated with a single type.
- Factory function that just calls `&Foo{...}` with the same arguments the
  caller already had.
- A `Builder` pattern for a struct with 1-3 fields.

**Proof required:**
- Single call site (`grep` for the function name).
- Wrapper body adds no validation, no logging, no transformation.
- No test uses the abstraction as a seam (`grep` test files for mock
  implementations).

**Not a finding when:**
- The wrapper marks an intentional **policy insertion point** — e.g. all
  signing flows through a thin wrapper so a future signature-policy hook
  has one place to land. Tag as `consensus-critical`, report-only.
- Interface has one impl today but a second is planned per a tracked
  leanSpec issue (check `references/spec-gean-mapping.md`).
- Helper is one call site but **clarifies intent** at the call site
  (`validate(...)` reads more clearly than 8 inline lines of checks). This
  is a judgment call; prefer to leave it.

---

## 3. `defensive-bloat`

Validation, nil-checks, or error handling for cases the caller already
guarantees can't happen.

**Looks like:**
- `if x == nil { return ErrNil }` at the top of a private function whose
  only callers pass `&Foo{}` literals.
- Bounds-checking a slice index after the function above already
  bounds-checked.
- `if err != nil` on a function the type system says returns no error.
- Re-validating SSZ fields that were already validated on decode.
- Sanity-checking an enum value whose only set sites assign valid values.
- Try/recover around code that can't panic.

**Proof required:**
- Trace all callers and show the precondition is established.
- For "can't return error": read the called function and show all return
  paths return `nil`.

**Not a finding when:**
- The function is exported and called from `cmd/` or `internal/api/` (external
  boundary — validate).
- The function is called from gossip handlers in `internal/p2p/` (external boundary —
  validate). Gossip messages come from untrusted peers; defensive checks
  there are not bloat.
- The check exists to make a future invariant violation **loud** (panics
  with a clear message rather than corrupting state silently). Tag
  `consensus-critical`, report-only.

---

## 4. `duplicates-existing-util`

New code reimplements something that already lives in the repo. This is the
single highest-value finding type — humans miss these most often, especially
in AI-generated code.

**Looks like:**
- Bitlist manipulation in `internal/node/` that could call `internal/types/bitlist.go`
  (`BitlistGet`, `BitlistSet`, `BitlistLen`, `BitlistExtend`,
  `NewBitlistSSZ`).
- Hex encoding/decoding helpers that duplicate `internal/types/util.go`.
- Custom logger formatting that should go through `logger.Info/Warn/Error`.
- A "merge two sorted slices" function when `sort.Slice` + iteration would
  be one line.
- A retry loop when `context.WithTimeout` does the job.
- A new SSZ encoder by hand when `make sszgen` would produce one.
- Manual storage key construction when `internal/storage/keys.go` has an encoder
  (`EncodeLiveChainKey` etc.).

**Proof required:**
- Name the existing utility with file:line.
- Show the signatures match (or trivially adapt).
- Verify the existing utility is reachable from the new code's package
  (not behind a build tag, not internal/).

**Not a finding when:**
- The "duplicate" is intentionally specialized for a hot path and the
  generic version allocates (rare — confirm with a benchmark first).
- The existing utility lives in a different test build (e.g. `internal/spectests/`
  with build tag `spectests`).

---

## 5. `comment-bloat`

Comments that don't help a future reader.

**Looks like:**
- Doc comments restating the function signature: `// GetBlock returns the
  block` on `func GetBlock(...) *Block`.
- "added for PR #142" / "used by the X flow" / "see issue #99" — PR/commit
  context that belongs in the commit message, not the code.
- TODOs older than ~6 months with no tracking issue.
- Multi-line block comments describing what the next 3 lines do.
- Commented-out code blocks.
- Section banners (`// ====== Helpers ======`).
- Docstring-style comments on private helpers.

**Proof required:**
- The comment can be deleted without losing information a future reader
  needs.

**Not a finding when:**
- The comment explains *why*, not *what* — e.g. "this must run before X
  because Y," "leanSpec PR #708 requires this gate," "wall-clock recovery
  needed or gossip rejects all attestations as future." Per the repo
  CLAUDE-style guidance, *why* comments stay; *what* comments go.
- It's a `// SPDX-License-Identifier` or similar required header.

---

## 6. `over-validated-boundary`

Treating an internal caller's input with the same suspicion you'd give a
gossip peer or an HTTP request.

**Looks like:**
- A `internal/statetransition/` function that re-checks fields the engine populated
  from a verified block.
- A `internal/forkchoice/` function that re-validates a root the engine just looked
  up from the store.
- A storage backend that validates the structure of a key constructed by
  `internal/storage/keys.go`.

**Proof required:**
- Show the caller is internal and the value comes from a trusted source
  (store lookup, just-verified block, etc.).
- Show no external boundary sits between the trusted source and this call.

**Not a finding when:**
- The function is also called from a test driver or `internal/api/test_driver.go`
  (`HIVE_LEAN_TEST_DRIVER=1` path) where input may be adversarial.
- The function is called from both internal trusted paths and external
  untrusted paths — leave the validation; it's the boundary.

---

## Cross-cutting rules

**The "would a reviewer notice this is gone?" test.** If you removed the
finding and asked a future contributor to read the file, would they notice
something missing or feel a gap in the logic? If yes, it wasn't bloat —
leave it.

**The "concept count" test.** A finding is real when removing it reduces the
number of named things (functions, types, branches) the reader has to
understand. Reducing LOC by collapsing two lines into one doesn't count.

**The "what does this protect against?" test.** For every defensive check or
wrapper you propose removing, write one sentence answering "what bug does
this exist to catch?" If you can't answer, it's likely bloat. If the answer
is "a real bug that happened or could happen in this code path," it stays.

**Bias toward leaving consensus code alone.** When a finding is in
`internal/statetransition/`, `internal/forkchoice/`, `internal/node/`, or `internal/types/`, the bar is higher.
A "saved" 5 LOC in fork-choice vote accounting is not worth a 1% chance of
introducing a fork.
