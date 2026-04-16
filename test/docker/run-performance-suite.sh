#!/bin/bash
# Performance test suite for Issue #3905
# Runs all 9 topologies in sequence: deploy, test, collect, wipe, next.
#
# Each node runs both DN and BVN (dual mode).
# Topologies: {2,3,4} validators × {1,2,3} BVNs
#
# Usage:
#   ./run-performance-suite.sh              # Run all 9
#   ./run-performance-suite.sh 2 1          # Run just 2-val-1-bvn
#   ./run-performance-suite.sh 3 2 4 3      # Run 3v2b then 4v3b

# NOTE: do NOT use set -e here — arithmetic like ((passed++)) returns 1
# when the value was 0, and run_test failures should be caught by the loop,
# not kill the entire suite.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
RESULTS_DIR="$SCRIPT_DIR/performance-results"
TIMESTAMP=$(date +%Y%m%d-%H%M%S)

mkdir -p "$RESULTS_DIR"
mkdir -p /tmp/loadtest-workspace

SUITE_LOG="$RESULTS_DIR/suite-$TIMESTAMP.log"
DASHBOARD_PORT=8888
DASHBOARD_PID=""

# All 9 topologies: validators bvns
ALL_CONFIGS=(
    "2 1"
    "2 2"
    "2 3"
    "3 1"
    "3 2"
    "3 3"
    "4 1"
    "4 2"
    "4 3"
)

# Test parameters
TEST_DURATION=0      # 0 = run until error cutoff stops the test
ERROR_THRESHOLD=5.0  # stop test at this error %

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*" | tee -a "$SUITE_LOG"
}

log_section() {
    echo "" | tee -a "$SUITE_LOG"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" | tee -a "$SUITE_LOG"
    echo -e "${BLUE}$*${NC}" | tee -a "$SUITE_LOG"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" | tee -a "$SUITE_LOG"
}

log_error() {
    echo -e "${RED}[ERROR] $*${NC}" | tee -a "$SUITE_LOG"
}

log_success() {
    echo -e "${GREEN}[OK] $*${NC}" | tee -a "$SUITE_LOG"
}

# Start the dashboard server
start_dashboard() {
    pkill -f "dashboard-server.py" 2>/dev/null || true
    sleep 1
    python3 "$SCRIPT_DIR/dashboard-server.py" "$DASHBOARD_PORT" > /tmp/dashboard-server.log 2>&1 &
    DASHBOARD_PID=$!
    log_success "Dashboard running at http://localhost:$DASHBOARD_PORT/"
}

# Stop the dashboard server
stop_dashboard() {
    if [ -n "$DASHBOARD_PID" ]; then
        kill "$DASHBOARD_PID" 2>/dev/null || true
        DASHBOARD_PID=""
    fi
    pkill -f "dashboard-server.py" 2>/dev/null || true
}

# Wipe all Docker state between tests
docker_cleanup() {
    log "Cleaning Docker state..."

    cd "$SCRIPT_DIR"

    # Try compose down for any compose file that might be active
    for f in docker-compose-*-val-*-bvn.yml docker-compose.yml; do
        docker compose -f "$f" down -v 2>/dev/null || true
    done

    # Kill any accumulate containers by name
    docker ps -a 2>/dev/null | grep -E "acc-" | awk '{print $1}' | xargs -r docker stop 2>/dev/null || true
    docker ps -a 2>/dev/null | grep -E "acc-" | awk '{print $1}' | xargs -r docker rm -f 2>/dev/null || true

    # Remove accumulate volumes
    docker volume ls -q 2>/dev/null | grep -E "docker_network-config|network-config" | xargs -r docker volume rm -f 2>/dev/null || true

    # Prune
    docker volume prune -f 2>/dev/null || true

    sleep 2
    log_success "Docker clean"
}

# Capture failure details
capture_failure() {
    local test_id=$1
    local error_msg=$2
    local stage=$3
    local compose_file=$4

    local failure_file="$RESULTS_DIR/FAILED-${test_id}-$(date +%Y%m%d-%H%M%S).txt"

    cat > "$failure_file" << EOF
FAILURE REPORT
==============
Test ID: $test_id
Timestamp: $(date -u +%Y-%m-%dT%H:%M:%SZ)
Stage: $stage
Error: $error_msg

DOCKER STATE
============
$(docker ps -a 2>&1)

CONTAINER LOGS (last 100 lines)
===============================
$(docker compose -f "$compose_file" logs 2>&1 | tail -100)

SYSTEM
======
Disk: $(df -h / 2>&1)
Memory: $(free -h 2>&1)
Load: $(uptime 2>&1)
EOF

    log_error "Failure details: $failure_file"
}

# Build Docker image once
build_image() {
    log_section "Building Docker image"
    cd "$REPO_ROOT"
    if ! docker build -t accumulated-test -f Dockerfile . > /tmp/docker-build.log 2>&1; then
        log_error "Docker build failed. See /tmp/docker-build.log"
        tail -20 /tmp/docker-build.log | tee -a "$SUITE_LOG"
        exit 1
    fi
    log_success "Docker image built"
}

# Get the list of API ports for a given topology
get_api_ports() {
    local validators=$1
    local bvns=$2
    local compose_file="$SCRIPT_DIR/docker-compose-${validators}-val-${bvns}-bvn.yml"

    # Extract host ports mapped to 26660
    grep -E '^\s+- "[0-9]+:26660"' "$compose_file" | sed 's/.*"\([0-9]*\):26660".*/\1/'
}

# Wait for network to be ready
wait_for_network() {
    local validators=$1
    local bvns=$2
    local max_wait=120
    local elapsed=0

    log "Waiting for network to be ready (up to ${max_wait}s)..."

    # Get the first API port to poll
    local first_port=$(get_api_ports "$validators" "$bvns" | head -1)
    if [ -z "$first_port" ]; then
        log_error "Failed to extract API port from compose file"
        return 1
    fi

    while [ $elapsed -lt $max_wait ]; do
        if curl -sf -X POST "http://localhost:${first_port}/v3" \
            -H "Content-Type: application/json" \
            -d '{"jsonrpc":"2.0","id":1,"method":"network-status","params":{}}' \
            > /dev/null 2>&1; then
            log_success "Network ready after ${elapsed}s"
            return 0
        fi
        sleep 5
        elapsed=$((elapsed + 5))
    done

    log_error "Network not ready after ${max_wait}s"
    return 1
}

# Run a single topology test — continuous ramp, one invocation
run_test() {
    local validators=$1
    local bvns=$2
    local total_nodes=$((validators * bvns))
    local test_id="${validators}v-${bvns}b"
    local compose_file="$SCRIPT_DIR/docker-compose-${validators}-val-${bvns}-bvn.yml"

    log_section "Test: ${validators} validators x ${bvns} BVN(s) = ${total_nodes} nodes"

    if [ ! -f "$compose_file" ]; then
        log_error "Missing: $compose_file"
        return 1
    fi

    # Clean before test
    docker_cleanup

    # Deploy
    log "Deploying ${test_id}..."
    cd "$SCRIPT_DIR"

    if ! docker compose -f "$compose_file" up -d > /tmp/compose-up-${test_id}.log 2>&1; then
        log_error "Deploy failed"
        capture_failure "$test_id" "docker compose up failed" "deploy" "$compose_file"
        docker compose -f "$compose_file" down -v 2>/dev/null || true
        return 1
    fi

    log "Waiting for init to finish..."
    docker compose -f "$compose_file" wait init > /dev/null 2>&1 || true
    log "Waiting for bootstrap and validators..."
    sleep 30

    if ! wait_for_network "$validators" "$bvns"; then
        capture_failure "$test_id" "Network did not come up" "startup" "$compose_file"
        docker compose -f "$compose_file" down -v 2>/dev/null || true
        return 1
    fi

    log_success "Network running: ${total_nodes} nodes"

    # Build API endpoint list
    local endpoints=""
    for port in $(get_api_ports "$validators" "$bvns"); do
        if [ -n "$endpoints" ]; then
            endpoints="${endpoints},"
        fi
        endpoints="${endpoints}http://localhost:${port}/v3"
    done

    # Clear stale status (but NOT the log — it accumulates across tests)
    rm -f /tmp/loadtest-workspace/status.json

    # Run continuous ramp load test
    local test_output="$RESULTS_DIR/${test_id}-output.txt"
    log "Starting load test: ${test_id}..."

    go run parallel-loadtest.go \
        -duration 0 \
        -start-tps 1000 \
        -max-tps 50000 \
        -ramp-step 1000 \
        -ramp-interval 20s \
        -error-cutoff "$ERROR_THRESHOLD" \
        -nodes "$endpoints" \
        -faucet-seed "FAUCET" \
        -oracle 1000 \
        -accounts 1000000 \
        -label "${test_id}" \
        > "$test_output" 2>&1 || true

    # Parse final results
    local submitted=$(grep "^  Submitted:" "$test_output" 2>/dev/null | awk '{print $2}')
    local failed=$(grep "^  Failed:" "$test_output" 2>/dev/null | awk '{print $2}')
    local peak=$(grep "^  Peak TPS:" "$test_output" 2>/dev/null | awk '{print $3}')
    local avg_tps=$(grep "^  Average TPS:" "$test_output" 2>/dev/null | awk '{print $3}')
    submitted=${submitted:-0}; failed=${failed:-0}; peak=${peak:-0}; avg_tps=${avg_tps:-0}

    log_success "${test_id}: peak=${peak} avg=${avg_tps} submitted=${submitted} failed=${failed}"

    # Tear down
    log "Tearing down ${test_id}..."
    docker compose -f "$compose_file" down -v > /dev/null 2>&1 || true
    docker_cleanup

    log_success "Test ${test_id} complete"
}

# Generate summary report
generate_report() {
    log_section "Generating Summary Report"

    local report="$RESULTS_DIR/PERFORMANCE-RESULTS-$TIMESTAMP.md"

    cat > "$report" << EOF
# Performance Test Results -- RC v1.5.1-breaking

**Date**: $(date -u +%Y-%m-%dT%H:%M:%SZ)
**Methodology**: Continuous ramp 1K->25K TPS (+2K every 30s), cutoff at ${ERROR_THRESHOLD}% errors
**Duration per topology**: ${TEST_DURATION}s
**Topologies**: {2,3,4} validators x {1,2,3} BVNs (every node runs DN+BVN)

## Results

| Topology | Nodes | Peak TPS | Avg TPS | Submitted | Failed |
|----------|-------|----------|---------|-----------|--------|
EOF

    for config in "${RUN_CONFIGS[@]}"; do
        IFS=' ' read -r v b <<< "$config"
        local test_id="${v}v-${b}b"
        local nodes=$((v * b))
        local output="$RESULTS_DIR/${test_id}-output.txt"

        if [ -f "$output" ]; then
            local peak=$(grep "^  Peak TPS:" "$output" 2>/dev/null | awk '{print $3}')
            local avg=$(grep "^  Average TPS:" "$output" 2>/dev/null | awk '{print $3}')
            local sub=$(grep "^  Submitted:" "$output" 2>/dev/null | awk '{print $2}')
            local fail=$(grep "^  Failed:" "$output" 2>/dev/null | awk '{print $2}')
            peak=${peak:-N/A}; avg=${avg:-N/A}; sub=${sub:-N/A}; fail=${fail:-N/A}
            echo "| ${v}v x ${b}b | ${nodes} | ${peak} | ${avg} | ${sub} | ${fail} |" >> "$report"
        else
            echo "| ${v}v x ${b}b | ${nodes} | FAILED | -- | -- | -- |" >> "$report"
        fi
    done

    cat >> "$report" << 'EOF'

## Detailed Output

See individual `*-output.txt` files in this directory.

## Test Log

See `/tmp/loadtest-workspace/log.jsonl` for the full event log across all topologies.
EOF

    log_success "Report: $report"
}

# Main
main() {
    log_section "Performance Test Suite — Issue #3905"

    # Parse args: pairs of (validators bvns), or run all 9
    RUN_CONFIGS=()
    if [ $# -gt 0 ]; then
        while [ $# -ge 2 ]; do
            RUN_CONFIGS+=("$1 $2")
            shift 2
        done
    else
        RUN_CONFIGS=("${ALL_CONFIGS[@]}")
    fi

    log "Topologies to test: ${#RUN_CONFIGS[@]}"
    for c in "${RUN_CONFIGS[@]}"; do
        IFS=' ' read -r v b <<< "$c"
        log "  ${v} validators × ${b} BVN(s) = $((v * b)) nodes"
    done

    # Clean slate
    docker_cleanup

    # Build image once
    build_image

    # Start dashboard — stays up for entire suite
    start_dashboard

    # Cleanup handler
    trap 'stop_dashboard; docker_cleanup' EXIT

    # Run each topology
    local passed=0
    local failed=0
    for config in "${RUN_CONFIGS[@]}"; do
        IFS=' ' read -r v b <<< "$config"
        if run_test "$v" "$b"; then
            passed=$((passed + 1))
        else
            failed=$((failed + 1))
            log_error "FAILED: ${v}v-${b}b"
            docker_cleanup
        fi
    done

    # Report
    generate_report

    # Summary
    log_section "Suite Complete"
    log "Passed: $passed / $((passed + failed))"
    log "Failed: $failed"
    log "Results: $RESULTS_DIR"
    log "Dashboard was at http://localhost:$DASHBOARD_PORT/"

    # Final cleanup (also handled by trap)
    stop_dashboard
    docker_cleanup
}

main "$@"
