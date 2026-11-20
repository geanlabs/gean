---
name: precheck
description: Run gean's pre-commit checks (gofmt, lint, build, and the tests for the packages a change touches) before committing. Use before every commit, and when asked to check a change.
---

# Pre-commit checks

Run these from the repository root, in order. Stop at the first failure, fix it, and run the
failing step again. Report what ran and what passed.

1. **Changed files.** Everything the commit would contain, staged or not, plus new files:

   ```bash
   { git diff --name-only HEAD; git ls-files --others --exclude-standard; } | sort -u
   ```

2. **Formatting.** If the next command lists any files, run `make fmt` and check again:

   ```bash
   gofmt -l $(git ls-files '*.go')
   ```

3. **Lint.** Formatting aside, import-rule violations and Rust `fmt`/`clippy` failures fail it; read
   the other golangci-lint findings and fix those in lines you changed:

   ```bash
   make lint
   ```

4. **Build:**

   ```bash
   make build
   ```

5. **Tests for the changed packages.** Turn the changed `.go` files into packages and test them:

   ```bash
   pkgs=$({ git diff --name-only HEAD -- '*.go'; git ls-files --others --exclude-standard -- '*.go'; } |
     xargs -r -n1 dirname | sort -u | while read -r d; do [ -d "$d" ] && echo "./$d"; done)
   if [ -n "$pkgs" ]; then go test $pkgs -count=1; else echo "no Go packages changed"; fi
   ```

   Then add what the change touches:

   | Changed | Also run |
   |---|---|
   | `internal/lean/transition`, `forkchoice`, `types` or `api` | `make lean-spectest` |
   | `internal/lean/node`, `importer`, `syncer`, `aggregation`, `p2p`, `internal/core/p2p` or `lean.Run` | `make lean-e2e` |
   | `internal/core/crypto/xmss` or its Rust code | `make lean-test-ffi` (slow) |
   | Anything shared by many packages | `make test` |

6. **Leftovers.** No references to plans, phases or the session in the change:

   ```bash
   git diff HEAD | grep -nE '^\+.*([Pp]hase [0-9]+|plan §)' || echo clean
   ```
