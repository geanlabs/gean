# Instability and non-finality testing

Pause validators to stall finality, measure block processing under an attestation backlog, and
check recovery.

Justification needs two-thirds of the validators: see the threshold in
[the devnet-log-review skill](../../devnet-log-review/SKILL.md#common-investigations). With 4
nodes of one validator each, pausing 1 still finalizes and pausing 2 stalls it.

## Before you start

- A running devnet with 4 gean nodes.
- Know which node aggregates (`isAggregator: true` in `validator-config.yaml`). Pausing it leaves
  blocks with `attestations=0` and stops finality at once; keep it running for the steps below.

## Stall finality

```bash
docker ps --format 'table {{.Names}}\t{{.Status}}' | grep gean
docker pause gean_0 gean_1                        # two non-aggregators
docker inspect gean_0 --format '{{.State.Paused}}' # true
docker logs --tail 20 gean_2 2>&1
docker unpause gean_0 gean_1
```

What follows:

- At once: the paused validators' slots are missed, and the aggregator gets votes from 2
  validators only.
- Within about 20 slots: justified and finalized slots stop advancing, and the attestation target
  falls behind the head.
- After 50+ slots: blocks fill to gean's cap of 3 attestations, block processing time grows with
  the count, and the target-to-head gap grows about one slot per slot.

```bash
# Finality stays put; the head keeps advancing
docker logs gean_2 2>&1 | grep -E 'CHAIN STATUS|Latest (Justified|Finalized)' | tail -9
```

## Block processing data

Each imported block is logged once on every node as `[chain] block slot=X ... attestations=N ...
proc_time=D`. One line per block, as `node slot attestations milliseconds`:

```bash
for n in $(docker ps --format '{{.Names}}' | grep '^gean_'); do
  docker logs "$n" 2>&1 | grep '\[chain\] block slot=' |
    awk -v n="$n" '{
      match($0, /slot=[0-9]+/); slot = substr($0, RSTART + 5, RLENGTH - 5)
      match($0, /attestations=[0-9]+/); atts = substr($0, RSTART + 13, RLENGTH - 13)
      match($0, /proc_time=[^ ]+/); d = substr($0, RSTART + 10, RLENGTH - 10)
      ms = d + 0; if (d !~ /ms$/) ms *= 1000
      print n, slot, atts, ms
    }'
done > blocks.txt

sort -k4 -n blocks.txt | tail -1                                   # slowest block
awk -v from=50 '$2 > from { s += $4; c++ } END { print c " blocks, mean " s / c " ms" }' blocks.txt
```

Set `from` to the slot you paused at. Plot `attestations` against milliseconds to see how
verification scales with the backlog.

## Pause or stop

`docker pause` freezes the process and keeps the container up: peers see it go silent and drop it
after a timeout, as in a network partition, and `docker unpause` resumes it at once.
`docker stop` ends the process, as in a crash: peers see the disconnect at once, and
`docker start` restarts the node, which then syncs back up. Both keep the data directory.
