# Finding categories

Each category lists what it looks like, the proof required, and when the same
code is not a finding.

## 1. `dead-code`

**Looks like:** an exported symbol with no callers (gean is not imported by
other projects); a helper whose call site was removed; a field nothing reads;
an unused import; a branch that cannot run.

**Proof:** `grep -rn '<symbol>' --include='*.go'` finds only the definition
(and its own test). For a branch, the unreachability proof in SKILL.md step 3.

**Not a finding:** the symbol satisfies an interface, or is used via
reflection, build tags, or codegen; a fixture-parser field.

## 2. `premature-abstraction`

**Looks like:** a helper called once; an interface with one implementation;
a wrapper whose methods only delegate; a generic with one instantiation; a
constructor that only builds a literal; a speculative flag or option.

**Proof:** one call site or implementation, and the layer adds no validation,
transformation, or behavior. "A future implementation or policy may need it"
does not justify it.

**Not a finding:** a test swaps the implementation through the interface; the
helper names a leanSpec function the code mirrors.

## 3. `defensive-bloat`

**Looks like:** a nil check, bounds check, or default for a case the callers
already rule out; `if err != nil` on a call whose every return path returns
`nil`; re-validating SSZ fields already validated on decode; `recover` around
code that cannot panic.

**Proof:** the unreachability proof in SKILL.md step 3: trace input boundaries,
every caller, and state construction. A lookup or a type alone is not proof.

**Not a finding:** validation of untrusted input — gossip and req/resp from
peers, `internal/api/` requests, CLI flags and config, checkpoint-sync
responses. Boundary validation is required, not filler.

## 4. `duplicates-existing-util`

The highest-value category; reviewers miss these most.

**Looks like:**
- Bitlist handling that could call `internal/types/bitlist.go` (`BitlistGet`,
  `BitlistSet`, `BitlistLen`, `BitlistCount`, `BitlistExtend`, `BitlistIndices`,
  `BitlistFromIndices`, `NewBitlistSSZ`).
- Root and proposer helpers duplicating `internal/types/helpers.go`
  (`IsZeroRoot`, `ProposerIndex`, `IsProposer`, `ShortRoot`).
- Log formatting that bypasses `logger.Info/Warn/Error` with a component
  constant (`logger.Node`, `logger.Chain`, …) from `internal/logger`.
- Storage key construction that bypasses `internal/storage/keys.go`
  (`EncodeLiveChainKey`, `DecodeLiveChainKey`).
- A hand-written or hand-edited SSZ codec. Always a finding: delete it, edit
  the struct and tags, run `make sszgen`. If `make sszgen` fails, report the
  error; never suggest patching the file by hand.

**Proof:** name the existing utility with `file:line` and show the semantics
match, not just the shape.

**Not a finding:** a specialized hot-path version backed by a benchmark; the
existing utility is only in a test build.

## 5. `comment-bloat`

**Looks like:** a doc comment restating the signature; change history ("added
for PR #142", "leanSpec PR #708 requires this", "used by the X flow") that
belongs in the commit message; a TODO with no tracking issue; a comment
narrating the next few lines; commented-out code; section banners; a comment
naming or copied from another client.

**Proof:** deleting it loses nothing a future reader needs. History can be
reworded to the *why* it implies ("gate X before Y because Z").

**Not a finding:** a comment explaining *why* (ordering constraint, safety
requirement, non-obvious behavior); a short leanSpec function-name reference
such as "mirrors `process_attestations`".

## 6. `over-validated-boundary`

**Looks like:** `internal/statetransition/` re-checking fields of a block the
engine already verified; `internal/forkchoice/` re-validating a root just read
from the store; a storage backend validating a key built by
`internal/storage/keys.go`.

**Proof:** every caller is internal and the value comes from a trusted source,
with no external boundary in between.

**Not a finding:** any caller passes untrusted input, including the hive test
driver (`internal/api/testdriver/`); then the check is the boundary.

## Cross-cutting tests

- **Would a reader notice it is gone?** If removing it leaves a gap in the
  logic, it was not bloat.
- **What does it protect against?** For each check or wrapper you would
  remove, name the bug it catches. If there is a real one, it stays.
- **Higher bar on consensus code.** Saving 5 lines of fork-choice vote
  accounting is not worth a small chance of a fork.
