#!/bin/bash
# Load Testing Docker Management Script
#
# This script provides convenient commands for managing the load testing
# Docker environment.

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
COMPOSE_FILE="${SCRIPT_DIR}/docker-compose.yml"
COMPOSE_DISTRIBUTED="${SCRIPT_DIR}/docker-compose.distributed.yml"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

print_usage() {
    cat <<EOF
Load Testing Docker Management

Usage: $0 [command] [options]

Commands:
    start [mode]         Start the network (mode: simple|distributed, default: simple)
    stop [mode]          Stop the network
    restart [mode]       Restart the network
    logs [mode] [node]   View logs (optionally filter by node)
    status [mode]        Show container status
    clean [mode]         Stop and remove all containers and volumes
    build [mode]         Build Docker images
    shell [mode] [node]  Open shell in container (distributed mode only)
    stats                Show resource usage
    health               Check network health
    help                 Show this help message

Examples:
    $0 start                    # Start simple mode
    $0 start distributed        # Start distributed mode
    $0 logs                     # View all logs (simple mode)
    $0 logs distributed bvn0-0  # View bvn0-0 logs (distributed)
    $0 shell distributed bvn1-2 # Shell into bvn1-2
    $0 clean                    # Clean up everything

EOF
}

get_compose_file() {
    local mode=${1:-simple}
    if [ "$mode" = "distributed" ]; then
        echo "$COMPOSE_DISTRIBUTED"
    else
        echo "$COMPOSE_FILE"
    fi
}

cmd_start() {
    local mode=${1:-simple}
    local compose=$(get_compose_file "$mode")

    echo -e "${BLUE}Starting load testing network in $mode mode...${NC}"
    docker compose -f "$compose" up -d

    echo -e "${GREEN}Network started successfully!${NC}"
    echo ""
    cmd_status "$mode"
}

cmd_stop() {
    local mode=${1:-simple}
    local compose=$(get_compose_file "$mode")

    echo -e "${YELLOW}Stopping load testing network...${NC}"
    docker compose -f "$compose" down
    echo -e "${GREEN}Network stopped.${NC}"
}

cmd_restart() {
    local mode=${1:-simple}
    cmd_stop "$mode"
    sleep 2
    cmd_start "$mode"
}

cmd_logs() {
    local mode=${1:-simple}
    local node=${2:-}
    local compose=$(get_compose_file "$mode")

    if [ -n "$node" ]; then
        docker compose -f "$compose" logs -f "$node"
    else
        docker compose -f "$compose" logs -f
    fi
}

cmd_status() {
    local mode=${1:-simple}
    local compose=$(get_compose_file "$mode")

    echo -e "${BLUE}Container Status:${NC}"
    docker compose -f "$compose" ps
}

cmd_clean() {
    local mode=${1:-simple}
    local compose=$(get_compose_file "$mode")

    echo -e "${RED}WARNING: This will delete all containers and volumes!${NC}"
    read -p "Are you sure? (y/N) " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        docker compose -f "$compose" down -v
        echo -e "${GREEN}Cleanup complete.${NC}"
    else
        echo "Cancelled."
    fi
}

cmd_build() {
    local mode=${1:-simple}
    local compose=$(get_compose_file "$mode")

    echo -e "${BLUE}Building Docker images...${NC}"
    docker compose -f "$compose" build
    echo -e "${GREEN}Build complete.${NC}"
}

cmd_shell() {
    local mode=${1:-simple}
    local node=${2:-load-testnet}
    local compose=$(get_compose_file "$mode")

    if [ "$mode" = "simple" ]; then
        node="load-testnet"
    fi

    echo -e "${BLUE}Opening shell in $node...${NC}"
    docker compose -f "$compose" exec "$node" /bin/bash
}

cmd_stats() {
    echo -e "${BLUE}Resource Usage:${NC}"
    docker stats --no-stream --format "table {{.Container}}\t{{.CPUPerc}}\t{{.MemUsage}}\t{{.NetIO}}" \
        $(docker ps --filter "name=load-test" --format "{{.Names}}")
}

cmd_health() {
    echo -e "${BLUE}Checking network health...${NC}"

    # Try simple mode first
    if curl -sf http://localhost:8080/v3/describe > /dev/null 2>&1; then
        echo -e "${GREEN}✓ BVN0 API responding${NC}"
    else
        echo -e "${RED}✗ BVN0 API not responding${NC}"
    fi

    if curl -sf http://localhost:8081/v3/describe > /dev/null 2>&1; then
        echo -e "${GREEN}✓ BVN1 API responding${NC}"
    else
        echo -e "${YELLOW}⚠ BVN1 API not responding${NC}"
    fi

    if curl -sf http://localhost:8082/v3/describe > /dev/null 2>&1; then
        echo -e "${GREEN}✓ BVN2 API responding${NC}"
    else
        echo -e "${YELLOW}⚠ BVN2 API not responding${NC}"
    fi
}

# Main command dispatcher
case "${1:-help}" in
    start)
        cmd_start "${2:-simple}"
        ;;
    stop)
        cmd_stop "${2:-simple}"
        ;;
    restart)
        cmd_restart "${2:-simple}"
        ;;
    logs)
        cmd_logs "${2:-simple}" "${3:-}"
        ;;
    status)
        cmd_status "${2:-simple}"
        ;;
    clean)
        cmd_clean "${2:-simple}"
        ;;
    build)
        cmd_build "${2:-simple}"
        ;;
    shell)
        cmd_shell "${2:-simple}" "${3:-}"
        ;;
    stats)
        cmd_stats
        ;;
    health)
        cmd_health
        ;;
    help|--help|-h)
        print_usage
        ;;
    *)
        echo -e "${RED}Unknown command: $1${NC}"
        echo ""
        print_usage
        exit 1
        ;;
esac
