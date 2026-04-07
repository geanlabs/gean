---
name: test-pr-devnet
description: Test gean PR changes in multi-client devnet. Use when users want to (1) Test a branch/PR with other Lean clients, (2) Validate BlocksByRoot or P2P protocol changes, (3) Test sync recovery with pause/unpause, (4) Verify cross-client interoperability, (5) Run integration tests before merging.
disable-model-invocation: true
---

# Test PR in Devnet

Test gean branch changes in a multi-client local devnet with zeam (Zig),
ream (Rust), lantern (C), ethlambda (Rust), and gean itself.

## Quick Start

```bash
# Test current branch (basic interoperability, ~60-90s)
.claude/skills/test-pr-devnet/scripts/test-branch.sh

# Test with sync recovery (BlocksByRoot validation, ~90-120s)
.claude/skills/test-pr-devnet/scripts/test-branch.sh --with-sync-test

# Test specific branch
.claude/skills/test-pr-devnet/scripts/test-branch.sh my-feature-branch

# Check status while running
.claude/skills/test-pr-devnet/scripts/check-status.sh

# Cleanup when done
.claude/skills/test-pr-devnet/scripts/cleanup.sh
```

## What It Does

1. **Builds branch-specific Docker image** tagged as `gean:<branch-name>`
2. **Updates lean-quickstart config** to use the new image (backs up original)
3. **Starts 5-node devnet** with fresh genesis (zeam, ream, lantern, ethlambda, gean)
4. **Optionally tests sync recovery** by pausing/unpausing nodes
5. **Analyzes results** and provides summary
6. **Leaves devnet running** for manual inspection

## Why 5 Clients

The 5-client setup is gean's standard test surface:

| Client | Language | Why included |
|---|---|---|
| zeam | Zig | Different SSZ implementation, catches encoding bugs |
| ream | Rust | Different libp2p stack |
| lantern | C | Most reliable client (gold standard) |
| ethlambda | Rust | Best fork choice viz, broadest interop |
| gean | Go | The system under test |

qlean is excluded because it's historically unreliable
(`listen_addrs=0` config bug, frequent disconnects, no log shipping).

## Prerequisites

| Requirement | Location | Check |
|---|---|---|
| lean-quickstart | `<repo-root>/lean-quickstart` | `ls $LEAN_QUICKSTART` |
| Docker running | — | `docker ps` |
| Git repository | gean repo root | `git branch` |

## Test Scenarios

### Basic Interoperability (~60-90s)

**Goal:** Verify gean produces blocks and reaches consensus with other clients

**Success criteria:**
- ✅ No errors in gean logs
- ✅ All 5 nodes at same head slot
- ✅ Finalization advancing (every 6-12 slots)
- ✅ Each validator produces blocks for their slots
- ✅ gean blocks have `attestations=` count ≤ ~6 (per-validator selection)

### Sync Recovery (~90-120s)

**Goal:** Test BlocksByRoot batched fetch when nodes fall behind

**Usage:** Add `--with-sync-test` flag

**What happens:**
1. Devnet runs for 10s (~2-3 slots)
2. Pauses `zeam_0` and `ream_0`
3. Network progresses 20s (~5 slots)
4. Unpauses nodes → nodes sync via batched `blocks_by_root`

**Success criteria:**
- ✅ gean's `[sync] batched fetch starting count=N` lines appear
- ✅ Paused nodes sync to current head
- ✅ No `MessageTooLarge` or oversized block errors

## Configuration Changes

The skill modifies `lean-quickstart/client-cmds/gean-cmd.sh` to use your
branch's Docker image.

**Automatic backup:** Creates `gean-cmd.sh.backup`

**Restore methods:**
```bash
# 1. Cleanup script (recommended)
.claude/skills/test-pr-devnet/scripts/cleanup.sh

# 2. Manual restore
mv $LEAN_QUICKSTART/client-cmds/gean-cmd.sh.backup \
   $LEAN_QUICKSTART/client-cmds/gean-cmd.sh

# 3. Git restore (if no uncommitted changes)
cd $LEAN_QUICKSTART && git checkout client-cmds/gean-cmd.sh
```

## Manual Workflow (Alternative to Script)

If you need fine-grained control:

### 1. Build Image

```bash
cd <gean-repo-root>
BRANCH=$(git rev-parse --abbrev-ref HEAD)
docker build \
    --build-arg GIT_COMMIT=$(git rev-parse HEAD) \
    --build-arg GIT_BRANCH=$BRANCH \
    -t gean:$BRANCH .
```

### 2. Update Configuration

Edit `$LEAN_QUICKSTART/client-cmds/gean-cmd.sh`:
```bash
node_docker="gean:<your-branch> \
```

### 3. Start Devnet

```bash
cd $LEAN_QUICKSTART
NETWORK_DIR=local-devnet ./spin-node.sh --node all --generateGenesis --metrics
```

### 4. Test Sync (Optional)

```bash
# Create sync gap
docker pause zeam_0 ream_0
sleep 20  # Network progresses

# Test recovery
docker unpause zeam_0 ream_0
sleep 10  # Wait for sync
```

### 5. Check Results

```bash
# Quick status
.claude/skills/test-pr-devnet/scripts/check-status.sh

# Detailed analysis (use devnet-log-review skill)
.claude/skills/devnet-log-review/scripts/analyze-logs.sh
```

## Protocol Compatibility

| Client | Status | Gossipsub | BlocksByRoot |
|---|---|---|---|
| zeam | ✅ Full | ✅ Full | ⚠️ SSZ bugs known |
| ream | ✅ Full | ✅ Full | ⚠️ Sync recovery fragile |
| lantern | ✅ Full | ✅ Full | ✅ Full |
| ethlambda | ✅ Full | ✅ Full | ✅ Full (but block bloat bug) |
| gean | ✅ Full | ✅ Full | ✅ Full (batched, up to 10 roots/req) |

**Notes:**
- zeam may crash with SSZ panics — not gean's fault
- ethlambda may emit MessageTooLarge during stalls — known upstream bug
- lantern is the most reliable peer for `blocks_by_root` responses

## Verification Checklist

| Check | Command | Expected |
|---|---|---|
| All nodes running | `docker ps --filter "name=_0"` | 5 containers |
| gean peers connected | `docker logs gean_0 \| grep "Connected Peers:" \| tail -1` | > 0 |
| Blocks produced | `docker logs gean_0 \| grep "\\[validator\\] proposed block" \| wc -l` | > 0 |
| No errors | `docker logs gean_0 \| grep -i ERROR \| wc -l` | 0 |
| Bounded blocks | `docker logs gean_0 \| grep -oE 'attestations=[0-9]+' \| sort -u` | values ≤ ~10 |
| No oversized cascades | `docker logs gean_0 \| grep "MessageTooLarge\|exceeds max"` | empty |

## Troubleshooting

### Build Fails
```bash
docker ps  # Check Docker running
docker system prune -a  # Clean cache if needed
```

### Nodes Won't Start
```bash
# Clean and retry
docker stop zeam_0 ream_0 lantern_0 ethlambda_0 gean_0 2>/dev/null
docker rm   zeam_0 ream_0 lantern_0 ethlambda_0 gean_0 2>/dev/null
cd $LEAN_QUICKSTART
NETWORK_DIR=local-devnet ./spin-node.sh --node all --generateGenesis
```

### Genesis Mismatch
```bash
cd $LEAN_QUICKSTART
NETWORK_DIR=local-devnet ./spin-node.sh --node all --cleanData --generateGenesis
```

### Image Tag Not Updated
```bash
# Verify the change
grep "node_docker=" $LEAN_QUICKSTART/client-cmds/gean-cmd.sh
# Should show your branch name, not :dev
```

### Port Already in Use
```bash
docker stop $(docker ps -q --filter "name=_0") 2>/dev/null || true
```

## Debugging

### gean per-block analysis

```bash
# Check gean's processing time per block
docker logs gean_0 2>&1 | grep -oE "proc_time=[0-9]+ms" | sort -u

# Check attestation count distribution
docker logs gean_0 2>&1 | grep -oE "attestations=[0-9]+" | sort | uniq -c

# Check for has_parent=false (orphan blocks)
docker logs gean_0 2>&1 | grep "has_parent=false" | wc -l

# Check sync events
docker logs gean_0 2>&1 | grep "\\[sync\\]"
```

### Cross-client finalization comparison

```bash
for node in zeam_0 ream_0 lantern_0 ethlambda_0 gean_0; do
    echo "$node:"
    docker logs "$node" 2>&1 | grep -i "finalized" | tail -1
done
```

### Devnet Status Checks

```bash
# Check all nodes are running
docker ps --format "{{.Names}}: {{.Status}}" --filter "name=_0"

# Get gean chain status
docker logs gean_0 2>&1 | tail -200 | grep "CHAIN STATUS" | tail -1

# Get gean peer count
docker logs gean_0 2>&1 | grep "Connected Peers:" | tail -1
```

### Common Investigation Patterns

```bash
# Verify gean is proposing blocks
docker logs gean_0 2>&1 | grep "\\[validator\\] proposing block\|\\[validator\\] proposed block"

# Verify gean is aggregating signatures
docker logs gean_0 2>&1 | grep "\\[signature\\] aggregate:" | head

# Check peer discovery
docker logs gean_0 2>&1 | grep -i "peer\|connection" | head -20
```

## References

- **[gean Makefile](../../../Makefile)** — Build and run targets
- **[devnet-log-review skill](../../devnet-log-review/SKILL.md)** — Comprehensive log analysis
- **[devnet-runner skill](../../devnet-runner/SKILL.md)** — Devnet management
