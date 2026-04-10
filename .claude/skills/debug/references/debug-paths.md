# Debug Paths

Map symptoms to the smallest likely gean subsystem before reading code.

## Symptom to subsystem

| Symptom | Inspect first | Notes |
|---|---|---|
| Peers do not connect, gossip is quiet, req/resp hangs | `p2p/host.go`, `p2p/gossip.go`, `p2p/reqresp.go`, `p2p/peers.go` | Usually transport, topic, peer, or startup-flag issues before consensus logic |
| Block received but rejected or not imported | `node/store_block.go`, `node/store_validate.go`, `statetransition/block.go`, `statetransition/transition.go` | Start from the first rejected block or validation error |
| Head does not advance or reorg behavior looks wrong | `forkchoice/forkchoice.go`, `forkchoice/protoarray.go`, `forkchoice/votes.go`, `node/store_tick.go` | Compare head, justified, and finalized progress first |
| Finalization stalls | `forkchoice/`, `node/store_aggregate.go`, `node/store_produce.go`, `node/validator.go` | Check whether attestations are present and counted before changing finalization logic |
| gean produces invalid blocks on devnet | `node/store_build.go`, `node/store_produce.go`, `node/block.go`, `types/` | Correlate proposer slot and attestation count with peer rejection |
| Deterministic spec fixture failure | `spectests/`, `statetransition/`, `forkchoice/`, `types/` | Treat as logic mismatch with leanSpec, not a networking issue |
| Checkpoint sync fetches wrong state or resumes incorrectly | `checkpoint/checkpoint.go`, `api/server.go`, `node/node.go`, `storage/pebble.go` | Verify the source endpoint before blaming the syncing node |
| Data survives incorrectly across restarts or state disappears | `storage/pebble.go`, `storage/keys.go`, `storage/tables.go`, `node/consensus_store.go` | Persistence bugs often show up as missing parents or inconsistent finalized state |
| Signature verification or aggregation fails | `xmss/ffi.go`, `xmss/keys.go`, `xmss/proof_pool.go`, `node/verify.go` | Separate Go marshaling bugs from Rust FFI failures |
| API output is wrong but chain logic seems healthy | `api/server.go`, `checkpoint/checkpoint.go` | Distinguish serialization/output bugs from consensus bugs |

## Fast package map

| Package | Responsibility |
|---|---|
| `cmd/gean` | Node startup and flag wiring |
| `node/` | Main event loop, block handling, validator duties, aggregation |
| `forkchoice/` | Head selection, proto-array state, votes |
| `statetransition/` | Block and slot processing, validation rules |
| `p2p/` | Gossip, req/resp, peer connectivity, host setup |
| `storage/` | Pebble and in-memory storage backends |
| `checkpoint/` | Checkpoint sync and finalized state handling |
| `api/` | HTTP endpoints for health, finalized state, checkpoints, fork choice |
| `types/` | Core data structures and SSZ encoding |
| `xmss/` | FFI bridge, key handling, aggregate proof logic |
| `spectests/` | leanSpec fixture-backed conformance tests |

## Choose the smallest useful reproduction

- `make test` for regular Go regressions
- `go test ./<package> -run TestName -v -count=1` for package-local failures
- `make test-spec` for leanSpec fixture mismatches
- `make test-ffi` for XMSS/Rust FFI issues
- `./.claude/skills/devnet-log-review/scripts/analyze-logs.sh` when logs already exist
- `devnet-runner` when you need a fresh multi-client reproduction
