# Contributing to Gean

Thanks for your interest in Gean, a Lean Ethereum consensus client written in Go. This guide covers how
to set up, make a change, and get it merged.

Security vulnerabilities must not be reported in public issues. Follow [SECURITY.md](SECURITY.md) instead.

## Before you start

- Open or find an issue describing the bug or feature. PRs targeting `main` or a `devnet-N` branch must
  link an issue.
- For larger changes, discuss the approach in the issue before writing code.
- The pinned [leanSpec](https://github.com/leanEthereum/leanSpec) revision (`LEAN_SPEC_COMMIT_HASH` in
  the [`Makefile`](Makefile)) is the source of truth for protocol behavior. Divergence from it is a bug
  even when nothing crashes. Do not bump the pin as part of an unrelated change.

## Development setup

Install the prerequisites listed in the [README](README.md#prerequisites) (Go, Rust, uv, Docker), then:

```sh
make build       # Build the Rust FFI and Go binaries
make test        # Go unit tests
make test-ffi    # XMSS FFI tests
make test-spec   # Consensus spec fixture tests
make lint        # Go and Rust linters
make fmt         # Format Go and Rust code
```

Run `make help` for all targets. `make test` and `make lint` do not build the FFI; run `make ffi` before
invoking `go test` or `go vet` directly.

## Making a change

Simplicity and minimalism are Gean's core values. Reviewability is treated as a consensus-safety
property, so code should be easy to read and audit.

- **Correctness first.** Among correct solutions, prefer the simplest to understand and review.
- **Subtract before adding.** Prefer a change that removes code over one that adds code, and delete code
  your change leaves unused. Reuse existing helpers where the semantics fit.
- **No over-engineering.** No single-implementation interfaces, speculative flags, helpers used once, or
  validation that cannot fire.
- **No fallbacks.** Gean is pre-mainnet and runs on resettable devnets. Do not add legacy paths,
  compatibility shims, data migrations, or retries that mask failures. Fail loudly and fix the cause.
- **Validate untrusted input** (lengths, indices, encodings, resource limits) before allocation,
  mutation, or FFI calls.
- **Keep changes focused.** Do not touch unrelated code, and update any docs your change makes incorrect.

### Go style

- `gofmt` all files. Group imports as stdlib, third-party, then `github.com/geanlabs/gean/...`.
- Wrap errors with lowercase context, without "failed to": `fmt.Errorf("parse config.yaml: %w", err)`.
  Log or return an error, not both.
- Use package-level `Err*` sentinels for errors callers branch on.
- Do not `panic` outside tests. Pass `ctx context.Context` first and propagate cancellation.
- Do not edit generated `internal/types/*_encoding.go` files; run `make sszgen`.

### Rust style (`xmss/rust/`)

- `cargo fmt`; Clippy runs with `-D warnings`.
- Wrap FFI function bodies that can panic in `ffi_guard!`, and free owned native objects exactly once.

## Testing

- Test the behavior your change adds or fixes, with the fewest cases that pin it down. Regression tests
  must fail without the fix.
- Prefer adding a case to an existing table-driven test over writing a new test. Use plain `testing`, no
  assertion libraries, and `t.TempDir()` for databases and files.
- Run `make test-spec` for consensus or SSZ changes, `make test-ffi` for `xmss/` changes, and
  `go test -race` for concurrency changes.
- For performance changes, include real measurements from benchmarks or metrics.

## Commits and pull requests

- Use [conventional commits](https://www.conventionalcommits.org/) for commit messages and PR titles:
  `type(scope): description`, e.g. `fix(node): reserve proposal duties through signing and acceptance`.
  Keep the subject under 72 characters.
- Name branches `type/short-description`, e.g. `perf/jemalloc-allocator`.
- Fill in the [pull request template](.github/pull_request_template.md): a short description of what
  changed and why, the linked issue, and your test plan.
- Make sure `make lint` and the relevant tests pass before requesting review.

## License

By contributing, you agree that your contributions are licensed under the [MIT License](LICENSE).
