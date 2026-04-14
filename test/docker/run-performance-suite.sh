#!/bin/bash
# Orchestration script for Issue #3905 performance testing
# Runs all 6 configurations in sequence with incremental TPS testing
# Results saved to test/docker/performance-results/

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
RESULTS_DIR="$SCRIPT_DIR/performance-results"
TIMESTAMP=$(date +%Y%m%d-%H%M%S)

# Ensure directories exist
mkdir -p "$RESULTS_DIR"
mkdir -p /tmp/perf-test-monitoring
mkdir -p /tmp/perf-test-data

# Log file for entire test suite
SUITE_LOG="$RESULTS_DIR/suite-$TIMESTAMP.log"

# Test configurations: (validators, bvns, test_id, description)
declare -a CONFIGS=(
    "3 1 A1 Single-BVN-3-Validators"
    "4 1 A2 Single-BVN-4-Validators"
    "3 2 B1 Dual-BVN-3-Validators"
    "4 2 B2 Dual-BVN-4-Validators"
    "3 3 C1 Triple-BVN-3-Validators"
    "4 3 C2 Triple-BVN-4-Validators"
)

# Incremental TPS sequence
TPS_SEQUENCE=(1000 2000 3000 5000 7000 10000 12000)
INCREMENT_DURATION=60
ERROR_THRESHOLD=0.05

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

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
    echo -e "${GREEN}[SUCCESS] $*${NC}" | tee -a "$SUITE_LOG"
}

# Complete Docker cleanup - wipe everything
docker_cleanup() {
    log "Performing complete Docker cleanup..."

    # Stop all accumulate containers
    docker ps -a --filter "label=accumulate=true" -q 2>/dev/null | xargs -r docker stop 2>/dev/null || true
    docker ps -a --filter "label=accumulate=true" -q 2>/dev/null | xargs -r docker rm -f 2>/dev/null || true

    # Stop containers by name pattern
    docker ps -a 2>/dev/null | grep -E "acc-|accumulate" | awk '{print $1}' | xargs -r docker stop 2>/dev/null || true
    docker ps -a 2>/dev/null | grep -E "acc-|accumulate" | awk '{print $1}' | xargs -r docker rm -f 2>/dev/null || true

    # Remove networks
    docker network ls 2>/dev/null | grep -E "acc-network|accumulate" | awk '{print $1}' | xargs -r docker network rm 2>/dev/null || true

    # Remove volumes
    docker volume ls 2>/dev/null | grep -E "acc-|accumulate" | awk '{print $2}' | xargs -r docker volume rm -f 2>/dev/null || true

    # Prune dangling images/volumes/networks (but not stopped containers we might need)
    docker volume prune -f --filter "label!=keep" 2>/dev/null || true

    # Clear local database directories
    rm -rf /tmp/accumulate-* 2>/dev/null || true
    rm -rf /tmp/perf-test-data/* 2>/dev/null || true
    rm -rf /tmp/perf-test-monitoring/* 2>/dev/null || true

    # Wait for cleanup to settle
    sleep 3

    log_success "Docker cleanup complete"
}

# Verify Docker is clean
verify_docker_clean() {
    log "Verifying Docker is clean..."

    local acc_containers=$(docker ps -a 2>/dev/null | grep -c -E "acc-|accumulate" || echo 0)
    if [ "$acc_containers" -gt 0 ]; then
        log_error "Found $acc_containers lingering accumulate containers. Forcing cleanup..."
        docker ps -a 2>/dev/null | grep -E "acc-|accumulate"
        docker system prune -f 2>/dev/null || true
        sleep 2
    fi

    local acc_volumes=$(docker volume ls 2>/dev/null | grep -c -E "acc-|accumulate" || echo 0)
    if [ "$acc_volumes" -gt 0 ]; then
        log_error "Found $acc_volumes lingering accumulate volumes. Forcing cleanup..."
        docker volume ls 2>/dev/null | grep -E "acc-|accumulate"
    fi

    log_success "Docker verification complete"
}

# Build binary once
build_binary() {
    log_section "Building accumulated binary"
    cd "$REPO_ROOT"
    if ! go build -o /tmp/accumulated ./cmd/accumulated 2>/tmp/build.log; then
        log_error "Build failed. See /tmp/build.log"
        tail -50 /tmp/build.log
        exit 1
    fi
    log_success "Binary built"
}

# Generate docker-compose for specific validator/BVN count
generate_docker_compose() {
    local validators=$1
    local bvns=$2
    local output_file="$SCRIPT_DIR/docker-compose-$validators-val-$bvns-bvn.yml"

    log "Generating docker-compose for $validators validators, $bvns BVNs"

    # Template based on current docker-compose.yml structure
    cat > "$output_file" << 'COMPOSE_EOF'
version: '3.8'

services:
  bootstrap:
    image: docker-bootstrap
    container_name: acc-bootstrap
    environment:
      - BVNS=%BVNS%
      - VALIDATORS=%VALIDATORS%
    ports:
      - "16593:16593"
    networks:
      - acc-network
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:16593/health"]
      interval: 5s
      timeout: 3s
      retries: 10
      start_period: 30s
COMPOSE_EOF

    # Generate validator services
    for ((i=1; i<=bvns; i++)); do
        for ((j=1; j<=validators; j++)); do
            cat >> "$output_file" << EOF

  bvn${i}-val${j}:
    image: docker-bvn${i}-val${j}
    container_name: acc-bvn${i}-val${j}
    depends_on:
      bootstrap:
        condition: service_healthy
    environment:
      - BVNS=$bvns
      - VALIDATORS=$validators
      - BVN_INDEX=$i
      - VAL_INDEX=$j
    ports:
      - "2666${i}${j}:26660"
    networks:
      - acc-network
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:26660/health"]
      interval: 5s
      timeout: 3s
      retries: 10
      start_period: 30s
EOF
        done
    done

    cat >> "$output_file" << 'COMPOSE_EOF'

networks:
  acc-network:
    driver: bridge
COMPOSE_EOF

    echo "$output_file"
}

# Run single test configuration
run_test_config() {
    local validators=$1
    local bvns=$2
    local test_id=$3
    local description=$4

    log_section "Test $test_id: $description ($validators validators, $bvns BVNs)"

    local config_results="$RESULTS_DIR/${test_id}-results.csv"
    local config_log="$RESULTS_DIR/${test_id}.log"

    # Initialize results CSV
    echo "TPS_TARGET,SUBMITTED,SUCCESS,FAILED,ERROR_RATE,ACTUAL_TPS,P50_LATENCY,P99_LATENCY,CPU_PCT,MEMORY_PCT,PUSHBACK_DETECTED" > "$config_results"

    # Complete Docker cleanup before this test
    log "Wiping Docker state before test..."
    docker_cleanup
    verify_docker_clean

    # Generate docker-compose
    local compose_file=$(generate_docker_compose "$validators" "$bvns")

    # Start network
    log "Starting Docker network..."
    export COMPOSE_FILE="$compose_file"
    cd "$SCRIPT_DIR"

    # One more explicit cleanup via compose
    docker compose down -v 2>/dev/null || true
    sleep 3

    docker compose build > /dev/null 2>&1 || {
        log_error "Docker build failed"
        docker_cleanup
        return 1
    }
    docker compose up -d > /dev/null 2>&1
    sleep 30

    # Check health
    if ! docker compose ps | grep -q "healthy"; then
        log_error "Network failed to start. Logs:"
        docker compose logs bootstrap 2>&1 | tail -30
        docker compose down -v 2>/dev/null || true
        return 1
    fi

    log_success "Network started ($validators validators, $bvns BVN(s))"

    # Start monitoring
    log "Starting monitoring..."
    mkdir -p "/tmp/perf-test-monitoring/$test_id"
    pkill -f "monitor.py" 2>/dev/null || true
    python3 "$SCRIPT_DIR/monitor.py" "/tmp/perf-test-monitoring/$test_id" 3600 10 > /dev/null 2>&1 &
    sleep 2

    # Run incremental TPS tests
    local pushback_detected=false
    for tps in "${TPS_SEQUENCE[@]}"; do
        if [ "$pushback_detected" = true ]; then
            log "Stopping increments: pushback already detected at lower TPS"
            break
        fi

        log "Testing at $tps TPS (${INCREMENT_DURATION}s)..."

        cd "$SCRIPT_DIR"
        local test_output="/tmp/perf-test-data/${test_id}-${tps}-tps.txt"

        timeout $((INCREMENT_DURATION + 30)) go run parallel-loadtest.go \
            -duration "${INCREMENT_DURATION}s" \
            -target-tps "$tps" \
            2>&1 | tee "$test_output" || true

        # Parse results from output
        local submitted=$(grep "Submitted:" "$test_output" | tail -1 | awk '{print $NF}')
        local success=$(grep "Success:" "$test_output" | tail -1 | awk '{print $NF}')
        local failed=$(grep "Failed:" "$test_output" | tail -1 | awk '{print $NF}')
        local actual_tps=$(grep "Average TPS:" "$test_output" | tail -1 | awk '{print $NF}')

        # Calculate error rate
        local error_rate="0.0"
        if [ -n "$submitted" ] && [ "$submitted" -gt 0 ]; then
            error_rate=$(awk "BEGIN {printf \"%.2f\", ($failed / $submitted) * 100}")
        fi

        # Get CPU/memory from monitoring (placeholder - needs refinement)
        local cpu_pct="N/A"
        local memory_pct="N/A"

        # Check for pushback
        if (( $(echo "$error_rate > $ERROR_THRESHOLD" | bc -l) )); then
            log_error "Pushback detected at $tps TPS (error rate: ${error_rate}%)"
            pushback_detected=true
            echo "$tps,$submitted,$success,$failed,$error_rate,$actual_tps,N/A,N/A,$cpu_pct,$memory_pct,TRUE" >> "$config_results"
        else
            log_success "$tps TPS: $actual_tps actual TPS, ${error_rate}% error"
            echo "$tps,$submitted,$success,$failed,$error_rate,$actual_tps,N/A,N/A,$cpu_pct,$memory_pct,FALSE" >> "$config_results"
        fi

        sleep 5
    done

    # Cleanup - aggressive wipe for this configuration
    log "Stopping network and cleaning Docker state..."
    docker compose down -v 2>/dev/null || true
    pkill -f "monitor.py" 2>/dev/null || true
    unset COMPOSE_FILE

    # Wait for containers to fully stop
    sleep 3

    # Wipe all Docker state to prevent contamination of next test
    docker_cleanup
    verify_docker_clean

    log_success "Test $test_id complete. Results in $config_results"
}

# Generate summary report
generate_summary_report() {
    log_section "Generating Summary Report"

    local report_file="$RESULTS_DIR/PERFORMANCE-RESULTS-RC-v1.5.1.md"

    cat > "$report_file" << 'REPORT_EOF'
# Performance Test Results - RC v1.5.1-breaking (Issue #3905)

**Test Date**: %TIMESTAMP%
**Test Suite**: Incremental TPS testing across 6 configurations
**Methodology**: Start at 1000 TPS, increment 1000-2000 TPS per step until error rate >5%

## Summary Table

| Test ID | Config | Per-BVN Limit | Total Network | Notes |
|---------|--------|---------------|----------------|-------|
REPORT_EOF

    # Add per-config summary
    for config in "${CONFIGS[@]}"; do
        IFS=' ' read -r validators bvns test_id desc <<< "$config"
        local results_file="$RESULTS_DIR/${test_id}-results.csv"

        if [ -f "$results_file" ]; then
            # Find highest TPS before pushback
            local max_tps=$(awk -F',' '$11=="FALSE" {print $1}' "$results_file" | tail -1)
            local total=$(echo "$max_tps * $bvns" | bc)
            echo "| $test_id | $validators val, $bvns BVN | ~${max_tps} TPS | ~${total} TPS | See detailed results |" >> "$report_file"
        fi
    done

    cat >> "$report_file" << 'REPORT_EOF'

## Detailed Results

REPORT_EOF

    # Add per-config detailed results
    for config in "${CONFIGS[@]}"; do
        IFS=' ' read -r validators bvns test_id desc <<< "$config"
        local results_file="$RESULTS_DIR/${test_id}-results.csv"

        if [ -f "$results_file" ]; then
            echo "### Test $test_id: $desc" >> "$report_file"
            echo "" >> "$report_file"
            echo "\`\`\`" >> "$report_file"
            cat "$results_file" >> "$report_file"
            echo "\`\`\`" >> "$report_file"
            echo "" >> "$report_file"
        fi
    done

    log_success "Summary report: $report_file"
}

# Main execution
main() {
    log_section "Starting Performance Test Suite - Issue #3905"
    log "Results directory: $RESULTS_DIR"
    log "Timestamp: $TIMESTAMP"
    log "Test configurations: ${#CONFIGS[@]}"
    log "TPS sequence: ${TPS_SEQUENCE[*]}"

    # Complete cleanup before starting
    log_section "Pre-Suite Docker Cleanup"
    docker_cleanup
    verify_docker_clean

    # Build once
    build_binary

    # Run each configuration
    local passed=0
    local failed=0
    for config in "${CONFIGS[@]}"; do
        IFS=' ' read -r validators bvns test_id desc <<< "$config"
        if run_test_config "$validators" "$bvns" "$test_id" "$desc"; then
            ((passed++))
        else
            ((failed++))
            log_error "Test $test_id failed"
            # Clean up aggressively after failure to prevent state bleed
            docker_cleanup
            verify_docker_clean
        fi
    done

    # Generate report
    generate_summary_report

    # Final summary
    log_section "Test Suite Complete"
    log "Passed: $passed"
    log "Failed: $failed"
    log "Results directory: $RESULTS_DIR"
    log "Full log: $SUITE_LOG"

    # Final cleanup to leave system clean
    log_section "Post-Suite Docker Cleanup"
    docker_cleanup
    verify_docker_clean
    log_success "All resources cleaned up"
}

main "$@"
