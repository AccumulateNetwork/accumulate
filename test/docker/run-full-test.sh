#!/bin/bash
# Full 10K TPS test setup - starts everything automatically
# Usage: ./run-full-test.sh [duration_seconds] [target_tps]

set -e

DURATION="${1:-300}"  # 5 minutes default
TARGET_TPS="${2:-10000}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

echo "==============================================="
echo "  Accumulate 10K TPS Test Network"
echo "==============================================="
echo ""
echo "Configuration:"
echo "  Duration: ${DURATION}s"
echo "  Target TPS: ${TARGET_TPS}"
echo "  Network: 1 bootstrap + 12 validators (3 BVNs × 4)"
echo "  Workers: 48 (4 per node)"
echo ""

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Step 1: Build binary
echo -e "${YELLOW}[1/6]${NC} Building accumulated binary..."
cd "$REPO_ROOT"
if ! go build ./cmd/accumulated 2>/dev/null; then
    echo -e "${RED}✗ Build failed${NC}"
    exit 1
fi
echo -e "${GREEN}✓ Binary built${NC}"

# Step 2: Build Docker images
echo -e "${YELLOW}[2/6]${NC} Building Docker images..."
cd "$SCRIPT_DIR"
if ! docker compose build > /dev/null 2>&1; then
    echo -e "${RED}✗ Docker build failed${NC}"
    exit 1
fi
echo -e "${GREEN}✓ Docker images built${NC}"

# Step 3: Start network
echo -e "${YELLOW}[3/6]${NC} Starting network..."
docker compose down -v 2>/dev/null || true
docker compose up -d > /dev/null 2>&1
sleep 30
if ! docker compose ps | grep -q "healthy"; then
    echo -e "${RED}✗ Network failed to start${NC}"
    docker compose logs bootstrap | tail -20
    exit 1
fi
echo -e "${GREEN}✓ Network started (12 validators healthy)${NC}"

# Step 4: Start monitoring
echo -e "${YELLOW}[4/6]${NC} Starting metrics collection..."
mkdir -p /tmp/monitoring-results
pkill -f "monitor.py" 2>/dev/null || true
python3 "$SCRIPT_DIR/monitor.py" /tmp/monitoring-results 3600 10 > /tmp/monitor.log 2>&1 &
MONITOR_PID=$!
sleep 2
echo -e "${GREEN}✓ Monitoring started (PID: $MONITOR_PID)${NC}"

# Step 5: Start dashboard
echo -e "${YELLOW}[5/6]${NC} Starting dashboard..."
pkill -f "metrics-server.py" 2>/dev/null || true
sleep 1
python3 "$SCRIPT_DIR/metrics-server.py" /tmp/loadtest.log /tmp/monitoring-results 8888 > /tmp/metrics-server.log 2>&1 &
DASHBOARD_PID=$!
sleep 2
echo -e "${GREEN}✓ Dashboard started (PID: $DASHBOARD_PID)${NC}"

# Reset metrics before starting test
sleep 1
curl -s "http://localhost:8888/reset" > /dev/null 2>&1 || echo "Warning: Could not reset metrics"

# Step 6: Run load test
echo -e "${YELLOW}[6/6]${NC} Starting load test..."
echo ""
echo "═══════════════════════════════════════════════════"
echo -e "  ${GREEN}Dashboard: http://localhost:8888/${NC}"
echo "═══════════════════════════════════════════════════"
echo ""

cd "$SCRIPT_DIR"
# For adaptive testing: start at 15K, ramp down to 10K, then up to find ceiling
go run parallel-loadtest.go \
    -start-tps 15000 \
    -min-tps 10000 \
    -max-tps 30000 \
    -ramp-down-duration 1m \
    -ramp-up-duration 30s \
    -error-threshold 0.05 \
    -duration "${DURATION}s" \
    2>&1 | tee /tmp/loadtest.log

# Summary
echo ""
echo "═══════════════════════════════════════════════════"
echo "Test Complete"
echo "═══════════════════════════════════════════════════"
echo ""
echo "Results:"
tail /tmp/loadtest.log | grep -E "Duration:|Submitted:|Success:|Failed:|Average TPS:"
echo ""
echo "Monitoring data:"
ls -lh /tmp/monitoring-results/*.csv 2>/dev/null | head -5 || echo "No monitoring files yet"
echo ""
echo "To view detailed metrics:"
echo "  - Dashboard: http://localhost:8888/"
echo "  - API: http://localhost:8888/metrics"
echo ""
echo "To stop everything:"
echo "  pkill -f parallel-loadtest"
echo "  pkill -f metrics-server.py"
echo "  pkill -f monitor.py"
echo "  docker compose down -v"
echo ""
