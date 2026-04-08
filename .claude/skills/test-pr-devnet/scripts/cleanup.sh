#!/bin/bash

# Cleanup devnet and restore configurations

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GEAN_ROOT="$(cd "$SCRIPT_DIR/../../../.." && pwd)"
LEAN_QUICKSTART="${LEAN_QUICKSTART:-$GEAN_ROOT/lean-quickstart}"
GEAN_CMD="$LEAN_QUICKSTART/client-cmds/gean-cmd.sh"

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${BLUE}=== Devnet Cleanup ===${NC}"
echo ""

# Stop devnet
if [[ -d "$LEAN_QUICKSTART" ]]; then
    echo "Stopping devnet..."
    cd "$LEAN_QUICKSTART"
    NETWORK_DIR=local-devnet ./spin-node.sh --node all --stop 2>/dev/null || true
fi

# Force remove containers
echo "Removing containers..."
docker rm -f zeam_0 ream_0 lantern_0 ethlambda_0 gean_0 2>/dev/null || true

echo -e "${GREEN}✓ Devnet stopped${NC}"
echo ""

# Restore config if backup exists
if [[ -f "$GEAN_CMD.backup" ]]; then
    echo "Restoring gean-cmd.sh..."
    mv "$GEAN_CMD.backup" "$GEAN_CMD"
    echo -e "${GREEN}✓ Config restored${NC}"
else
    echo "No backup found, skipping config restore"
fi

echo ""
echo "Cleanup complete!"
