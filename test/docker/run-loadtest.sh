#!/bin/bash
# Run a local Accumulate load test: 4 validators, 1 BVN, 1 bootstrap, metrics dashboard
#
# Usage:
#   ./run-loadtest.sh                    # Default: 16 workers, 120s, adaptive TPS
#   ./run-loadtest.sh --duration 300s    # Custom duration
#   ./run-loadtest.sh --workers 8        # 8 workers (2 per node)
#   ./run-loadtest.sh --keep             # Don't tear down after test
#   ./run-loadtest.sh --teardown         # Just tear down and exit

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
COMPOSE_FILE="$SCRIPT_DIR/docker-compose.yml"
LOG_DIR="/tmp/loadtest-workspace"
LOADTEST_LOG="$LOG_DIR/loadtest.log"
MONITORING_DIR="$LOG_DIR/monitoring"
DOCKER_LOG="$LOG_DIR/docker.log"

# Defaults
DURATION="120s"
WORKERS_PER_NODE=8
NODES=4
START_TPS=5000
MIN_TPS=1000
MAX_TPS=30000
KEEP=false
TEARDOWN_ONLY=false

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --duration)     DURATION="$2"; shift 2 ;;
        --workers)      WORKERS_PER_NODE=$(( $2 / NODES )); shift 2 ;;
        --workers-per-node) WORKERS_PER_NODE="$2"; shift 2 ;;
        --start-tps)    START_TPS="$2"; shift 2 ;;
        --min-tps)      MIN_TPS="$2"; shift 2 ;;
        --max-tps)      MAX_TPS="$2"; shift 2 ;;
        --keep)         KEEP=true; shift ;;
        --teardown)     TEARDOWN_ONLY=true; shift ;;
        *)              echo "Unknown option: $1"; exit 1 ;;
    esac
done

TOTAL_WORKERS=$(( WORKERS_PER_NODE * NODES ))

cleanup_processes() {
    echo "Stopping background processes..."
    [[ -n "${DASHBOARD_PID:-}" ]] && kill "$DASHBOARD_PID" 2>/dev/null || true
    [[ -n "${MONITORING_PID:-}" ]] && kill "$MONITORING_PID" 2>/dev/null || true
    [[ -n "${DOCKER_LOGS_PID:-}" ]] && kill "$DOCKER_LOGS_PID" 2>/dev/null || true
}

teardown() {
    echo ""
    echo "=== Tearing down ==="
    cleanup_processes
    cd "$REPO_ROOT"
    docker compose -f "$COMPOSE_FILE" down -v > "$LOG_DIR/teardown.log" 2>&1 || true
    echo "Network stopped. Logs in $LOG_DIR/"
}

trap cleanup_processes EXIT

if $TEARDOWN_ONLY; then
    teardown
    exit 0
fi

echo "============================================"
echo "  Accumulate Load Test"
echo "============================================"
echo "  Validators:  $NODES"
echo "  Workers:     $TOTAL_WORKERS ($WORKERS_PER_NODE per node)"
echo "  Duration:    $DURATION"
echo "  TPS:         $START_TPS start, $MIN_TPS min, $MAX_TPS max"
echo "  Logs:        $LOG_DIR/"
echo "  Dashboard:   http://localhost:8888/"
echo "============================================"
echo ""

# Create log directory
mkdir -p "$LOG_DIR" "$MONITORING_DIR"

# Step 1: Clean up any previous run
echo "=== Step 1: Cleaning up previous run ==="
cd "$REPO_ROOT"
docker compose -f "$COMPOSE_FILE" down -v > "$LOG_DIR/cleanup.log" 2>&1 || true
# Remove any orphaned containers with acc- prefix
docker ps -a --format "{{.Names}}" | grep -E "^acc-" | xargs -r docker rm -f >> "$LOG_DIR/cleanup.log" 2>&1 || true
docker volume ls -q | grep -E "network-config" | xargs -r docker volume rm -f >> "$LOG_DIR/cleanup.log" 2>&1 || true
docker network ls -q --filter name=docker_ | xargs -r docker network rm >> "$LOG_DIR/cleanup.log" 2>&1 || true
# Free ports used by the network
for PORT in 16593 26660 26661 26662 26663; do
    PID=$(lsof -ti:"$PORT" 2>/dev/null) && kill -9 "$PID" 2>/dev/null || true
done
echo "  Clean."

# Step 2: Build Docker image
echo "=== Step 2: Building Docker image ==="
docker compose -f "$COMPOSE_FILE" build > "$LOG_DIR/build.log" 2>&1
echo "  Build complete. (log: $LOG_DIR/build.log)"

# Step 3: Start network
echo "=== Step 3: Starting network ==="
docker compose -f "$COMPOSE_FILE" up -d > "$LOG_DIR/startup.log" 2>&1
echo "  Containers started."

# Capture container logs in background
docker compose -f "$COMPOSE_FILE" logs -f > "$DOCKER_LOG" 2>&1 &
DOCKER_LOGS_PID=$!

# Step 4: Wait for health
echo "=== Step 4: Waiting for validators to become healthy ==="
MAX_WAIT=120
WAITED=0
while [ $WAITED -lt $MAX_WAIT ]; do
    # Check if JSON-RPC responds
    if curl -s -X POST http://localhost:26660/v3 \
        -H "Content-Type: application/json" \
        -d '{"jsonrpc":"2.0","id":1,"method":"node-info","params":{}}' \
        > /dev/null 2>&1; then
        echo "  Validators healthy after ${WAITED}s"
        break
    fi
    sleep 3
    WAITED=$((WAITED + 3))
    echo "  Waiting... (${WAITED}s/${MAX_WAIT}s)"
done

if [ $WAITED -ge $MAX_WAIT ]; then
    echo "  ERROR: Validators did not become healthy within ${MAX_WAIT}s"
    echo "  Check logs: $DOCKER_LOG"
    tail -50 "$DOCKER_LOG"
    exit 1
fi

# Step 5: Start dashboard and monitoring
echo "=== Step 5: Starting dashboard and monitoring ==="
python3 "$SCRIPT_DIR/dashboard-server.py" 8888 > "$LOG_DIR/dashboard-server.log" 2>&1 &
DASHBOARD_PID=$!
python3 "$SCRIPT_DIR/monitoring.py" > "$LOG_DIR/monitoring.log" 2>&1 &
MONITORING_PID=$!

echo "  Dashboard: http://localhost:8888/"

# Step 6: Build and run load test
echo ""
echo "=== Step 6: Running load test ($TOTAL_WORKERS workers, $DURATION) ==="
echo ""

cd "$SCRIPT_DIR"
go build -o loadtest-bin parallel-loadtest.go
./loadtest-bin \
    -test-label "Manual: ${NODES}v x 1b" \
    -workers-per-node "$WORKERS_PER_NODE" \
    -nodes "$NODES" \
    -start-tps "$START_TPS" \
    -min-tps "$MIN_TPS" \
    -max-tps "$MAX_TPS" \
    -duration "$DURATION" \
    2>&1 | tee "$LOADTEST_LOG"

LOADTEST_EXIT=${PIPESTATUS[0]}

echo ""
echo "=== Load test finished (exit code: $LOADTEST_EXIT) ==="
echo ""
echo "Logs:"
echo "  Load test:   $LOADTEST_LOG"
echo "  Docker:      $DOCKER_LOG"
echo "  Monitoring:  $MONITORING_DIR/"
echo "  Metrics:     $LOG_DIR/metrics-server.log"

# Step 7: Teardown (unless --keep)
if $KEEP; then
    echo ""
    echo "Network left running (--keep). To tear down:"
    echo "  $0 --teardown"
    # Kill background log capture but leave containers
    kill "$DOCKER_LOGS_PID" 2>/dev/null || true
else
    teardown
fi

exit $LOADTEST_EXIT
