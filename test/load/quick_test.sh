#!/bin/bash

# Quick Test Script - Simple wrapper for common operations

set -e

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${BLUE}🚀 Quick Accumulate Load Test${NC}"
echo "================================"

case "${1:-quick}" in
    "quick"|"")
        echo "Running quick test against existing devnet..."
        ./devnet_manager.sh test
        ;;
    "fresh")
        echo "Full restart with fresh devnet..."
        ./devnet_manager.sh restart
        ;;
    "status")
        echo "Checking devnet status..."
        ./devnet_manager.sh status
        ;;
    *)
        echo "Usage: $0 [quick|fresh|status]"
        echo ""
        echo "  quick  - Run tests against existing devnet (default)"
        echo "  fresh  - Kill + restart + test everything"  
        echo "  status - Show devnet status"
        exit 1
        ;;
esac

echo -e "${GREEN}✅ Done!${NC}"