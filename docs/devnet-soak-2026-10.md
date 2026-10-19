# Devnet soak, 9–15 October 2026

Three back-to-back gean runs on one multi-client devnet, about 6 days in total. Numbers come from the devnet's Prometheus (30-day retention) and from the saved gean logs; the queries are listed at the end.

## Setup

One QEMU/KVM host: 16 cores, 31 GB RAM, Ubuntu 26.04, kernel 7.0.0-38 for all three runs. Node data lives on a 984 GB volume. Four gean nodes in every run, all aggregators; leanSpec pinned at `eca701e`.

| run | gean | network | from (UTC) | to (UTC) | length | how it ended |
|---|---|---|---|---|---|---|
| 1 | `44dedd7` (BLAKE2s line) | 4 gean, 6 ethlambda, 4 lantern | 9 Oct 21:53 | 13 Oct 11:33 | 85 h | stopped for the next build |
| 2 | `8fdf252` (run 1 + #485) | same 14 nodes | 13 Oct 11:33 | 15 Oct 15:41 | 52 h | finality collapse, host reboot |
| 3 | `8fdf252` | 4 gean, 4 ethlambda, 4 lantern | 15 Oct 16:43 | still running | 5 h at writing | — |

Run 3 uses `ghcr.io/lambdaclass/ethlambda:devnet5-leanvm-b` and `bitminemavan/lantern:devnet5-leanvm-b-txready`. The other clients' versions for runs 1 and 2 were not recorded. None of the runs used plain `main`.

## Summary

| | run 1 | run 2 | run 3 |
|---|---|---|---|
| gean process restarts | 0 | 0 | 0 |
| gean panics in logs | 0 | logs not kept | — |
| wedges | 0 | 1 (gean_2, kernel fault) | 0 |
| median gean RSS | 237–256 MB | 248–383 MB | 230–351 MB |
| RSS trend | +1 to +8 MB/day | +2 to +53 MB/day | too short |
| `signed_blocks` growth (largest node) | +2.95 GB/day | +3.09 GB/day | +3.45 GB/day |
| `states` table at end | 0.07 GB | 1.48 GB | 0.00 GB |
| finality gap: median / p95 / max (slots) | 12 / 32 / 87 | 11 / 45 / 1962 | 10 / 20 / 34 |
| time with the gap over 40 slots | 1.9% | 5.8% | 0.0% |
| stalls of 10 minutes or more | 0 | 1 | 0 |
| gean time within 4 slots of the best head | 92.0–99.4% | 91.6–99.4% | 95.7–100% |
| peers per gean node (min–max) | 12–13 | 10–13 | 10–11 |

## Memory

No leak shows over 85 hours of run 1: the fitted RSS trend is +1 to +8 MB a day on a median of about 250 MB. RSS peaks at 1–2.3 GB during proofs and falls back, because the prover uses the system allocator by default (`--prover-arena` is off). Run 2's steeper trends, +34 to +53 MB a day on three nodes, come from its last hours, when the finality collapse left work queued; its medians stayed at 248–383 MB.

## Storage

- **Block proofs grow without bound.** `signed_blocks` grew about 3 GB a day in every run and reached 10.5 GB on the largest node after run 1's 85 hours. Headers, bodies and the live chain stayed near 0.01 GB, so the proofs are almost all of it. Filed as #444; fixed by #493, which drops proofs of finalized blocks older than a day.
- **States pile up during a finality stall.** `states` sat at 0.03 GB while finality was healthy and rose to 1.48 GB in the two hours of run 2's stall (13:40–15:40 on 15 Oct). Filed as #489; fixed by #494, which keeps 2,048 slots of states during a stall.
- The data volume had 673 GB free at the start of run 2 and 624 GB at its end.

## Finality and recovery

Runs 1 and 3 never stalled for long. Run 1's gap crossed 40 slots 77 times, never for 10 minutes. Run 3's gap never crossed 40 slots.

Run 2 stalled at 13:32 on 15 Oct and had not recovered by the reboot at 15:41: the gap peaked at 1,962 slots and justification stopped at slot 45005. In that window:
- ethlambda nodes skipped attestations while syncing;
- lantern nodes alternated between synced and syncing, and two were OOM-killed (15:27 and 15:34);
- host load ran between 22 and 80.

The stall began 2 h 16 min after gean_2 wedged (below), and gean_2 was one of the four aggregators. This report does not establish whether the wedge caused the collapse.

gean's head kept pace through the stall: no gean node fell more than 22 slots behind the best head in run 2, and none more than 15 in run 1.

## Restarts, crashes and wedges

- **No gean process restarted or panicked.** The run 1 logs, about 1.9 million lines per node, hold 40 ERROR lines in total, all `proposal prover unavailable`.
- **One wedge, gean_2 in run 2.**
  - At 11:16:41 on 15 Oct, one second after it began proposing slot 42940, the kernel took a general protection fault in `folio_lruvec_lock_irqsave` while the leanVM merge freed memory.
  - The merge thread stayed in uninterruptible sleep and the proving gate stayed held, so gean_2 stopped proposing and aggregating while it kept logging and serving metrics.
  - Its metrics stopped at 12:35, when a core dump was taken for evidence, and it never answered again before the reboot.
  - The same fault, at the same instruction, hit ethlambda_4's prover thread on 8 Oct on the older kernel. That points to a kernel bug that leanVM's memory frees trigger, not to client code.
  - See #430, and #495 for `lean_proving_held_seconds`, which makes such a wedge visible.
- Shutdown ordering (#482) was not exercised: no gean node was stopped during the runs.

## Catch-up

On 15 Oct gean_3 was paused for 14.5 minutes in run 3 to test #378. It came back 220 slots behind and reached the head in about 11 s, with no fetch-queue saturation. The range was fetched from 3 peers at once, the duplicate download noted in #488.

## Not covered

- None of the runs used plain `main`, and the fixes in #491–#495 were not running. A later run should show `lean_table_bytes` staying bounded with #493 and #494.
- Run 2's container logs were not kept, only its metrics and the gean_2 dumps.
- Restart under load and a finality stall on a gean-only network were not tested.

## Queries

All against the devnet Prometheus, with `job=~"gean_.*"` for gean nodes:

- Memory: `process_resident_memory_bytes`
- Storage: `max by (table) (lean_table_bytes{job=~"gean_.*"})`
- Finality gap: `max(lean_head_slot) - max(lean_latest_finalized_slot)`
- Head lag per node: `max(lean_head_slot) - on() group_right lean_head_slot{job=~"gean_.*"}`
- Peers: `lean_connected_peers`
- Availability and restarts: `up`, `process_start_time_seconds`
- Data volume: `node_filesystem_avail_bytes{mountpoint="/var/lib/gean"}`
