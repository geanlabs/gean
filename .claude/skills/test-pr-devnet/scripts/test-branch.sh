#!/bin/bash
set -euo pipefail

# Test gean branch in multi-client devnet
# Usage: ./test-branch.sh [branch-name] [--with-sync-test]

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GEAN_ROOT="$(cd "$SCRIPT_DIR/../../../.." && pwd)"
LEAN_QUICKSTART="${LEAN_QUICKSTART:-$GEAN_ROOT/lean-quickstart}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Parse arguments
BRANCH_NAME=""
WITH_SYNC_TEST=false

# First positional arg is branch name (if not a flag)
for arg in "$@"; do
    if [[ "$arg" == "--with-sync-test" ]]; then
        WITH_SYNC_TEST=true
    elif [[ -z "$BRANCH_NAME" ]]; then
        BRANCH_NAME="$arg"
    fi
done

# Default to current branch if not specified
if [[ -z "$BRANCH_NAME" ]]; then
    BRANCH_NAME=$(git -C "$GEAN_ROOT" rev-parse --abbrev-ref HEAD)
fi

echo -e "${BLUE}=== gean Devnet Testing ===${NC}"
echo ""
echo "Branch:          $BRANCH_NAME"
echo "Sync test:       $WITH_SYNC_TEST"
echo "gean root:       $GEAN_ROOT"
echo "lean-quickstart: $LEAN_QUICKSTART"
echo ""

# Validate prerequisites
echo "Validating prerequisites..."

if [[ ! -d "$LEAN_QUICKSTART" ]]; then
    echo -e "${RED}✗ Error: lean-quickstart not found at $LEAN_QUICKSTART${NC}"
    echo "  Set LEAN_QUICKSTART environment variable or run:"
    echo "  cd $GEAN_ROOT && make lean-quickstart"
    exit 1
fi

if [[ ! -f "$LEAN_QUICKSTART/spin-node.sh" ]]; then
    echo -e "${RED}✗ Error: spin-node.sh not found in lean-quickstart${NC}"
    exit 1
fi

if ! docker info &>/dev/null; then
    echo -e "${RED}✗ Error: Docker is not running${NC}"
    echo "  Start Docker Desktop or docker daemon"
    exit 1
fi

# Use `git rev-parse` instead of `-d .git` to support git worktrees,
# where .git is a file (not a directory) pointing to the main repo.
if ! git -C "$GEAN_ROOT" rev-parse --git-dir &>/dev/null; then
    echo -e "${RED}✗ Error: Not in a git repository${NC}"
    echo "  Run this script from gean repository root"
    exit 1
fi

echo -e "${GREEN}✓ Prerequisites validated${NC}"
echo ""

# Step 1: Build Docker image
echo -e "${BLUE}[1/6] Building Docker image...${NC}"
cd "$GEAN_ROOT"
GIT_COMMIT=$(git rev-parse HEAD)

docker build \
    --build-arg GIT_COMMIT="$GIT_COMMIT" \
    --build-arg GIT_BRANCH="$BRANCH_NAME" \
    -t "gean:$BRANCH_NAME" \
    .

echo -e "${GREEN}✓ Image built: gean:$BRANCH_NAME${NC}"
echo ""

# Step 2: Update gean-cmd.sh
echo -e "${BLUE}[2/6] Updating lean-quickstart config...${NC}"
GEAN_CMD="$LEAN_QUICKSTART/client-cmds/gean-cmd.sh"

if [[ ! -f "$GEAN_CMD" ]]; then
    echo -e "${RED}✗ Error: $GEAN_CMD not found${NC}"
    echo "  lean-quickstart may not have a gean entry yet."
    exit 1
fi

# Backup original
cp "$GEAN_CMD" "$GEAN_CMD.backup"

# Update docker tag
sed -i.tmp "s|gean:[^ ]*|gean:$BRANCH_NAME|" "$GEAN_CMD"
rm "$GEAN_CMD.tmp"

echo -e "${GREEN}✓ Updated $GEAN_CMD${NC}"
echo "  (Backup saved as $GEAN_CMD.backup)"
echo ""

# Step 3: Stop any existing devnet
echo -e "${BLUE}[3/6] Cleaning up existing devnet...${NC}"
cd "$LEAN_QUICKSTART"
NETWORK_DIR=local-devnet ./spin-node.sh --node all --stop 2>/dev/null || true
docker rm -f zeam_0 ream_0 lantern_0 ethlambda_0 gean_0 2>/dev/null || true

echo -e "${GREEN}✓ Cleanup complete${NC}"
echo ""

# Step 4: Start devnet
echo -e "${BLUE}[4/6] Starting devnet...${NC}"
echo "This will take ~40 seconds (genesis generation + startup)"
echo ""

# Run devnet in background
NETWORK_DIR=local-devnet ./spin-node.sh --node all --generateGenesis --metrics > "/tmp/devnet-$BRANCH_NAME.log" 2>&1 &
DEVNET_PID=$!

# Wait for nodes to start (check docker ps)
echo -n "Waiting for nodes to start"
for i in {1..40}; do
    sleep 1
    echo -n "."
    running=$(docker ps --filter "name=_0" --format "{{.Names}}" | grep -cE '^(gean|zeam|ream|lantern|ethlambda)_0$' || echo 0)
    if [[ "$running" -ge 5 ]]; then
        echo ""
        echo -e "${GREEN}✓ All 5 nodes running${NC}"
        break
    fi
done
echo ""

# Show node status
docker ps --format "  {{.Names}}: {{.Status}}" --filter "name=_0"
echo ""

# Step 5: Sync recovery test (optional)
if [[ "$WITH_SYNC_TEST" == "true" ]]; then
    echo -e "${BLUE}[5/6] Testing sync recovery...${NC}"

    # Let devnet run for a bit
    echo "Letting devnet run for 10 seconds..."
    sleep 10

    # Pause nodes
    echo "Pausing zeam_0 and ream_0..."
    docker pause zeam_0 ream_0
    echo -e "${YELLOW}⏸  Nodes paused${NC}"

    # Wait for network to progress
    echo "Network progressing for 20 seconds (~5 slots)..."
    sleep 20

    # Unpause
    echo "Unpausing nodes..."
    docker unpause zeam_0 ream_0
    echo -e "${GREEN}▶  Nodes resumed${NC}"

    # Wait for sync
    echo "Waiting 10 seconds for sync recovery..."
    sleep 10

    echo -e "${GREEN}✓ Sync recovery test complete${NC}"
    echo ""
else
    echo -e "${BLUE}[5/6] Skipping sync recovery test${NC}"
    echo "Use --with-sync-test to enable"
    echo ""

    # Just let it run for a bit
    echo "Letting devnet run for 30 seconds..."
    sleep 30
fi

# Step 6: Analyze results
echo -e "${BLUE}[6/6] Analyzing results...${NC}"
echo ""

# Quick status check
echo "=== Quick Status ==="
echo ""

# Check each node
for node in zeam_0 ream_0 lantern_0 ethlambda_0 gean_0; do
    if docker ps --format "{{.Names}}" | grep -q "^$node$"; then
        echo -e "${GREEN}✓${NC} $node: Running"
    else
        echo -e "${RED}✗${NC} $node: Not running"
    fi
done
echo ""

# Check gean specifics
echo "=== gean Status ==="
echo ""

# Get latest chain status
LATEST_STATUS=$(docker logs gean_0 2>&1 | grep "CHAIN STATUS" | tail -1 || echo "No chain status found")
echo "$LATEST_STATUS"

# Latest finalized
LATEST_FIN=$(docker logs gean_0 2>&1 | grep "Latest Finalized:" | tail -1 || echo "")
[[ -n "$LATEST_FIN" ]] && echo "$LATEST_FIN"
echo ""

# Count blocks
BLOCKS_PROPOSED=$(docker logs gean_0 2>&1 | grep -c "\[validator\] proposed block" || echo "0")
BLOCKS_PROPOSED=$(echo "$BLOCKS_PROPOSED" | tr -d '\n' | xargs)
echo "Blocks proposed: $BLOCKS_PROPOSED"

# Max attestations per block (regression check)
MAX_ATTS=$(docker logs gean_0 2>&1 | grep -oE "attestations=[0-9]+" | grep -oE "[0-9]+" | sort -n | tail -1 || echo "0")
echo "Max attestations per block: $MAX_ATTS"
if [[ "${MAX_ATTS:-0}" -gt 30 ]]; then
    echo -e "  ${RED}⚠ WARNING: attestations > 30 — possible block bloat regression${NC}"
fi

# Count errors
ERROR_COUNT=$(docker logs gean_0 2>&1 | grep -c "ERROR" || echo "0")
ERROR_COUNT=$(echo "$ERROR_COUNT" | tr -d '\n' | xargs)
if [[ "$ERROR_COUNT" -eq 0 ]]; then
    echo -e "Errors: ${GREEN}$ERROR_COUNT${NC}"
else
    echo -e "Errors: ${RED}$ERROR_COUNT${NC}"
fi

# Critical regression check: MessageTooLarge / oversized blocks
SIZE_ERRORS=$(docker logs gean_0 2>&1 | grep -cE "MessageTooLarge|exceeds max" || echo "0")
SIZE_ERRORS=$(echo "$SIZE_ERRORS" | tr -d '\n' | xargs)
if [[ "$SIZE_ERRORS" -eq 0 ]]; then
    echo -e "Oversized block errors: ${GREEN}0${NC}"
else
    echo -e "Oversized block errors: ${RED}$SIZE_ERRORS${NC} ${RED}⚠ REGRESSION${NC}"
fi
echo ""

# Sync stats (if sync test was run)
if [[ "$WITH_SYNC_TEST" == "true" ]]; then
    echo "=== Sync Activity ==="
    echo ""

    BATCHED=$(docker logs gean_0 2>&1 | grep -c "batched fetch starting" || echo "0")
    BATCHED=$(echo "$BATCHED" | tr -d '\n' | xargs)
    QUEUED=$(docker logs gean_0 2>&1 | grep -c "queueing missing block" || echo "0")
    QUEUED=$(echo "$QUEUED" | tr -d '\n' | xargs)
    EXHAUSTED=$(docker logs gean_0 2>&1 | grep -c "fetch exhausted for root" || echo "0")
    EXHAUSTED=$(echo "$EXHAUSTED" | tr -d '\n' | xargs)

    echo "Batched fetches issued: $BATCHED"
    echo "Roots queued for fetch: $QUEUED"
    echo "Fetches exhausted:      $EXHAUSTED"
    echo ""
fi

# Final verdict
echo "=== Test Result ==="
echo ""
if [[ "$ERROR_COUNT" -eq 0 ]] && [[ "$SIZE_ERRORS" -eq 0 ]] && [[ "$BLOCKS_PROPOSED" -gt 0 ]]; then
    echo -e "${GREEN}✓ PASSED${NC} - Devnet running successfully"
elif [[ "$SIZE_ERRORS" -gt 0 ]]; then
    echo -e "${RED}✗ FAILED${NC} - Block bloat regression detected"
else
    echo -e "${YELLOW}⚠ CHECK LOGS${NC} - Some issues detected"
fi
echo ""

# Next steps
echo "=== Next Steps ==="
echo ""
echo "Check detailed logs:"
echo "  docker logs gean_0 2>&1 | less"
echo ""
echo "Run log analysis:"
echo "  cd $GEAN_ROOT"
echo "  .claude/skills/devnet-log-review/scripts/analyze-logs.sh"
echo ""
echo "Stop devnet:"
echo "  $SCRIPT_DIR/cleanup.sh"
echo ""

# Keep devnet running
echo -e "${YELLOW}Devnet is still running. Stop it when done testing.${NC}"
