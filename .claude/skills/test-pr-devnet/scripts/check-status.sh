#!/bin/bash

# Quick devnet status check for the gean 5-client test

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${BLUE}=== Devnet Status ===${NC}"
echo ""

# Check running nodes
echo "Running nodes:"
docker ps --format "  {{.Names}}: {{.Status}}" --filter "name=_0"
echo ""

# Check each node's latest status
for node in gean_0 zeam_0 ream_0 lantern_0 ethlambda_0; do
    if docker ps --format "{{.Names}}" | grep -q "^$node$"; then
        echo -e "${GREEN}$node${NC}:"

        case $node in
            gean_0)
                docker logs gean_0 2>&1 | tail -300 | grep "CHAIN STATUS" | tail -1 | sed 's/^/  /'
                docker logs gean_0 2>&1 | tail -300 | grep "Latest Finalized:" | tail -1 | sed 's/^/  /'
                ;;
            zeam_0)
                docker logs zeam_0 2>&1 | tail -300 | grep -i "finalized\|head_slot" | tail -1 | sed 's/^/  /'
                ;;
            ream_0)
                docker logs ream_0 2>&1 | tail -300 | grep -i "finalized\|Processing block" | tail -1 | sed 's/^/  /'
                ;;
            lantern_0)
                docker logs lantern_0 2>&1 | tail -300 | grep -i "imported\|finalized" | tail -1 | sed 's/^/  /'
                ;;
            ethlambda_0)
                docker logs ethlambda_0 2>&1 | grep "Fork choice head updated" | tail -1 | sed 's/^/  /'
                ;;
        esac
        echo ""
    fi
done

# gean specifics
if docker ps --format "{{.Names}}" | grep -q "^gean_0$"; then
    echo "gean key metrics:"

    # Max attestations per block (block bloat regression check)
    MAX_ATTS=$(docker logs gean_0 2>&1 | grep -oE "attestations=[0-9]+" | grep -oE "[0-9]+" | sort -n | tail -1)
    MAX_ATTS=${MAX_ATTS:-0}
    if [[ "$MAX_ATTS" -gt 30 ]]; then
        echo -e "  Max attestations/block: ${RED}$MAX_ATTS${NC} ⚠ regression risk"
    else
        echo -e "  Max attestations/block: ${GREEN}$MAX_ATTS${NC}"
    fi

    # Block bloat error count
    SIZE_ERRORS=$(docker logs gean_0 2>&1 | grep -cE "MessageTooLarge|exceeds max" 2>/dev/null | head -1 | tr -d ' \n')
    SIZE_ERRORS=${SIZE_ERRORS:-0}
    if [[ "$SIZE_ERRORS" -eq 0 ]]; then
        echo -e "  Oversized block errors: ${GREEN}0${NC}"
    else
        echo -e "  Oversized block errors: ${RED}$SIZE_ERRORS${NC} ⚠ regression"
    fi

    echo ""
fi

# Quick error check
echo "Error counts:"
for node in gean_0 zeam_0 ream_0 lantern_0 ethlambda_0; do
    if docker ps --format "{{.Names}}" | grep -q "^$node$"; then
        COUNT=$(docker logs "$node" 2>&1 | grep -c "ERROR" 2>/dev/null | head -1 | tr -d ' \n')
        COUNT=${COUNT:-0}
        if [[ "$COUNT" -eq 0 ]]; then
            echo -e "  $node: ${GREEN}$COUNT${NC}"
        else
            echo -e "  $node: ${RED}$COUNT${NC}"
        fi
    fi
done
