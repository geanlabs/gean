# Execution-layer integration with geth

Status: implemented on branch `feat/el-integration-geth`, 2026-09-14. The
decisions below record what was built; where the build departed from the
first draft, the decision says so.

This plan gives gean the ability to drive a standard Ethereum execution
client (geth first) over the Engine API, so that every lean block carries a
real execution payload, proposers obtain payloads from geth, and importers
have geth validate them. It is informed by ethlambda PR #367, which did the
same for ethrex, and deliberately departs from it where that PR cut corners.

## 1. What we are building

Per slot, an EL-paired gean node does four things:

| When | Call | Purpose |
|---|---|---|
| interval 4 of slot N-1, if we propose N | `engine_forkchoiceUpdatedV3` + payload attributes | ask geth to start building the payload for slot N |
| interval 0 of slot N, in the proposal worker | `engine_getPayloadV3` | fetch the built payload and embed it in the block body |
| on every block received (gossip or sync) | `engine_newPayloadV3` | have geth execute and validate the payload before import |
| after every head change | `engine_forkchoiceUpdatedV3` | keep geth's head, safe, and finalized blocks aligned with fork choice |

The consensus schema gains one field on `BlockBody` (`execution_payload`)
and one on `State` (`latest_execution_payload_header`), and the state
transition gains `ProcessExecutionPayload` between header and attestation
processing.

## 2. Design decisions

Each decision is numbered so PRs can reference it.

**D1. Adopt ethlambda's exact SSZ shape.** `ExecutionPayloadV3` is the
unmodified Cancun container appended to `BlockBody` after `attestations`;
`ExecutionPayloadHeader` (transactions and withdrawals replaced by their
roots) is appended to `State`. Constants per execution-apis:
`BYTES_PER_LOGS_BLOOM=256`, `MAX_EXTRA_DATA_BYTES=32`,
`MAX_BYTES_PER_TRANSACTION=2^30`, `MAX_TRANSACTIONS_PER_PAYLOAD=2^20`,
`MAX_WITHDRAWALS_PER_PAYLOAD=16`. Using the identical shape is what makes a
gean+geth node and an ethlambda+ethrex node able to share a devnet.

**D2. Spec fixtures are gated, not forked.** The pinned leanSpec has no
execution payload and this branch touches gean only, so fixtures cannot be
regenerated. Fixture families whose bytes encode the previous `BlockBody`
and `State` shapes (state transition, fork choice, signature verification,
sync, API, and the SSZ vectors for those four containers) are skipped
behind one constant, `schemaFixturesPending` in `internal/spectests/gate.go`;
every other family still runs. Clear the constant once fixtures exist for
the new schema. The consensus rule itself is covered by table-driven unit
tests instead.

**D3. Keep ethlambda's fork digest.** `p2p.ForkDigest` stays `12345678`,
the value ethlambda's EL branch also keeps, so the two clients share topics
and ENR fork fields with no coordination step. The cost is that an EL build
is not wire-isolated from devnet-5 by the digest alone: a mixed mesh would
see undecodable blocks and mismatched status roots rather than silence. The
guard is operational, not protocol: an EL network has its own genesis
(different `EXECUTION_GENESIS_BLOCK_HASH`, hence a different genesis root)
and its own bootnode list, and the startup check in D4 refuses to run
against the wrong EL. Revisit if leanSpec assigns the schema a fork digest.

**D4. The EL genesis hash is a network parameter, not a node flag.**
`config.yaml` gains `EXECUTION_GENESIS_BLOCK_HASH`. Genesis state seeds
`latest_execution_payload_header.block_hash` and the genesis block body's
`execution_payload.block_hash` from it, so every node computes the same
genesis root. At startup an EL-paired node reads geth's block 0 hash over the
auth port (`eth_getBlockByNumber`) and refuses to start if it differs.
ethlambda's per-node `--execution-genesis-block-hash` flag is the single
easiest way to split a network; we remove that class of error.

**D5. The genesis header decides whether a chain has an execution layer.**
Without `EXECUTION_GENESIS_BLOCK_HASH` the cached header's block hash stays
zero for the life of the chain, and the transition requires every block to
carry the zero payload; nothing is synthesised and existing behaviour is
unchanged. With the key, the header starts at the execution genesis hash,
every block must carry a payload that chains from it with the slot's
timestamp, and a validator without an execution endpoint refuses to start.
A proposer that cannot obtain a payload skips the slot rather than emit a
block every peer's execution client would reject.

**D6. Cancun (V3) Engine methods.** `forkchoiceUpdatedV3`, `getPayloadV3`,
`newPayloadV3`, with empty blob hashes. Geth's genesis activates Shanghai and
Cancun at time 0 and leaves Prague unset. V4 requires `executionRequests`
to round-trip through the block, which the schema does not carry; adding
Prague later is a change inside the client package behind the same
interface, plus a schema decision, and is out of scope here.

**D7. `parentBeaconBlockRoot` is the lean parent root.** At build time it is
the head root the payload was requested on; at import time it is
`block.ParentRoot`. Both must be identical or geth's block-hash check fails.
The proposal worker verifies, immediately before signing, that the block's
parent root still equals the root the payload was built on and skips the
slot otherwise. This closes the race ethlambda has between its stale-id
guard and the spec's proposal-head recomputation.

**D8. Verdict policy.** `VALID`, `SYNCING`, `ACCEPTED` import.
`INVALID`, `INVALID_BLOCK_HASH` reject the block before it reaches the
store. A transport failure is treated as `SYNCING`, counted under its own
metric label, and puts the client into a two-second back-off during which
calls fail fast instead of waiting out a timeout per block; consensus never
stalls on a flaky EL and the reachability gauge shows the gap. Proposals are
stricter: an unreachable EL means no proposal.

**D9. Nothing on the dispatch loop.** Every EL call runs on an existing
off-loop worker or a new `executionDriver` goroutine. Fork-choice updates are
fire-and-forget. `newPayload` runs in a serial ingress worker that sits
between p2p delivery and `BlockCh`, preserving sync ordering. `getPayload`
runs inside the proposal worker, which already runs off the loop.

**D10. No new Go dependencies.** JSON-RPC over `net/http` and
`encoding/json`; JWT HS256 with an `iat` claim over `crypto/hmac` and
`encoding/base64`. The whole client is a few hundred lines.

## 3. Package layout

| Package | New or changed | Contents |
|---|---|---|
| `internal/types/execution_payload.go` | new | `ExecutionPayload`, `ExecutionPayloadHeader`, `Withdrawal`, `ToHeader()`, constants |
| `internal/types/execution_payload_encoding.go` | generated | via `make sszgen`, new target line |
| `internal/types/block.go`, `state.go` | changed | one field each; regenerate |
| `internal/statetransition/execution.go` | new | `ProcessExecutionPayload`, `ComputeTimeAtSlot`, `SecondsPerSlot` reuse, typed errors |
| `internal/genesis/` | changed | config key, header and body seeding |
| `internal/blockbuilder/` | changed | `Input.ExecutionPayload` threaded through trial blocks |
| `internal/execution/` | new | `jwt.go`, `rpc.go`, `types.go`, `client.go`, `engine.go` (interface), `mock.go` (test) |
| `internal/node/execution.go` | new | driver: prepare at interval 4, take at interval 0, FCU on head change, ingress verifier |
| `internal/metrics/` | changed | counters, histograms, reachability gauge |
| `cmd/gean/flags.go`, `execution.go` | changed / new | flags, client construction, capability handshake, genesis check |
| `internal/specfixtures/` | changed | fixture parsers learn the new fields |
| `Makefile`, `scripts/el-demo/` | changed / new | `sszgen` target, `run-el-demo`, geth genesis JSON |

## 4. Phases

Each phase is one PR unless noted. Every phase ends with `make build`,
`make test`, `make lint`, and, for phases that touch consensus,
`make test-spec`.

### Phase 0: spec fork and fixtures (leanSpec, Python) — not done, see D2

- Branch `execution-payload` on a geanlabs fork of leanSpec.
- `containers/execution_payload.py`: `ExecutionPayload`,
  `ExecutionPayloadHeader`, `Withdrawal`, constants.
- `BlockBody.execution_payload`, `State.latest_execution_payload_header`.
- `process_execution_payload(state, block)` in the state transition, called
  from `process_block` after the header. Checks parent hash and timestamp,
  caches the header.
- Genesis helper takes an optional EL genesis hash and seeds both places.
- Block-production test helpers fill the synthetic payload.
- Regenerate fixtures (`uv run fill --fork=lstar --scheme=prod`).
- gean: point `Makefile` clone URL and `LEAN_SPEC_COMMIT_HASH` at the fork.

Deliverable: fixtures exist that encode the new schema. Size: small Python
change, mostly plumbing through the test filler.

### Phase 1: schema and state transition (gean)

- Types per D1 with fastssz tags; `make sszgen` (install `sszgen` first).
- `ProcessExecutionPayload` per the spec fork, wired into `ProcessBlock`.
- Genesis per D4: config key parsed and validated as 32 bytes; state and
  genesis block body seeded.
- Block builder per D5: synthetic fill when the network has no EL hash.
- `specfixtures` parsers extended.
- Tests: table-driven STF tests (happy path, parent mismatch, timestamp
  mismatch, chaining across two blocks); genesis root determinism; `make
  test-spec` green against the fork's fixtures.

At the end of this phase a pure-consensus devnet of gean nodes still runs
end to end with zero payloads. Nothing talks to geth yet.

### Phase 2: Engine API client (gean, no node wiring)

- `internal/execution/jwt.go`: HS256 token with `iat`; hex secret file
  loader with and without `0x`; 32-byte length check.
- `internal/execution/rpc.go`: JSON-RPC envelope, bearer header, per-call
  `context` deadline, typed error for transport, RPC error, decode.
- `internal/execution/types.go`: `ForkchoiceState`, `PayloadAttributes`,
  `PayloadStatus`, `ForkchoiceUpdatedResponse`, `PayloadID`, and JSON
  codecs for the payload (hex quantities, hex data, `SCREAMING_SNAKE`
  status). The SSZ type from Phase 1 gets the JSON marshalling here, not in
  `internal/types`, so consensus types stay free of wire concerns.
- `internal/execution/engine.go`: the interface the node depends on:
  `ForkchoiceUpdated`, `GetPayload`, `NewPayload`, `GenesisBlockHash`,
  `ExchangeCapabilities`. `client.go` implements it; `mock.go` is a
  scriptable test double.
- Tests: JWT vector against a known token; JSON round-trips for every type
  using execution-apis example bodies; an `httptest.Server` that asserts
  method names, param shapes, and the bearer header, and returns canned
  `SYNCING` / `VALID` / error envelopes.

### Phase 3: startup and configuration (gean)

- Flags: `--execution-endpoint`, `--execution-jwt-secret`,
  `--suggested-fee-recipient` (node level; zero warns). Endpoint and secret
  are required together.
- Validation per D5: EL hash in config plus validator keys requires an
  endpoint; endpoint without EL hash in config is an error.
- `cmd/gean/execution.go`: build the client, `engine_exchangeCapabilities`
  handshake (log the intersection, warn on missing methods), then
  `GenesisBlockHash` compared with the config key; mismatch is fatal.
- Metrics: `lean_execution_reachable` gauge set from the handshake and
  refreshed by every later call.

### Phase 4: proposal path and fork-choice updates (gean)

- `internal/node/execution.go`, the driver:
  - `prepareNextSlot(slot)` at interval 4 when `proposingAt(slot+1)`:
    builds `ForkchoiceState` from the head, safe target, and finalized
    roots by reading `execution_payload.block_hash` off the stored blocks
    (small root-to-hash cache), sends the build-mode FCU on a goroutine,
    stores `(slot, headRoot, payloadID)` under a mutex.
  - `takePayload(slot, parentRoot)` called by the proposal worker: returns
    the payload only if the stash matches slot and parent root; otherwise
    skips the slot per D5 and D7.
  - `notifyForkchoice()` after `updateHead` changes the head, and once per
    slot at interval 0 regardless, fire-and-forget on a goroutine.
- Proposal worker: call `takePayload` before `blockbuilder.Build`, pass it
  in `Input.ExecutionPayload`, re-check parent root before signing per D7,
  then send `newPayload` for our own block on a goroutine so geth promotes
  the candidate it built.
- Metrics: `lean_execution_call_duration_seconds{method}`,
  `lean_execution_fcu_total{status}`, `lean_execution_getpayload_total{result}`,
  `lean_proposals_skipped_total{reason="no_payload"}`.
- Verified live with `make run-el-demo`: one gean validator, one geth,
  blocks build with non-zero payloads, geth's head follows.

### Phase 5: import gating (gean)

- Ingress verifier per D8 and D9: `OnBlock` and `OnSyncBlock` hand the
  block to a serial worker that calls `newPayload` with
  `block.ParentRoot`, then forwards to `BlockCh` or drops. Gossip keeps its
  drop-on-full behaviour; sync keeps its blocking behaviour.
- Blocks whose parent is unknown still go to the pending buffer unverified;
  they are verified when replayed, because geth cannot judge a payload whose
  parent it has not seen.
- Metrics: `lean_execution_newpayload_total{status}` including
  `unreachable`; `lean_blocks_rejected_total{reason="execution_invalid"}`.
- Tests with the mock engine: verdict matrix, retry-then-syncing, ordering
  preserved under sync delivery, INVALID never reaches the store.

### Phase 6: devnet and interop

- `scripts/el-demo/`: geth genesis JSON, `run.sh` that runs `geth init`,
  starts geth, generates a lean testnet with the EL hash written into
  `config.yaml`, and starts gean.
- Three-node gean network, one geth per gean, geths peered with static
  enodes so transactions propagate; send transactions and confirm they
  land.
- Failure drills: stop geth mid-run (attest continues, proposals skip,
  recovery on restart); tamper a payload in one node and confirm peers
  reject; force a reorg and confirm FCU follows.
- Interop with an ethlambda PR #367 build; digests already match per D3.

### Phase 7: hardening and docs

- Timeouts tuned per call: FCU 1 s, getPayload 600 ms (inside interval 0),
  newPayload 2 s with one retry.
- Late-joining node: backfill feeds `newPayload` in slot order so geth
  imports the EL chain from the consensus stream; document that geth p2p
  peering is optional but faster.
- README section, CLAUDE.md architecture note, `devnet-runner` skill
  reference for the EL genesis bundle.

## 5. Slot timeline with EL calls

```
slot N-1                                    slot N
|--- 0 ---|--- 1 ---|--- 2 ---|--- 3 ---|--- 4 ---|--- 0 ---|--- 1 ---|
                                          FCU+attrs  getPayload
                                          (if we     build, sign,
                                          propose N) newPayload(own)
                                                     FCU(head)
 on any block arrival: newPayload -> BlockCh -> import -> updateHead -> FCU
```

Interval 4 to interval 0 gives geth 800 ms to build. Geth returns an empty
block immediately and a fuller one as it recommits, so `getPayload` at
interval 0 always has something.

## 6. Geth setup

Genesis (`scripts/el-demo/genesis.json`): `chainId` for the devnet,
`terminalTotalDifficulty: 0`, `shanghaiTime: 0`, `cancunTime: 0`, no
`pragueTime`, a modest `gasLimit` so a full block stays well under gean's
10 MiB gossip payload cap, and the EIP-4788 beacon-roots contract in
`alloc` so `parentBeaconBlockRoot` is stored as on mainnet.

```
geth init --datadir data/geth0 scripts/el-demo/genesis.json
openssl rand -hex 32 > data/jwt.hex
geth --datadir data/geth0 \
     --authrpc.addr 127.0.0.1 --authrpc.port 8551 --authrpc.jwtsecret data/jwt.hex \
     --http --http.api eth,net --nodiscover --syncmode full
```

Geth's auth port serves both `engine` and `eth`, which is what lets gean
read block 0 for the D4 check without a second endpoint. The genesis hash
geth prints on init goes into `config.yaml` as
`EXECUTION_GENESIS_BLOCK_HASH`.

## 7. What changes for operators

```
bin/gean --custom-network-config-dir testnet --node-key ... --node-id node0 \
         --execution-endpoint http://127.0.0.1:8551 \
         --execution-jwt-secret data/jwt.hex \
         --suggested-fee-recipient 0x...
```

Without `EXECUTION_GENESIS_BLOCK_HASH` in the network config, the flags are
rejected and the node behaves as today apart from the schema.

## 8. Risks and open questions

- **Shared fork digest with devnet-5.** Per D3 the digest does not isolate
  an EL build from devnet-5 peers. Operators must never reuse a devnet-5
  bootnode list for an EL network; the symptom would be a log full of block
  decode failures and status mismatches, not a silent fork.
- **Upstream schema divergence.** If leanSpec adopts a different shape,
  Phase 1 types and the fork pin must be redone. Keeping the shape verbatim
  Cancun minimises that risk.
- **Block size.** A full 30M-gas block is several MiB of transactions inside
  a gossip cap of 10 MiB and an SSZ list limit far above that. The devnet
  gas limit bounds it; a production gas limit is a separate decision.
- **Prague and beyond.** V4 needs `executionRequests` in the block. Out of
  scope; the client interface leaves room.
- **Optimistic import.** `SYNCING` blocks are imported and can become head.
  Mainnet clients suppress proposals on an optimistic head; we do not in v1.
  Flagged for Phase 7.
- **Timestamp granularity.** Lean slots are 4 s, geth requires strictly
  increasing timestamps; fine, but a slot gap larger than one slot yields a
  timestamp jump, which geth accepts.

## 9. Sequence and rough sizes

| Phase | Depends on | Size | Gate |
|---|---|---|---|
| 0 spec fork | none | S | fixtures regenerate |
| 1 schema + STF | 0 | M | `make test-spec` green |
| 2 client | none | M | unit + httptest |
| 3 startup | 2 | S | handshake + genesis check |
| 4 proposal + FCU | 1, 3 | M | single-node live run |
| 5 import gating | 4 | M | mock verdict matrix + live |
| 6 devnet + interop | 5 | M | three-node run, failure drills |
| 7 hardening + docs | 6 | S | review |

Phases 0 and 2 can proceed in parallel.
