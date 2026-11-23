# Validator config

`lean-quickstart/local-devnet/genesis/validator-config.yaml` lists the devnet's nodes and how the
validators are dealt to them. An entry looks like:

```yaml
validators:
  - name: "gean_0"
    privkey: "<64 hex characters>"
    enrFields:
      ip: "127.0.0.1"
      quic: 9001
    metricsPort: 8081
    count: 1
    isAggregator: true
```

- `name`: `gean_<n>`. The prefix selects lean-quickstart's `client-cmds/gean-cmd.sh`.
- `privkey`: the node's libp2p identity, 32 bytes as hex; make one with `openssl rand -hex 32`.
- `enrFields.ip` and `enrFields.quic`: where peers reach the node. Locally the IP is `127.0.0.1`
  and each node needs its own QUIC port.
- `metricsPort`: unique per node locally. `gean-cmd.sh` also passes gean's API port from here.
- `count`: how many validator indices the node runs; the counts add up to the validator total.
- `isAggregator`: at least one node needs it, or nothing finalizes.

The top of the file also sets `shuffle` (proposer order; `roundrobin` gives fixed turns),
`deployment_mode` (`local` here) and the key settings under `config`, which stay as they are.

## Adding or removing a node

Add an entry with the next name, a new `privkey` and unused ports, or delete one. The validator
set changes either way, so regenerate genesis, with new keys when a node was added:

```bash
cd lean-quickstart
NETWORK_DIR=local-devnet ./spin-node.sh --node all --generateGenesis --forceKeyGen   # after adding
NETWORK_DIR=local-devnet ./spin-node.sh --node all --generateGenesis                # after removing
```

## Ports

Give node `gean_<n>` QUIC port `9001+n` and metrics port `8081+n`, so no two local nodes collide.
To find which node a port in a log belongs to, look it up in this file.
