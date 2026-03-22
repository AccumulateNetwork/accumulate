#!/bin/bash
# Cleanup and Reset Script for Load Testing Environment
#
# This script cleans up all artifacts from load testing:
#   - Docker containers, volumes, and networks
#   - Generated test data and logs
#   - Monitoring logs and alerts
#   - Optionally: test wallet
#
# Usage:
#   ./cleanup.sh [options]

set -e

# Directories
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
DOCKER_DIR="$PROJECT_ROOT/test/docker"
WALLET_DIR="$HOME/.accumulate"
MONITOR_DIR="/tmp"

# Cleanup options
CLEANUP_DOCKER=true
CLEANUP_LOGS=true
CLEANUP_WALLET=false
CLEANUP_VOLUMES=true
CLEANUP_NETWORK=true
DRY_RUN=false
FORCE=false

# Colors
RED='\033[0;31m'
YELLOW='\033[1;33m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'

print_usage() {
    cat <<EOF
Cleanup and Reset Script for Load Testing

Usage: $0 [options]

Options:
    --all                   Clean everything (equivalent to all flags)
    --docker                Clean Docker containers and images (default: yes)
    --no-docker             Skip Docker cleanup
    --logs                  Clean log files (default: yes)
    --no-logs               Skip log cleanup
    --wallet                Clean test wallet (default: no)
    --volumes               Clean Docker volumes (default: yes)
    --no-volumes            Keep Docker volumes
    --network               Clean Docker networks (default: yes)
    --dry-run               Show what would be cleaned without doing it
    --force                 Skip confirmation prompts
    -h, --help              Show this help

Examples:
    $0                      # Standard cleanup (Docker, logs, volumes)
    $0 --all --force        # Complete cleanup without prompts
    $0 --docker --logs      # Only Docker and logs
    $0 --dry-run            # Preview cleanup actions
    $0 --wallet --force     # Include wallet cleanup

EOF
}

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --all)
            CLEANUP_DOCKER=true
            CLEANUP_LOGS=true
            CLEANUP_WALLET=true
            CLEANUP_VOLUMES=true
            CLEANUP_NETWORK=true
            shift
            ;;
        --docker)
            CLEANUP_DOCKER=true
            shift
            ;;
        --no-docker)
            CLEANUP_DOCKER=false
            shift
            ;;
        --logs)
            CLEANUP_LOGS=true
            shift
            ;;
        --no-logs)
            CLEANUP_LOGS=false
            shift
            ;;
        --wallet)
            CLEANUP_WALLET=true
            shift
            ;;
        --volumes)
            CLEANUP_VOLUMES=true
            shift
            ;;
        --no-volumes)
            CLEANUP_VOLUMES=false
            shift
            ;;
        --network)
            CLEANUP_NETWORK=true
            shift
            ;;
        --dry-run)
            DRY_RUN=true
            shift
            ;;
        --force)
            FORCE=true
            shift
            ;;
        -h|--help)
            print_usage
            exit 0
            ;;
        *)
            echo -e "${RED}Unknown option: $1${NC}"
            print_usage
            exit 1
            ;;
    esac
done

# Logging functions
log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_action() {
    if [ "$DRY_RUN" = true ]; then
        echo -e "${BLUE}[DRY-RUN]${NC} Would: $1"
    else
        echo -e "${BLUE}[ACTION]${NC} $1"
    fi
}

execute() {
    if [ "$DRY_RUN" = false ]; then
        eval "$1"
    fi
}

# Clean Docker resources
cleanup_docker() {
    log_info "Cleaning Docker resources..."

    # Check if compose files exist
    local compose_simple="$DOCKER_DIR/docker-compose.yml"
    local compose_distributed="$DOCKER_DIR/docker-compose.distributed.yml"

    # Stop and remove containers
    if [ -f "$compose_simple" ]; then
        log_action "Stopping simple mode containers"
        execute "docker compose -f '$compose_simple' down 2>/dev/null || true"
    fi

    if [ -f "$compose_distributed" ]; then
        log_action "Stopping distributed mode containers"
        execute "docker compose -f '$compose_distributed' down 2>/dev/null || true"
    fi

    # Remove load test containers by name pattern
    local containers=$(docker ps -a --filter "name=load-test" --format "{{.Names}}" 2>/dev/null || echo "")
    if [ -n "$containers" ]; then
        log_action "Removing load-test containers: $(echo $containers | tr '\n' ' ')"
        execute "docker rm -f $containers 2>/dev/null || true"
    fi

    # Clean volumes if requested
    if [ "$CLEANUP_VOLUMES" = true ]; then
        if [ -f "$compose_simple" ]; then
            log_action "Removing simple mode volumes"
            execute "docker compose -f '$compose_simple' down -v 2>/dev/null || true"
        fi

        if [ -f "$compose_distributed" ]; then
            log_action "Removing distributed mode volumes"
            execute "docker compose -f '$compose_distributed' down -v 2>/dev/null || true"
        fi

        # Remove volumes by name pattern
        local volumes=$(docker volume ls --filter "name=load-test" --format "{{.Name}}" 2>/dev/null || echo "")
        if [ -n "$volumes" ]; then
            log_action "Removing load-test volumes: $(echo $volumes | tr '\n' ' ')"
            execute "docker volume rm $volumes 2>/dev/null || true"
        fi
    fi

    # Clean networks if requested
    if [ "$CLEANUP_NETWORK" = true ]; then
        local networks=$(docker network ls --filter "name=load-test" --format "{{.Name}}" 2>/dev/null || echo "")
        if [ -n "$networks" ]; then
            log_action "Removing load-test networks: $(echo $networks | tr '\n' ' ')"
            execute "docker network rm $networks 2>/dev/null || true"
        fi
    fi

    log_info "Docker cleanup complete"
}

# Clean log files
cleanup_logs() {
    log_info "Cleaning log files..."

    # Monitor logs
    local log_files=(
        "$MONITOR_DIR/disk-monitor.log"
        "$MONITOR_DIR/network-monitor.log"
        "$MONITOR_DIR/network-monitor.alerts"
        "$MONITOR_DIR/load-test.log"
        "$MONITOR_DIR/monitor.out"
    )

    for log_file in "${log_files[@]}"; do
        if [ -f "$log_file" ]; then
            log_action "Removing $log_file"
            execute "rm -f '$log_file'"
        fi
    done

    # Load generator logs (pattern match)
    local load_logs=$(find "$MONITOR_DIR" -name "load-generator*.log" 2>/dev/null || echo "")
    if [ -n "$load_logs" ]; then
        log_action "Removing load generator logs"
        execute "rm -f $load_logs"
    fi

    log_info "Log cleanup complete"
}

# Clean test wallet
cleanup_wallet() {
    log_info "Cleaning test wallet..."

    local wallet_file="$WALLET_DIR/test-wallet.json"

    if [ -f "$wallet_file" ]; then
        log_warn "This will delete the test wallet with all generated keys!"

        if [ "$FORCE" = false ]; then
            read -p "Are you sure you want to delete the test wallet? (y/N) " -n 1 -r
            echo
            if [[ ! $REPLY =~ ^[Yy]$ ]]; then
                log_info "Skipping wallet cleanup"
                return
            fi
        fi

        log_action "Removing test wallet: $wallet_file"
        execute "rm -f '$wallet_file'"
        log_info "Wallet cleanup complete"
    else
        log_info "No test wallet found"
    fi
}

# Show cleanup summary
show_summary() {
    echo ""
    echo -e "${BLUE}═══════════════════════════════════════${NC}"
    echo -e "${BLUE}     Cleanup Summary                   ${NC}"
    echo -e "${BLUE}═══════════════════════════════════════${NC}"
    echo ""
    echo "Items to be cleaned:"
    echo ""

    if [ "$CLEANUP_DOCKER" = true ]; then
        echo -e "  ${GREEN}✓${NC} Docker containers"
        if [ "$CLEANUP_VOLUMES" = true ]; then
            echo -e "  ${GREEN}✓${NC} Docker volumes"
        fi
        if [ "$CLEANUP_NETWORK" = true ]; then
            echo -e "  ${GREEN}✓${NC} Docker networks"
        fi
    else
        echo -e "  ${YELLOW}○${NC} Docker (skipped)"
    fi

    if [ "$CLEANUP_LOGS" = true ]; then
        echo -e "  ${GREEN}✓${NC} Log files"
    else
        echo -e "  ${YELLOW}○${NC} Logs (skipped)"
    fi

    if [ "$CLEANUP_WALLET" = true ]; then
        echo -e "  ${RED}✓${NC} Test wallet (WARNING: keys will be lost)"
    else
        echo -e "  ${YELLOW}○${NC} Wallet (skipped)"
    fi

    echo ""
    echo -e "${BLUE}═══════════════════════════════════════${NC}"
    echo ""

    if [ "$DRY_RUN" = true ]; then
        log_warn "DRY RUN MODE - No changes will be made"
        echo ""
    fi
}

# Main cleanup function
main() {
    log_info "Starting cleanup for load testing environment"

    # Show summary
    show_summary

    # Confirm unless forced or dry-run
    if [ "$FORCE" = false ] && [ "$DRY_RUN" = false ]; then
        read -p "Proceed with cleanup? (y/N) " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            log_info "Cleanup cancelled"
            exit 0
        fi
    fi

    echo ""

    # Run cleanup operations
    if [ "$CLEANUP_DOCKER" = true ]; then
        cleanup_docker
        echo ""
    fi

    if [ "$CLEANUP_LOGS" = true ]; then
        cleanup_logs
        echo ""
    fi

    if [ "$CLEANUP_WALLET" = true ]; then
        cleanup_wallet
        echo ""
    fi

    # Final summary
    if [ "$DRY_RUN" = false ]; then
        log_info "Cleanup complete!"
    else
        log_info "Dry run complete (no changes made)"
    fi

    # Show what remains
    echo ""
    log_info "Current state:"
    echo "  Docker containers: $(docker ps -a --filter 'name=load-test' --format '{{.Names}}' | wc -l)"
    echo "  Docker volumes: $(docker volume ls --filter 'name=load-test' --format '{{.Name}}' | wc -l)"
    echo "  Docker networks: $(docker network ls --filter 'name=load-test' --format '{{.Name}}' | wc -l)"

    if [ -f "$WALLET_DIR/test-wallet.json" ]; then
        echo "  Test wallet: exists"
    else
        echo "  Test wallet: not found"
    fi
}

# Run main function
main
