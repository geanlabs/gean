---
name: gean-perf
description: Stress-test gean under the Shadow simulator. Use when the user wants to run gean on Shadow (single-client, multi-client, or multi-subnet), sweep the XMSS prover-cost rates, or find the load at which gean stops keeping up with the slot budget. For symptoms on a live devnet use devnet-triage; for saved devnet logs use devnet-log-review.
---

# gean performance under Shadow

Find the prover cost at which gean falls behind the 4s slot (5 x 800ms intervals, from
`internal/types/constants.go`) and report which metric breaks first.

## Prerequisites

This skill drives two sibling repositories that are not part of gean:

- `../shadow`: the Shadow Simulation Mastery Guide (metrics chapter `guide/11-reading-the-metrics.md`,
  runbook chapters `guide/07`-`10`, cheatsheet `guide/13-reference-cheatsheet.md`).
- `../lean-shadow-fuzzer`: the runner. It reads a `config.toml`, generates genesis, topology, and
  `shadow.yaml`, runs Shadow, and renders the results.

Check both paths first. If either is missing, stop and tell the user; do not improvise a harness.

gean itself ships only the prover-cost model (`internal/shadow`), the `--shadow-xmss-*` rate flags
(`cmd/gean/flags.go`), and the `ghcr.io/geanlabs/gean:shadow` image (`make docker-build`).

## Workflow

1. **Frame the question against the budget.** Aggregation must finish by the interval-4 boundary
   (`aggregationDeadlineOffset` in `internal/node/tick.go`). An interval-2 dispatch has about 1600ms; an
   early dispatch from `maybeEarlyAggregate` in interval 1 has more. Proposals run off the tick loop.
2. **Run the matrix** with `lean-shadow-fuzzer`: nodes, committee count, images, and prover-cost rates.
   Shadow finds the breaking rate deterministically. For absolute magnitudes on real proving, use a
   devnet on the user's server (devnet-runner skill).
3. **Read the failure chain** in this order: `lean_proving_duration_seconds{aggregation}` rises,
   `lean_aggregation_worker_total_time_seconds` approaches the deadline, sessions truncate,
   `lean_aggregation_dispatch_dropped_total` climbs and `lean_proving_queue_depth` pins,
   `lean_tick_interval_duration_seconds` drifts past 0.8s, and `lean_head_slot` minus
   `lean_latest_finalized_slot` widens.
4. **Justify changes with measurements.** Propose a performance change only with evidence: a metric
   crossing its budget in the matrix, a reproducible benchmark, or a profile. Report the rates, topology,
   revision, and results.

Report each run as keeps up / falls behind / insufficient evidence, naming the first metric to break.
