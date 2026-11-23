---
paths:
  - ".github/**"
---

# GitHub workflows

- **Checks.** `actionlint` and `zizmor` run on every change under `.github/` and must pass; zizmor
  covers permissions, persisted credentials and template injection.
- **Pinned actions.** Every `uses:` pins a full commit SHA with the version in a comment
  (`actions/checkout@<sha> # v7.0.1`).
- **Least privilege.** The workflow sets `permissions: {}`; each job grants only what it needs.
- **Bounded jobs.** Every job sets `timeout-minutes`.
- **Leave workflow logic alone.** Don't change triggers, jobs, steps or matrices unless the task
  asks for it.
