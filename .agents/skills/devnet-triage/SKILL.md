---
name: devnet-triage
description: Diagnose a live, running lean devnet from its Prometheus and container logs, then report findings with proposed fixes and stop for approval. Use when the user reports a symptom on a running devnet — finality or justification lagging, the tick loop drifting, memory growing, aggregation producing nothing, nodes flapping sync status — or asks what a live metric reading means. Not for saved logs of a finished run (use devnet-log-review) or Shadow stress tests (use gean-perf).
---

# Devnet triage

Diagnose a **running** devnet by reading metrics and logs, then hand the user findings and a
proposed fix. Sibling skills cover other phases: `devnet-runner` starts a devnet,
`devnet-log-review` analyses a finished run's saved logs, `gean-perf` is Shadow stress testing only.

## The contract

1. Pick the bundle for the symptom. Emit it **whole** — one paste, one output.
2. Read the output. State what it rules **in** and what it rules **out**.
3. Propose a fix with the evidence behind it.
4. **Stop.** No code changes, no commits, no node restarts, no config edits until the user says go.

Read-only by default. Never stop, restart, or reconfigure a node, never touch the aggregator
toggle, never run `--stop`, `--cleanData` or `--generateGenesis`, without explicit approval for
that specific action. Approval for one action is not approval for the next.

## Connection

Run every command through the SSH alias:

```bash
ssh gean-devnet '<command>'
```

The alias resolves via the user's `~/.ssh/config`, which holds the host, user and key. **Never write
host, user, key path or IP into this file or any committed file, and never print them back.** The
alias name alone is meaningless without that config, which is why it is safe here.

Check it before anything else:

```bash
ssh gean-devnet 'hostname; docker ps --format "{{.Names}}" | wc -l'
```

If that fails — the alias is unconfigured, or the laptop is off the tailnet — do not guess a host.
Fall back to emitting each bundle for the user to paste, and read their output. The bundles are
identical either way.

Container names vary per devnet; list them with `docker ps --format '{{.Names}}'`. Bundles use
`$c` for the gean container under inspection — set it, or loop over every gean container.

gean logs carry ANSI colour codes, with a reset between `[component]` and the message, so a
grep spanning them misses. Strip before every grep:

```bash
docker logs "$c" 2>&1 | sed -E 's/\x1b\[[0-9;]*m//g' | grep ...
```

Note the shell nesting: bundles below are written to run *on the host*. Wrap them in
`ssh gean-devnet '...'` and keep inner quoting single-level, or write the bundle to a heredoc.

Prometheus is reachable on the host; each node's metrics port comes from its own container args:

```bash
docker inspect "$c" --format '{{join .Args " "}}' | tr ' ' '\n' | grep -A1 -- '--metrics-port' | tail -1
```

Do not assume a port; they differ per node.

## Traps — read this before interpreting anything

These cost real time to rediscover.

**Histograms that cannot see their own tail.** `lean_proving_duration_seconds` tops out at **8 s**
and `lean_pq_sig_aggregated_signatures_building_time_seconds` at **4 s**. A p95/p99 sitting exactly
on those numbers means "at least that", not "that". Logs carry the real value; the metric does not.

**Counters with only one possible label value.** gean increments
`lean_finalizations_total{result="success"}` and nothing else — there is no error path. A dashboard
"Finalizations – Errors" panel therefore contains **no gean series at all**, which looks like a
clean record and is not one.

**Cross-client metric semantics can differ.** The same name may count different things (attempts
vs advances), be left unpopulated, or use different buckets. Read both clients' definitions before
comparing any metric across clients.

**`max_over_time` on a short window reads as a trend and is not one.** A gap that oscillates 1 → 169
and returns produces a rising `max` if you sample two windows. Always pair it with `min_over_time`
and `avg_over_time` before calling anything a trend.

**Means dominated by no-op samples.** `lean_proving_duration_seconds` records whole sessions,
including those that prove nothing, so its low quantiles read far below a real proof. Use a high
quantile, or filter, or read the logs.

**Host saturation invalidates every latency number.** Check CPU before trusting any timing:

```bash
docker stats --no-stream --format '{{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}'
uptime
```

Sum the CPU column against core count. Above ~100% of the machine, every latency figure is
contended and the honest statement is "at most this fast".

**gean is usually the only aggregator.** Its verification and import paths compete with its own
proving; other clients verify on idle processes. A gean-vs-other latency gap may be self-contention
rather than a slower code path. Separating them needs a gean node with aggregation off.

## Bundles

### Health snapshot — always start here

```bash
ssh gean-devnet 'docker ps --format "{{.Names}}\t{{.Status}}" | sort
for c in $(docker ps --format "{{.Names}}" | grep "^gean"); do echo "$c restarts=$(docker inspect $c --format "{{.RestartCount}}")"; done
nproc; uptime; free -g | head -2
docker stats --no-stream --format "{{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}" | sort -k2 -hr
vmstat 1 3 | tail -2
curl -sS -G localhost:9090/api/v1/query --data-urlencode "query=lean_current_slot" | head -c 300'
```

Non-zero `RestartCount` on any node invalidates the run — a crash loop behind
`--restart unless-stopped` still shows "Up".

**A single `docker stats` is not representative of gean.** gean's load is bursty: several cores during
a proof, near idle between them. One snapshot catches it mid-burst or mid-idle and neither is the
truth. Use `uptime`'s load average against `nproc` for the
honest summary, and read the three figures as a trend — a 1-minute average well above the 15-minute
one means load is climbing.

Check swap explicitly. A box can show idle CPU and a load average several times its core count at
the same time, because load counts processes blocked on I/O. `free -g` plus the `si`/`so` columns of
`vmstat` tell you whether memory pressure is the real constraint.

### Tick loop / scheduling

```bash
curl -sS -G localhost:9090/api/v1/query --data-urlencode 'query=rate(lean_tick_interval_duration_seconds_sum[10m])/rate(lean_tick_interval_duration_seconds_count[10m])'
curl -sS -G localhost:9090/api/v1/query --data-urlencode 'query=max_over_time((rate(lean_tick_interval_duration_seconds_sum{job=~"gean_.*"}[10m])/rate(lean_tick_interval_duration_seconds_count{job=~"gean_.*"}[10m]))[12h:10m])'
curl -sS -G localhost:9090/api/v1/query --data-urlencode 'query=lean_tick_last_age_seconds'
curl -sS -G localhost:9090/api/v1/query --data-urlencode 'query=rate(lean_dispatch_event_duration_seconds_sum[10m])/rate(lean_dispatch_event_duration_seconds_count[10m])'
```

Target 0.800 s. `lean_dispatch_event_duration_seconds{event=}` attributes a slow loop to `tick`,
`block`, `proposal_result`, `early_aggregate` or `failed_root`. Note the tick-interval histogram is
observed *inside* `onTick`, so a fully blocked loop makes it go **silent** rather than spike —
`lean_tick_last_age_seconds` is the gauge that keeps climbing.

### Finality and justification

```bash
curl -sS -G localhost:9090/api/v1/query --data-urlencode 'query=lean_head_slot - on(job) lean_latest_justified_slot'
curl -sS -G localhost:9090/api/v1/query --data-urlencode 'query=lean_latest_justified_slot - on(job) lean_latest_finalized_slot'
curl -sS -G localhost:9090/api/v1/query --data-urlencode 'query=min_over_time((lean_latest_justified_slot - on(job) lean_latest_finalized_slot)[1h:1m])'
curl -sS -G localhost:9090/api/v1/query --data-urlencode 'query=avg_over_time((lean_latest_justified_slot - on(job) lean_latest_finalized_slot)[1h:1m])'
curl -sS -G localhost:9090/api/v1/query --data-urlencode 'query=max_over_time((lean_latest_justified_slot - on(job) lean_latest_finalized_slot)[1h:1m])'
```

Separate the two gaps. Head−justified is *aggregation health*. Justified−finalized is *3SF rung
spacing* — it is supposed to be a staircase, widening then snapping shut, so judge it by min/mean,
never by a single sample. Finalisation lands only on justifiable distances: ≤5, perfect squares, or
d where 4d+1 is an odd perfect square (…110, 121, 132, 156, 182, 210, 240). A gap sitting on one of
those is the rule working.

### Aggregation producing nothing

```bash
curl -sS -G localhost:9090/api/v1/query --data-urlencode 'query=increase(lean_proof_operations_total{operation="aggregation"}[1h])'
curl -sS -G localhost:9090/api/v1/query --data-urlencode 'query=increase(lean_aggregation_groups_skipped_total[1h])'
curl -sS -G localhost:9090/api/v1/query --data-urlencode 'query=increase(lean_pq_sig_aggregated_signatures_total[1h])'
curl -sS -G localhost:9090/api/v1/query --data-urlencode 'query=increase(lean_aggregator_skipped_total[1h])'
curl -sS -G localhost:9090/api/v1/query --data-urlencode 'query=increase(lean_aggregation_dispatch_dropped_total[1h])'
curl -sS -G localhost:9090/api/v1/query --data-urlencode 'query=increase(lean_dispatch_event_duration_seconds_count{event="early_aggregate"}[1h])'
docker logs "$c" 2>&1 | sed -E 's/\x1b\[[0-9;]*m//g' | grep -E 'aggregation (worker|session|skipped|yielded)' | tail -20
```

When a session starts: `maybeEarlyAggregate` (`internal/node/tick.go`) dispatches in interval 1
once this slot's collected votes reach ceil(2n/3) of the voters the node expects on its subnets;
below that, interval 2 is the fallback. Either path dispatches at most once per slot, and neither
sits behind the sync-lag duty gate — a node marked syncing still aggregates. The
`early_aggregate` dispatch event counts coalesced attestation wake-ups checked, not sessions started.

`lean_proof_operations_total{result=}`: `success` (≥1 aggregate), `empty` (ran, produced nothing),
`canceled` (lost the prover to a proposal — the priority rule working, not a fault), `expired`
(arrived past its deadline), `truncated` (hit the budget — counted *alongside* success or empty, not
instead of). `lean_aggregation_dispatch_dropped_total` rising means the worker was still busy with
the previous slot's session when the next was dispatched.

`lean_aggregation_groups_skipped_total{reason=}` is normally dominated by `target_justified`, which
is correct: the target is already justified so the vote cannot advance anything. That dominance
hides the other reasons — look at them individually, not at the total.

Log lines: `aggregation worker:` per completed session (`produced=`, `duration=`), `aggregation
session ...` for budget truncation/overrun, `aggregation skipped:` and `aggregation yielded to
proposal:` for the other outcomes. They carry the real durations the histograms cannot reach.

### Storage and pruning

```bash
curl -sS -G localhost:9090/api/v1/query --data-urlencode 'query=lean_table_bytes'
docker logs "$c" 2>&1 | sed -E 's/\x1b\[[0-9;]*m//g' | grep -E 'pruning:|finalization stalled:' | tail -20
```

Finalisation prune lines (`pruning: finalized_slot=...`) carry `states= blocks= live_chain=
non_canonical=`. **Zero `states` and `blocks` on every line means finalisation pruning is not
deleting anything** and the tables grow for the life of the chain; `blocks` should track
`non_canonical`. `finalization stalled:` followed by `periodic pruning: ancestor_slot=...` means
finalisation is lagging and the periodic path is bounding storage instead — the finality problem,
not storage, is the thing to chase.

### Memory

```bash
docker exec "$c" cat /proc/1/smaps_rollup | head -8
docker exec "$c" sh -c 'echo "$LD_PRELOAD"'
curl -sS -G localhost:9090/api/v1/query --data-urlencode 'query=lean_node_rss_bytes - go_memstats_heap_inuse_bytes'
curl -sS -G localhost:9090/api/v1/query --data-urlencode 'query=min_over_time(lean_node_rss_bytes[2h]) - min_over_time(lean_node_rss_bytes[2h] offset 10h)'
```

RSS oscillates by ~3 GB with prover activity, so **two point samples cannot show a trend** — the
*floor* (`min_over_time`) is the signal. Almost all of gean's RSS is outside the Go heap; the Go
heap is a rounding error. The image preloads jemalloc (`LD_PRELOAD=libjemalloc.so.2` in
`Dockerfile`) so prover scratch is returned to the OS on a decay timer; if `LD_PRELOAD` is empty the
node runs on glibc malloc, which retains freed prover memory, and a rising floor is expected.

### Signature verification

```bash
curl -sS -G localhost:9090/api/v1/query --data-urlencode 'query=rate(lean_pq_sig_aggregated_signatures_verification_time_seconds_sum[1h])/rate(lean_pq_sig_aggregated_signatures_verification_time_seconds_count[1h])'
curl -sS -G localhost:9090/api/v1/query --data-urlencode 'query=rate(lean_attestation_validation_time_seconds_sum[10m])/rate(lean_attestation_validation_time_seconds_count[10m])'
curl -sS -G localhost:9090/api/v1/query --data-urlencode 'query=histogram_quantile(0.95, rate(lean_gossip_aggregation_size_bytes_bucket[1h]))'
```

Compare the **mean**, not a quantile — it is immune to bucket boundaries. Before calling a gap a
defect, rule out bigger proofs (`lean_gossip_aggregation_size_bytes`) and higher volume
(`rate(..._count)`); if proof size and count match and time does not, the gap is per-operation.

### Sync flapping

```bash
curl -sS -G localhost:9090/api/v1/query --data-urlencode 'query=lean_node_sync_status'
curl -sS -G localhost:9090/api/v1/query --data-urlencode 'query=changes(lean_node_sync_status[1h])'
```

gean marks itself syncing at `currentSlot - headSlot > SyncLagSlots` (2), i.e. 12 seconds behind on
a 4 s slot. When lagging, the sync-lag duty gate skips block production and attestation
(`lean_node_blocks_skipped_lag_total`, `lean_node_attestations_skipped_lag_total`); aggregation is
not gated. The threshold is tight enough that flapping is usually jitter — but all gean nodes
flapping while other clients do not is a pattern worth pursuing.

## Preserving evidence

Container logs are the only copy in local mode — there is no Loki, and `spin-node.sh --stop` runs
`docker rm -f`, which deletes them. Capturing writes files on the host, so it is **opt-in**: offer it
before any stop or before a run ends, and run it only when the user approves:

```bash
mkdir -p ~/devnet-evidence/$(date +%F) && cd ~/devnet-evidence/$(date +%F)
docker ps -a --format '{{.Names}}\t{{.Status}}' | tee containers.txt
for c in $(docker ps --format '{{.Names}}' | grep -Ev 'grafana|prometheus'); do docker logs --timestamps "$c" 2>&1 | gzip > $c.log.gz; done
docker stats --no-stream > docker-stats.txt
```

`2>&1` matters — gean writes to both streams.

## Reporting

Give a verdict per symptom: **healthy** (a metric or log line positively shows the expected
behaviour), **unhealthy** (evidence of the fault), or **insufficient evidence**. Never call anything
healthy from the absence of errors — a missing series or a quiet log is insufficient evidence.

State what was measured, what it rules in, what it rules out, and what remains unknown. Give the
proposed fix with the evidence for it. Where a hypothesis died, say so — a rejected explanation
saves the next person from testing it again. Then stop and wait.
