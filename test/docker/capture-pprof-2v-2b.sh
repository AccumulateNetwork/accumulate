#!/bin/bash
# Issue #3946: Capture pprof profile on binary submit path at steady-state load.
#
# Deploys 2v-2b topology, drives ~10K TPS for 120s, captures a 30s CPU +
# allocation profile on bvn1-val1 (pprof exposed on host 6160 via the compose
# port mapping added for this issue).

set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
TIMESTAMP=$(date +%Y%m%d-%H%M%S)
OUT_DIR="$SCRIPT_DIR/pprof-results/$TIMESTAMP"
COMPOSE="$SCRIPT_DIR/docker-compose-2-val-2-bvn.yml"
PPROF_HOST_PORT=6160
TARGET_TPS=10000
LOAD_DURATION=120
PROFILE_DURATION=30
PROFILE_DELAY=40  # seconds to wait after load start before capturing

mkdir -p "$OUT_DIR"

log() {
    echo "[$(date '+%H:%M:%S')] $*" | tee -a "$OUT_DIR/run.log"
}

cleanup() {
    log "Cleanup: docker compose down"
    docker compose -f "$COMPOSE" down -v >> "$OUT_DIR/cleanup.log" 2>&1 || true
}
trap cleanup EXIT

log "Output: $OUT_DIR"

# Fresh slate
log "Pre-clean"
docker compose -f "$COMPOSE" down -v > /dev/null 2>&1 || true
docker ps -a 2>/dev/null | grep -E "acc-.*-2v-2b" | awk '{print $1}' | xargs -r docker rm -f > /dev/null 2>&1 || true

log "Deploying 2v-2b"
if ! docker compose -f "$COMPOSE" up -d > "$OUT_DIR/compose-up.log" 2>&1; then
    log "FAIL: compose up"
    exit 1
fi

log "Waiting for init to finish"
docker compose -f "$COMPOSE" wait init > /dev/null 2>&1 || true
sleep 30

log "Waiting for network API"
for i in $(seq 1 24); do
    if curl -sf -X POST "http://localhost:26660/v3" \
        -H "Content-Type: application/json" \
        -d '{"jsonrpc":"2.0","id":1,"method":"network-status","params":{}}' \
        > /dev/null 2>&1; then
        log "Network ready"
        break
    fi
    sleep 5
    [ "$i" -eq 24 ] && { log "FAIL: network not ready"; exit 1; }
done

log "Verifying pprof endpoint"
if ! curl -sf "http://localhost:${PPROF_HOST_PORT}/debug/pprof/" > /dev/null 2>&1; then
    log "FAIL: pprof endpoint unreachable on host port ${PPROF_HOST_PORT}"
    log "  — is bvn1-val1 listening on container :6060? check cmd_init_network.go"
    exit 1
fi
log "pprof reachable"

# Load test endpoints — all 4 nodes. Ports from docker-compose-2-val-2-bvn.yml:
#   bvn1-val1=26660, bvn1-val2=26661, bvn2-val1=26662, bvn2-val2=26663
ENDPOINTS="http://localhost:26660/v3,http://localhost:26661/v3,http://localhost:26662/v3,http://localhost:26663/v3"

log "Starting load test: ${TARGET_TPS} TPS flat for ${LOAD_DURATION}s"
go run "$SCRIPT_DIR/parallel-loadtest.go" \
    -duration "${LOAD_DURATION}s" \
    -start-tps "$TARGET_TPS" \
    -max-tps "$TARGET_TPS" \
    -ramp-step 0 \
    -ramp-interval 999s \
    -nodes "$ENDPOINTS" \
    -faucet-seed "FAUCET" \
    -oracle 1000 \
    -accounts 100000 \
    -label "pprof-capture" \
    > "$OUT_DIR/loadtest.log" 2>&1 &
LOAD_PID=$!

log "Load test PID $LOAD_PID; sleeping ${PROFILE_DELAY}s for steady state"
sleep "$PROFILE_DELAY"

# Sanity: is load test still running?
if ! kill -0 "$LOAD_PID" 2>/dev/null; then
    log "FAIL: load test exited early"
    tail -20 "$OUT_DIR/loadtest.log" | tee -a "$OUT_DIR/run.log"
    exit 1
fi

log "Capturing ${PROFILE_DURATION}s CPU profile (foreground, blocks)"
curl -v -o "$OUT_DIR/cpu.pprof" \
    "http://localhost:${PPROF_HOST_PORT}/debug/pprof/profile?seconds=${PROFILE_DURATION}" \
    > "$OUT_DIR/cpu-curl.log" 2>&1
CPU_RC=$?
if [ $CPU_RC -ne 0 ] || [ ! -s "$OUT_DIR/cpu.pprof" ]; then
    log "FAIL: CPU profile curl rc=$CPU_RC size=$(stat -c %s "$OUT_DIR/cpu.pprof" 2>/dev/null || echo 0)"
    log "  see $OUT_DIR/cpu-curl.log for details"
    tail -20 "$OUT_DIR/cpu-curl.log" | tee -a "$OUT_DIR/run.log"
    exit 1
fi
log "CPU profile: $(stat -c %s "$OUT_DIR/cpu.pprof") bytes"

log "Capturing heap (alloc_space) snapshot"
curl -sf -o "$OUT_DIR/heap.pprof" \
    "http://localhost:${PPROF_HOST_PORT}/debug/pprof/heap" || log "WARN: heap capture failed"

log "Capturing allocs snapshot"
curl -sf -o "$OUT_DIR/allocs.pprof" \
    "http://localhost:${PPROF_HOST_PORT}/debug/pprof/allocs" || log "WARN: allocs capture failed"

log "Waiting for load test to finish"
wait "$LOAD_PID" 2>/dev/null || true

# Analysis
log "Running pprof -top for CPU"
go tool pprof -top -cum "$OUT_DIR/cpu.pprof" 2>&1 | head -40 > "$OUT_DIR/cpu-top.txt" || true

log "Running pprof -top for alloc_space"
go tool pprof -top -sample_index=alloc_space "$OUT_DIR/allocs.pprof" 2>&1 | head -40 > "$OUT_DIR/alloc-top.txt" || true

log "Running pprof -top for alloc_objects"
go tool pprof -top -sample_index=alloc_objects "$OUT_DIR/allocs.pprof" 2>&1 | head -40 > "$OUT_DIR/alloc-objects-top.txt" || true

# Summary
log "Capture complete. Results in $OUT_DIR"
log "Load test summary:"
grep -E "^  (Submitted|Failed|Peak TPS|Average TPS):" "$OUT_DIR/loadtest.log" 2>/dev/null | tee -a "$OUT_DIR/run.log" || true
