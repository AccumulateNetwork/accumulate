#!/bin/bash
# Disk Space Monitor for Load Testing
#
# This script monitors disk space and automatically stops the Docker network
# when available space falls below a configurable threshold.
#
# Usage:
#   ./disk-monitor.sh [options]
#
# Options:
#   --threshold <GB>        Minimum free space in GB (default: 10)
#   --check-interval <sec>  How often to check in seconds (default: 60)
#   --compose-file <path>   Docker compose file to manage (default: ../docker/docker-compose.yml)
#   --action <action>       Action to take: stop|pause|warn (default: stop)
#   --log-file <path>       Log file path (default: /tmp/disk-monitor.log)

set -e

# Default configuration
THRESHOLD_GB=10
CHECK_INTERVAL=60
COMPOSE_FILE="$(dirname "$0")/../docker/docker-compose.yml"
ACTION="stop"
LOG_FILE="/tmp/disk-monitor.log"
WARNED=false

# Colors
RED='\033[0;31m'
YELLOW='\033[1;33m'
GREEN='\033[0;32m'
NC='\033[0m'

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --threshold)
            THRESHOLD_GB="$2"
            shift 2
            ;;
        --check-interval)
            CHECK_INTERVAL="$2"
            shift 2
            ;;
        --compose-file)
            COMPOSE_FILE="$2"
            shift 2
            ;;
        --action)
            ACTION="$2"
            shift 2
            ;;
        --log-file)
            LOG_FILE="$2"
            shift 2
            ;;
        --help|-h)
            echo "Usage: $0 [options]"
            echo ""
            echo "Options:"
            echo "  --threshold <GB>        Minimum free space (default: 10)"
            echo "  --check-interval <sec>  Check interval (default: 60)"
            echo "  --compose-file <path>   Docker compose file"
            echo "  --action <action>       Action: stop|pause|warn (default: stop)"
            echo "  --log-file <path>       Log file path"
            echo ""
            echo "Examples:"
            echo "  $0 --threshold 20 --check-interval 30"
            echo "  $0 --action warn"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

# Logging functions
log() {
    local timestamp=$(date '+%Y-%m-%d %H:%M:%S')
    echo "[$timestamp] $1" | tee -a "$LOG_FILE"
}

log_error() {
    echo -e "${RED}$1${NC}" >&2
    log "ERROR: $1"
}

log_warn() {
    echo -e "${YELLOW}$1${NC}" >&2
    log "WARNING: $1"
}

log_info() {
    echo -e "${GREEN}$1${NC}"
    log "INFO: $1"
}

# Get available disk space in GB
get_free_space_gb() {
    df -BG . | awk 'NR==2 {print $4}' | sed 's/G//'
}

# Get total disk space in GB
get_total_space_gb() {
    df -BG . | awk 'NR==2 {print $2}' | sed 's/G//'
}

# Get used percentage
get_used_percent() {
    df . | awk 'NR==2 {print $5}' | sed 's/%//'
}

# Stop the Docker network
stop_network() {
    log_error "CRITICAL: Disk space below threshold! Stopping Docker network..."

    if [ -f "$COMPOSE_FILE" ]; then
        docker compose -f "$COMPOSE_FILE" down
        log_info "Docker network stopped successfully"
    else
        log_error "Compose file not found: $COMPOSE_FILE"
    fi

    # Exit after stopping
    exit 1
}

# Pause the Docker network
pause_network() {
    log_warn "WARNING: Disk space low! Pausing Docker containers..."

    if [ -f "$COMPOSE_FILE" ]; then
        docker compose -f "$COMPOSE_FILE" pause
        log_info "Docker containers paused"
    else
        log_error "Compose file not found: $COMPOSE_FILE"
    fi
}

# Warn only
warn_only() {
    if [ "$WARNED" = false ]; then
        log_warn "WARNING: Disk space is below threshold ($free_space GB < $THRESHOLD_GB GB)"
        log_warn "Please free up disk space or the test may fail"
        WARNED=true
    fi
}

# Take action based on configuration
take_action() {
    case "$ACTION" in
        stop)
            stop_network
            ;;
        pause)
            pause_network
            ;;
        warn)
            warn_only
            ;;
        *)
            log_error "Unknown action: $ACTION"
            exit 1
            ;;
    esac
}

# Main monitoring loop
main() {
    log_info "Starting disk space monitor"
    log_info "Threshold: ${THRESHOLD_GB}GB, Check interval: ${CHECK_INTERVAL}s, Action: $ACTION"
    log_info "Monitoring: $(pwd)"

    # Verify compose file exists
    if [ ! -f "$COMPOSE_FILE" ]; then
        log_warn "Compose file not found: $COMPOSE_FILE (monitoring will continue)"
    fi

    while true; do
        # Get disk space metrics
        free_space=$(get_free_space_gb)
        total_space=$(get_total_space_gb)
        used_percent=$(get_used_percent)

        # Check if below threshold
        if [ "$free_space" -lt "$THRESHOLD_GB" ]; then
            log_warn "Low disk space detected: ${free_space}GB free (${used_percent}% used)"
            take_action
        else
            # Reset warned flag when space is available
            if [ "$WARNED" = true ]; then
                log_info "Disk space recovered: ${free_space}GB free"
                WARNED=false
            fi

            # Log status every 10 checks
            if [ $(($(date +%s) % 600)) -lt "$CHECK_INTERVAL" ]; then
                log_info "Status: ${free_space}GB free of ${total_space}GB (${used_percent}% used)"
            fi
        fi

        # Wait before next check
        sleep "$CHECK_INTERVAL"
    done
}

# Signal handlers
trap 'log_info "Disk monitor stopped"; exit 0' SIGINT SIGTERM

# Start monitoring
main
