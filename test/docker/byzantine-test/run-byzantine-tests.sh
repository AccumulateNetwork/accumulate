#!/bin/bash

# Byzantine Fault Tolerance Test Runner
#
# This script orchestrates Byzantine attack tests on a 25-node Docker network.
# It validates the security fixes for:
#   - Issue #3869: Duplicate vote handling
#   - Issue #3870: Non-validator vote spam protection
#
# Usage:
#   ./test/docker/byzantine-test/run-byzantine-tests.sh
#
# Environment Variables:
#   SKIP_BUILD=1           - Skip Docker build
#   SKIP_NETWORK_START=1   - Skip network startup (use existing)
#   TEST_DURATION=5m       - Duration for each attack test
#   KEEP_NETWORK=1         - Don't tear down network after tests

set -euo pipefail

# Color output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Directories
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
COMPOSE_FILE="$SCRIPT_DIR/docker-compose-25node.yml"
RESULTS_DIR="$SCRIPT_DIR/results"
LOGS_DIR="$SCRIPT_DIR/logs"

# Test configuration
MALICIOUS_NODES="${MALICIOUS_NODES:-9}"
DUPLICATE_VOTES="${DUPLICATE_VOTES:-10}"
NUM_SPAMMERS="${NUM_SPAMMERS:-100}"
VOTES_PER_SECOND="${VOTES_PER_SECOND:-1000}"
ATTACK_DURATION="${ATTACK_DURATION:-5m}"
CPU_THRESHOLD="${CPU_THRESHOLD:-10.0}"

# Timestamps
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Setup results and logs directories
setup_directories() {
    log_info "Setting up directories..."
    mkdir -p "$RESULTS_DIR"
    mkdir -p "$LOGS_DIR"
}

# Build Docker images
build_images() {
    if [[ "${SKIP_BUILD:-0}" == "1" ]]; then
        log_info "Skipping Docker build (SKIP_BUILD=1)"
        return
    fi

    log_info "Building Docker images..."
    cd "$PROJECT_ROOT"
    docker compose -f "$COMPOSE_FILE" build > "$LOGS_DIR/build_${TIMESTAMP}.log" 2>&1
    log_success "Docker images built"
}

# Start the 25-node network
start_network() {
    if [[ "${SKIP_NETWORK_START:-0}" == "1" ]]; then
        log_info "Skipping network start (SKIP_NETWORK_START=1)"
        return
    fi

    log_info "Starting 25-node Byzantine test network..."
    log_info "  - 3 BVNs with 8 validators each = 24 validators"
    log_info "  - 1 DN with 1 validator = 25 total"
    log_info "  - 1 bootstrap node"

    # Clean up any existing network
    docker compose -f "$COMPOSE_FILE" down -v > /dev/null 2>&1 || true

    # Start network
    docker compose -f "$COMPOSE_FILE" up -d > "$LOGS_DIR/startup_${TIMESTAMP}.log" 2>&1

    log_info "Waiting for network to stabilize (60 seconds)..."
    sleep 60

    # Check if nodes are healthy
    check_network_health
}

# Check network health
check_network_health() {
    log_info "Checking network health..."

    local healthy=0
    local total=26  # 25 validators + 1 bootstrap

    for container in $(docker compose -f "$COMPOSE_FILE" ps -q); do
        if docker inspect "$container" --format='{{.State.Health.Status}}' 2>/dev/null | grep -q healthy; then
            ((healthy++))
        fi
    done

    log_info "Healthy nodes: $healthy / $total"

    if [[ $healthy -lt 17 ]]; then  # Need at least 2/3 for consensus
        log_error "Insufficient healthy nodes for consensus ($healthy / $total)"
        return 1
    fi

    log_success "Network is healthy"
}

# Build attack simulation programs
build_attack_tools() {
    log_info "Building attack simulation tools..."

    cd "$SCRIPT_DIR"

    # Build Byzantine attack simulator
    log_info "  - Building byzantine-attack..."
    go build -o "$SCRIPT_DIR/byzantine-attack" byzantine-attack.go

    # Build vote spam attack simulator
    log_info "  - Building vote-spam-attack..."
    go build -o "$SCRIPT_DIR/vote-spam-attack" vote-spam-attack.go

    log_success "Attack tools built"
}

# Run Byzantine attack test (Issue #3869)
run_byzantine_attack_test() {
    log_info "=========================================="
    log_info "Running Byzantine Attack Test (Issue #3869)"
    log_info "=========================================="
    log_info "Configuration:"
    log_info "  - Malicious validators: $MALICIOUS_NODES"
    log_info "  - Duplicate votes per round: $DUPLICATE_VOTES"
    log_info "  - Attack duration: $ATTACK_DURATION"

    local report_file="$RESULTS_DIR/byzantine-attack-${TIMESTAMP}.txt"

    MALICIOUS_NODES="$MALICIOUS_NODES" \
    DUPLICATE_VOTES="$DUPLICATE_VOTES" \
    ATTACK_DURATION="$ATTACK_DURATION" \
    "$SCRIPT_DIR/byzantine-attack" > "$LOGS_DIR/byzantine-attack-${TIMESTAMP}.log" 2>&1

    # Copy report to results
    if [[ -f /tmp/byzantine-attack-report.txt ]]; then
        cp /tmp/byzantine-attack-report.txt "$report_file"
        log_success "Byzantine attack test completed"
        log_info "Report: $report_file"
        echo ""
        cat "$report_file"
        echo ""
    else
        log_error "Byzantine attack report not found"
        return 1
    fi
}

# Run vote spam attack test (Issue #3870)
run_vote_spam_attack_test() {
    log_info "=========================================="
    log_info "Running Vote Spam Attack Test (Issue #3870)"
    log_info "=========================================="
    log_info "Configuration:"
    log_info "  - Spam nodes: $NUM_SPAMMERS"
    log_info "  - Votes per second (each): $VOTES_PER_SECOND"
    log_info "  - Total vote rate: $((NUM_SPAMMERS * VOTES_PER_SECOND)) votes/sec"
    log_info "  - CPU threshold: $CPU_THRESHOLD%"
    log_info "  - Attack duration: $ATTACK_DURATION"

    local report_file="$RESULTS_DIR/vote-spam-attack-${TIMESTAMP}.txt"

    NUM_SPAMMERS="$NUM_SPAMMERS" \
    VOTES_PER_SECOND="$VOTES_PER_SECOND" \
    CPU_THRESHOLD="$CPU_THRESHOLD" \
    ATTACK_DURATION="$ATTACK_DURATION" \
    "$SCRIPT_DIR/vote-spam-attack" > "$LOGS_DIR/vote-spam-attack-${TIMESTAMP}.log" 2>&1

    # Copy report to results
    if [[ -f /tmp/vote-spam-attack-report.txt ]]; then
        cp /tmp/vote-spam-attack-report.txt "$report_file"
        log_success "Vote spam attack test completed"
        log_info "Report: $report_file"
        echo ""
        cat "$report_file"
        echo ""
    else
        log_error "Vote spam attack report not found"
        return 1
    fi
}

# Collect network metrics
collect_metrics() {
    log_info "Collecting network metrics..."

    local metrics_file="$RESULTS_DIR/network-metrics-${TIMESTAMP}.txt"

    {
        echo "Network Metrics - $(date)"
        echo "=================================="
        echo ""
        echo "Docker Stats:"
        docker compose -f "$COMPOSE_FILE" ps
        echo ""
        echo "Container Resource Usage:"
        docker stats --no-stream --format "table {{.Container}}\t{{.CPUPerc}}\t{{.MemUsage}}\t{{.NetIO}}"
        echo ""
    } > "$metrics_file"

    log_info "Metrics saved to: $metrics_file"
}

# Collect container logs
collect_logs() {
    log_info "Collecting container logs..."

    local logs_archive="$RESULTS_DIR/container-logs-${TIMESTAMP}.tar.gz"

    # Collect logs from all containers
    docker compose -f "$COMPOSE_FILE" logs > "$LOGS_DIR/all-containers-${TIMESTAMP}.log" 2>&1

    # Archive logs
    tar -czf "$logs_archive" -C "$LOGS_DIR" . 2>/dev/null || true

    log_info "Logs archived to: $logs_archive"
}

# Stop and clean up network
cleanup_network() {
    if [[ "${KEEP_NETWORK:-0}" == "1" ]]; then
        log_info "Keeping network running (KEEP_NETWORK=1)"
        return
    fi

    log_info "Stopping and cleaning up network..."
    docker compose -f "$COMPOSE_FILE" down -v > "$LOGS_DIR/cleanup_${TIMESTAMP}.log" 2>&1
    log_success "Network cleaned up"
}

# Generate final summary report
generate_summary() {
    log_info "Generating summary report..."

    local summary_file="$RESULTS_DIR/summary-${TIMESTAMP}.txt"

    {
        echo "Byzantine Fault Tolerance Test Summary"
        echo "======================================"
        echo "Timestamp: $(date)"
        echo ""
        echo "Test Configuration:"
        echo "  - Total Validators: 25"
        echo "  - Byzantine Validators: $MALICIOUS_NODES ($(echo "scale=1; $MALICIOUS_NODES * 100 / 25" | bc)%)"
        echo "  - Spam Nodes: $NUM_SPAMMERS"
        echo "  - Attack Duration: $ATTACK_DURATION"
        echo ""
        echo "Test Results:"
        echo ""

        if [[ -f "$RESULTS_DIR/byzantine-attack-${TIMESTAMP}.txt" ]]; then
            echo "1. Byzantine Attack Test (Issue #3869):"
            echo "   Report: byzantine-attack-${TIMESTAMP}.txt"
            grep -A 5 "^Result:" "$RESULTS_DIR/byzantine-attack-${TIMESTAMP}.txt" || echo "   [See full report]"
            echo ""
        fi

        if [[ -f "$RESULTS_DIR/vote-spam-attack-${TIMESTAMP}.txt" ]]; then
            echo "2. Vote Spam Attack Test (Issue #3870):"
            echo "   Report: vote-spam-attack-${TIMESTAMP}.txt"
            grep -A 5 "^Result:" "$RESULTS_DIR/vote-spam-attack-${TIMESTAMP}.txt" || echo "   [See full report]"
            echo ""
        fi

        echo "All Results: $RESULTS_DIR"
        echo "All Logs: $LOGS_DIR"
    } > "$summary_file"

    log_success "Summary report generated: $summary_file"
    echo ""
    cat "$summary_file"
}

# Main execution
main() {
    log_info "Byzantine Fault Tolerance Test Suite"
    log_info "======================================"
    echo ""

    # Setup
    setup_directories

    # Build
    build_images
    build_attack_tools

    # Start network
    start_network

    # Run tests
    run_byzantine_attack_test
    run_vote_spam_attack_test

    # Collect metrics
    collect_metrics
    collect_logs

    # Cleanup
    cleanup_network

    # Generate summary
    generate_summary

    log_success "All tests completed!"
    log_info "Results directory: $RESULTS_DIR"
}

# Run main with error handling
if main; then
    exit 0
else
    log_error "Tests failed"
    exit 1
fi
