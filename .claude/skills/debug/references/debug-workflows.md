# Debug Workflows

Use these workflows after classifying the failing surface.

## 1. Existing devnet logs

Use this path when `.log` files already exist or a previous run has been
captured.

```bash
./.claude/skills/devnet-log-review/scripts/analyze-logs.sh
./.claude/skills/devnet-log-review/scripts/show-errors.sh -n gean_0 -w
```

Then:

1. Identify the first slot where gean diverges from peers.
2. Confirm whether the divergence is:
   - a rejected incoming block
   - a bad locally produced block
   - missing attestations / aggregation
   - peer connectivity / sync failure
3. Follow the slot into the relevant code path:
   - incoming block: `node/store_block.go`, `node/store_validate.go`
   - local production: `node/store_build.go`, `node/store_produce.go`
   - head/finalization mismatch: `forkchoice/`
   - transport mismatch: `p2p/`

Do not diagnose from the last error line if the first divergence is earlier.

## 2. Fresh multi-client reproduction

Use this path when no good logs exist yet.

1. Use the `devnet-runner` skill for a clean reproduction.
2. If the issue is branch-specific or PR-specific, use `test-pr-devnet`.
3. Capture logs and return immediately to the existing-log workflow.

Prefer a short run that reproduces the symptom over a long noisy run.

## 3. gean-only Go test failure

Start with the narrowest failing target:

```bash
make test
go test ./node -run TestName -v -count=1
go test ./forkchoice -run TestName -v -count=1
go test ./statetransition -run TestName -v -count=1
```

Then:

1. Record the exact failing assertion, panic, or mismatch.
2. Search for the error string or invariant with `rg`.
3. Read the nearest production code before scanning unrelated packages.
4. If the failure depends on leanSpec fixtures, switch to the spec workflow.

## 4. Spec fixture mismatch

Use this when `make test-spec` fails or a consensus rule appears wrong.

```bash
make test-spec
go test ./spectests/ -count=1 -tags=spectests -run TestName -v
```

Then:

1. Identify whether the mismatch is in state transition, forkchoice, or
   signature verification.
2. Compare the failing fixture type to the target package:
   - STF fixture -> `statetransition/`
   - forkchoice fixture -> `forkchoice/`
   - signature fixture -> `xmss/`, `node/verify.go`
3. Treat the fixture as deterministic and investigate logic, encoding, or
   proof handling before trying a devnet reproduction.

## 5. XMSS / Rust FFI failure

Use this when FFI tests fail, signatures are invalid, or Rust panics appear.

```bash
make test-ffi
go test ./xmss -run TestName -v -count=1
```

Then isolate whether the bug is:

- Go-side encoding or pointer/lifetime handling in `xmss/ffi.go`
- key/proof preparation in `xmss/keys.go` or `xmss/proof_pool.go`
- Rust glue/build/runtime behavior in `xmss/rust/`

Avoid editing consensus logic until the signature path is proven sound.

## 6. Checkpoint sync or restart behavior

Use this when a node restarts incorrectly, syncs to the wrong finalized state,
or behaves differently after persistence.

```bash
curl http://127.0.0.1:5052/lean/v0/health
curl http://127.0.0.1:5052/lean/v0/checkpoints/justified
curl http://127.0.0.1:5052/lean/v0/states/finalized -o /tmp/finalized.ssz
```

Then:

1. Verify the source node is healthy and exposes the expected finalized state.
2. Confirm startup flags and node role before assuming corrupted storage.
3. Inspect `checkpoint/checkpoint.go`, `api/server.go`, `storage/`, and
   `node/node.go`.
4. Only wipe data or restart from genesis when the user wants a fresh-state
   reproduction.

## Root-cause standard

Do not stop at “the test failed in package X.” A good debug result ends with:

- the smallest known failing surface
- the likely subsystem
- the concrete evidence
- the next verification step
