# Error classification

## Critical

| Line | Meaning | Action |
|---|---|---|
| `panic:` | The node crashed | Read the stack trace |
| `ERROR [chain] block processing failed ... state root mismatch` | The block's state root differs from the one this node computes | If every node computes the same root, block building filled the header wrongly; otherwise the transition isn't deterministic |
| `ERROR [chain] block processing failed` (any other reason) | A block this node can't import | One node failing: its validation; every node: the proposer |
| `ERROR [signature] aggregated attestation verification failed` | A proof in a block or on gossip doesn't verify | Check XMSS aggregation on the aggregator |
| `ERROR [forkchoice]` | Fork choice rejected an update | Read the error; often a missing block |

## Medium

| Line | Meaning | Action |
|---|---|---|
| `WARN [chain] block parent missing ... storing as pending` | Parent not seen yet | gean fetches it; a problem only if `fetch exhausted` follows |
| `WARN [sync] fetch exhausted for root` | The parent never arrived; its children were dropped | Check peers and whether the parent was ever published |
| `WARN [forkchoice] REORG depth=N` | The head switched forks | Deep or frequent reorgs: see [FORK_ANALYSIS.md](FORK_ANALYSIS.md) |
| `WARN [forkchoice] justified root ... unknown to fork choice` | The head can't advance past a justified block it lacks | A missing block; see the sync lines |
| `WARN [gossip] dropped N gossip attestations` | The verify queue overflowed | The node is overloaded; votes may be lost |
| `WARN [signature] aggregation session overran budget` / `hit budget without output` | The aggregator ran out of time | Fewer votes reach blocks; finality may slow |
| `WARN [store] finalization lagging` | Finality is far behind the head | See [FINALIZATION_DEBUG.md](FINALIZATION_DEBUG.md) |

## Benign

| Line | Meaning | Why it's OK |
|---|---|---|
| `Error response from daemon: manifest unknown` (in `devnet.log`) | Docker image tag not in the remote registry | Docker uses the local image; a problem only if there is none |
| `WARN [network] bootnode connect failed` | Dialling a node that isn't running | Expected for nodes in the validator config that weren't started |
| `WARN [validator] discarding proposal slot=X: head moved` | A block for that slot arrived first | Normal when occasional |

To tell which node an address belongs to, find its `enrFields.quic` port in
`lean-quickstart/local-devnet/genesis/validator-config.yaml`. Stale containers from an earlier run
show as nodes that can't sync from genesis; see
[the devnet-runner skill](../../devnet-runner/SKILL.md#troubleshooting).
