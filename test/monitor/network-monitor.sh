#!/bin/bash
# Comprehensive Network Monitor for Load Testing
#
# Monitors multiple health metrics for the Docker-based load testing network:
#   - Disk space (stops on low space)
#   - Memory usage
#   - Container health
#   - API responsiveness
#   - Log errors
#
# Usage:
#   ./network-monitor.sh [options]

set -e

# Default configuration
DISK_THRESHOLD_GB=10
MEMORY_THRESHOLD_PERCENT=90
CHECK_INTERVAL=30
COMPOSE_FILE="$(dirname "$0")/../docker/docker-compose.yml"
LOG_FILE="/tmp/network-monitor.log"
ALERT_FILE="/tmp/network-monitor.alerts"
API_ENDPOINTS=("http://localhost:8080/v3/describe" "http://localhost:8081/v3/describe" "http://localhost:8082/v3/describe")
AUTO_STOP=true

# State tracking
DISK_WARNING_SENT=false
MEMORY_WARNING_SENT=false
CONTAINER_FAILURES=0
MAX_CONTAINER_FAILURES=3

# Colors
RED='\033[0;31m'
YELLOW='\033[1;33m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --disk-threshold)
            DISK_THRESHOLD_GB="$2"
            shift 2
            ;;
        --memory-threshold)
            MEMORY_THRESHOLD_PERCENT="$2"
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
        --log-file)
            LOG_FILE="$2"
            shift 2
            ;;
        --no-auto-stop)
            AUTO_STOP=false
            shift
            ;;
        --help|-h)
            cat <<EOF
Network Monitor for Load Testing

Usage: $0 [options]

Options:
  --disk-threshold <GB>           Minimum free disk space (default: 10)
  --memory-threshold <percent>    Maximum memory usage (default: 90)
  --check-interval <seconds>      Check interval (default: 30)
  --compose-file <path>           Docker compose file
  --log-file <path>               Log file path
  --no-auto-stop                  Don't auto-stop on critical errors

Examples:
  $0
  $0 --disk-threshold 20 --memory-threshold 85
  $0 --no-auto-stop --check-interval 60

EOF
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

# Logging
log() {
    local timestamp=$(date '+%Y-%m-%d %H:%M:%S')
    echo "[$timestamp] $1" | tee -a "$LOG_FILE"
}

log_error() {
    echo -e "${RED}$1${NC}" >&2
    log "ERROR: $1"
    echo "[$timestamp] ERROR: $1" >> "$ALERT_FILE"
}

log_warn() {
    echo -e "${YELLOW}$1${NC}" >&2
    log "WARNING: $1"
}

log_info() {
    echo -e "${GREEN}$1${NC}"
    log "INFO: $1"
}

log_metric() {
    echo -e "${BLUE}$1${NC}"
    log "METRIC: $1"
}

# Disk space check
check_disk_space() {
    local free_gb=$(df -BG . | awk 'NR==2 {print $4}' | sed 's/G//')
    local used_percent=$(df . | awk 'NR==2 {print $5}' | sed 's/%//')

    log_metric "Disk: ${free_gb}GB free (${used_percent}% used)"

    if [ "$free_gb" -lt "$DISK_THRESHOLD_GB" ]; then
        log_error "CRITICAL: Disk space below threshold (${free_gb}GB < ${DISK_THRESHOLD_GB}GB)"

        if [ "$AUTO_STOP" = true ]; then
            log_error "Stopping network due to low disk space..."
            docker compose -f "$COMPOSE_FILE" down
            exit 1
        fi

        DISK_WARNING_SENT=true
        return 1
    elif [ "$DISK_WARNING_SENT" = true ]; then
        log_info "Disk space recovered"
        DISK_WARNING_SENT=false
    fi

    return 0
}

# Memory check
check_memory() {
    local total_mem=$(free -g | awk '/^Mem:/ {print $2}')
    local used_mem=$(free -g | awk '/^Mem:/ {print $3}')
    local used_percent=$((used_mem * 100 / total_mem))

    log_metric "Memory: ${used_mem}GB / ${total_mem}GB (${used_percent}% used)"

    if [ "$used_percent" -gt "$MEMORY_THRESHOLD_PERCENT" ]; then
        log_warn "High memory usage: ${used_percent}%"
        MEMORY_WARNING_SENT=true
        return 1
    elif [ "$MEMORY_WARNING_SENT" = true ]; then
        log_info "Memory usage normalized"
        MEMORY_WARNING_SENT=false
    fi

    return 0
}

# Container health check
check_containers() {
    if [ ! -f "$COMPOSE_FILE" ]; then
        return 0
    fi

    local running=$(docker compose -f "$COMPOSE_FILE" ps --status running | tail -n +2 | wc -l)
    local total=$(docker compose -f "$COMPOSE_FILE" ps | tail -n +2 | wc -l)
    local exited=$(docker compose -f "$COMPOSE_FILE" ps --status exited | tail -n +2 | wc -l)

    log_metric "Containers: ${running}/${total} running, ${exited} exited"

    if [ "$exited" -gt 0 ]; then
        log_warn "Found $exited exited containers"
        CONTAINER_FAILURES=$((CONTAINER_FAILURES + 1))

        if [ "$CONTAINER_FAILURES" -ge "$MAX_CONTAINER_FAILURES" ]; then
            log_error "Too many container failures ($CONTAINER_FAILURES)"

            if [ "$AUTO_STOP" = true ]; then
                log_error "Stopping network due to repeated container failures..."
                docker compose -f "$COMPOSE_FILE" down
                exit 1
            fi
        fi

        return 1
    else
        CONTAINER_FAILURES=0
    fi

    return 0
}

# API health check
check_api_health() {
    local healthy=0
    local total=${#API_ENDPOINTS[@]}

    for endpoint in "${API_ENDPOINTS[@]}"; do
        if curl -sf "$endpoint" > /dev/null 2>&1; then
            ((healthy++)) || true
        fi
    done

    log_metric "API Health: ${healthy}/${total} endpoints responding"

    if [ "$healthy" -eq 0 ] && [ "$total" -gt 0 ]; then
        log_warn "No API endpoints responding"
        return 1
    fi

    return 0
}

# Check Docker stats
check_docker_stats() {
    if ! docker ps --filter "name=load-test" --format "{{.Names}}" | grep -q "load-test"; then
        return 0
    fi

    local containers=$(docker ps --filter "name=load-test" --format "{{.Names}}")
    local stats=$(docker stats --no-stream --format "{{.Container}}: CPU={{.CPUPerc}} MEM={{.MemPerc}}" $containers 2>/dev/null || echo "")

    if [ -n "$stats" ]; then
        log_metric "Docker Stats:"
        echo "$stats" | while read line; do
            log_metric "  $line"
        done
    fi
}

# Print summary
print_summary() {
    echo ""
    echo -e "${BLUE}═══════════════════════════════════════${NC}"
    echo -e "${BLUE}     Network Monitor Status Summary    ${NC}"
    echo -e "${BLUE}═══════════════════════════════════════${NC}"

    check_disk_space && echo -e "${GREEN}✓ Disk space OK${NC}" || echo -e "${RED}✗ Disk space LOW${NC}"
    check_memory && echo -e "${GREEN}✓ Memory OK${NC}" || echo -e "${YELLOW}⚠ Memory high${NC}"
    check_containers && echo -e "${GREEN}✓ Containers healthy${NC}" || echo -e "${YELLOW}⚠ Container issues${NC}"
    check_api_health && echo -e "${GREEN}✓ APIs responding${NC}" || echo -e "${YELLOW}⚠ API issues${NC}"

    echo -e "${BLUE}═══════════════════════════════════════${NC}"
    echo ""
}

# Main monitoring loop
main() {
    log_info "Starting network monitor"
    log_info "Disk threshold: ${DISK_THRESHOLD_GB}GB"
    log_info "Memory threshold: ${MEMORY_THRESHOLD_PERCENT}%"
    log_info "Check interval: ${CHECK_INTERVAL}s"
    log_info "Auto-stop: $AUTO_STOP"
    log_info "Log file: $LOG_FILE"

    # Create/truncate alert file
    > "$ALERT_FILE"

    while true; do
        # Run all checks
        check_disk_space
        check_memory
        check_containers
        check_api_health
        check_docker_stats

        # Print summary every 10 checks
        if [ $(($(date +%s) % (CHECK_INTERVAL * 10))) -lt "$CHECK_INTERVAL" ]; then
            print_summary
        fi

        sleep "$CHECK_INTERVAL"
    done
}

# Signal handlers
cleanup() {
    log_info "Network monitor stopped"
    print_summary
    exit 0
}

trap cleanup SIGINT SIGTERM

# Start monitoring
main
