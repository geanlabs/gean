---
name: devnet-triage
description: Diagnose a running lean devnet from its Prometheus and container logs, then report findings with proposed fixes and stop for approval. Use when the user reports a symptom on a live devnet — finality or justification lagging, the tick loop drifting, memory growing, aggregation producing nothing, nodes flapping sync status — or asks what a metric means, whether a number is healthy, or why gean differs from another client. Replaces one-query-at-a-time debugging with a single bundle per symptom.
---

# Devnet triage

Diagnose a **running** devnet by reading metrics and logs, then hand the user findings and a
proposed fix. Sibling skills cover other phases: `devnet-runner` starts a devnet,
`devnet-log-review` analyses a finished run's dumped logs, `gean-perf` covers Shadow.

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

Note the shell nesting: bundles below are written to run *on the host*. Wrap them in
`ssh gean-devnet '...'` and keep inner quoting single-level, or write the bundle to a heredoc.

Prometheus is reachable on the host; each node's metrics port comes from its own container args:

```bash
docker inspect gean_0 --format '{{join .Args " "}}' | tr ' ' '\n' | grep -A1 -- '--metrics-port' | tail -1
```

Do not assume 8080. It is usually 8081 for gean_0 and the ports differ per node.

## Traps — read this before interpreting anything

These cost real time to rediscover.

**Histograms that cannot see their own tail.** `lean_proving_duration_seconds` tops out at **8 s**
and `lean_pq_sig_aggregated_signatures_building_time_seconds` at **4 s**. A p95/p99 sitting exactly
on those numbers means "at least that", not "that". Logs carry the real value; the metric does not.

**Counters with only one possible label value.** gean increments
`lean_finalizations_total{result="success"}` and nothing else — there is no error path. A dashboard
"Finalizations – Errors" panel therefore contains **no gean series at all**, which looks like a
clean record and is not one.

**Cross-client metric semantics do not match.** Same metric name, different meaning. ethlambda's
finalisation counter counts *attempts*; gean's counts *advances* — comparing them is meaningless.
`lean_gossip_signatures` reads 0 on ethlambda because it does not populate it. Before comparing any
metric across clients, check both definitions. Buckets sometimes *do* match (the aggregated-signature
verification histogram is identical in gean and ethlambda) — verify, do not assume either way.

**`max_over_time` on a short window reads as a trend and is not one.** A gap that oscillates 1 → 169
and returns produces a rising `max` if you sample two windows. Always pair it with `min_over_time`
and `avg_over_time` before calling anything a trend.

**Means dominated by no-op samples.** `lean_proving_duration_seconds` records whole sessions
including the ~46% that prove nothing, so its p50 reads ~80 ms while real proofs take seconds. Use a
high quantile, or filter, or read the logs.

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
for c in gean_0 gean_1 gean_2 gean_3; do echo "$c restarts=$(docker inspect $c --format "{{.RestartCount}}")"; done
nproc; uptime; free -g | head -2
docker stats --no-stream --format "{{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}" | sort -k2 -hr
vmstat 1 3 | tail -2
curl -sS -G localhost:9090/api/v1/query --data-urlencode "query=lean_current_slot" | head -c 300'
```

Non-zero `RestartCount` on any node invalidates the run — a crash loop behind
`--restart unless-stopped` still shows "Up".

**A single `docker stats` is not representative of gean.** gean's load is bursty: ~250% CPU during a
7-8 s proof, under 3% between them, with proofs roughly every third slot. One snapshot catches it
mid-burst or mid-idle and neither is the truth. Use `uptime`'s load average against `nproc` for the
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
docker logs gean_0 2>&1 | grep -iE "aggregation session" | tail -20
```

`lean_proof_operations_total{result=}`: `success` (≥1 aggregate), `empty` (ran, produced nothing),
`canceled` (lost the prover to a proposal — the priority rule working, not a fault), `expired`
(arrived past its deadline), `truncated` (hit the budget — counted *alongside* success or empty, not
instead of).

`lean_aggregation_groups_skipped_total{reason=}` is normally ~98% `target_justified`, which is
correct behaviour: the target is already justified so the vote cannot advance anything. That
dominance hides the other reasons — look at them individually, not at the total.

Session logs carry the real durations the histograms cannot reach.

### Storage and pruning

```bash
curl -sS -G localhost:9090/api/v1/query --data-urlencode 'query=lean_table_bytes'
docker logs gean_0 2>&1 | grep -i pruning | tail -20
```

Prune lines read `states=N blocks=N live_chain=N non_canonical=N`. **Zero `states` and `blocks` on
every line means finalisation pruning is not deleting anything** and the tables grow for the life of
the chain. `blocks` should track `non_canonical`.

### Memory

```bash
docker exec gean_0 cat /proc/1/smaps_rollup | head -8
docker exec gean_0 sh -c "awk '/^Rss:/{if (\$2>60000 && \$2<70000) n++} END{print n\" arenas of ~64MB\"}' /proc/1/smaps"
curl -sS -G localhost:9090/api/v1/query --data-urlencode 'query=lean_node_rss_bytes - go_memstats_heap_inuse_bytes'
curl -sS -G localhost:9090/api/v1/query --data-urlencode 'query=min_over_time(lean_node_rss_bytes[2h]) - min_over_time(lean_node_rss_bytes[2h] offset 10h)'
```

RSS oscillates by ~3 GB with prover activity, so **two point samples cannot show a trend** — the
*floor* (`min_over_time`) is the signal. Almost all of gean's RSS is outside the Go heap; the Go
heap is a rounding error. Repeated ~64 MiB anonymous mappings are glibc malloc arenas, which
outlive the threads that created them — so `go_threads` is *not* a proxy for how many exist.

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
a 4 s slot. That threshold is tight enough that flapping is usually jitter — but all gean nodes
flapping while other clients do not is a pattern worth pursuing.

## Preserving evidence

Container logs are the only copy in local mode — there is no Loki, and `spin-node.sh --stop` runs
`docker rm -f`, which deletes them. Before any stop, and before drawing conclusions from a run that
is about to end:

```bash
mkdir -p ~/devnet-evidence/$(date +%F) && cd ~/devnet-evidence/$(date +%F)
docker ps -a --format '{{.Names}}\t{{.Status}}' | tee containers.txt
for c in $(docker ps --format '{{.Names}}' | grep -Ev 'grafana|prometheus'); do docker logs --timestamps "$c" 2>&1 | gzip > $c.log.gz; done
docker stats --no-stream > docker-stats.txt
```

`2>&1` matters — gean writes to both streams.

## Reporting

State what was measured, what it rules in, what it rules out, and what remains unknown. Give the
proposed fix with the evidence for it. Where a hypothesis died, say so — a rejected explanation
saves the next person from testing it again. Then stop and wait.
