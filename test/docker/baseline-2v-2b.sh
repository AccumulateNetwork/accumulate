#!/bin/bash
# Baseline: just run the docker test network + 10K TPS load test.
# No pprof, no profile capture. Proves the code runs.
#
# Usage: baseline-2v-2b.sh [TPS] [DURATION_SEC]

set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
TIMESTAMP=$(date +%Y%m%d-%H%M%S)
OUT_DIR="$SCRIPT_DIR/baseline-results/$TIMESTAMP"
COMPOSE="$SCRIPT_DIR/docker-compose-2-val-2-bvn.yml"
TARGET_TPS="${1:-10000}"
LOAD_DURATION="${2:-120}"

mkdir -p "$OUT_DIR"

log() { echo "[$(date '+%H:%M:%S')] $*" | tee -a "$OUT_DIR/run.log"; }

cleanup() {
    log "Capturing container state before teardown"
    docker ps -a --filter "name=acc-" --format "{{.Names}}\t{{.Status}}" > "$OUT_DIR/container-state.txt" 2>&1 || true
    for svc in bvn1-val1 bvn1-val2 bvn2-val1 bvn2-val2; do
        docker logs "acc-${svc}-2v-2b" > "$OUT_DIR/log-${svc}.txt" 2>&1 || true
    done
    docker stats --no-stream --format "{{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}" > "$OUT_DIR/docker-stats.txt" 2>&1 || true
    log "Cleanup: docker compose down"
    docker compose -f "$COMPOSE" down -v >> "$OUT_DIR/cleanup.log" 2>&1 || true
}
trap cleanup EXIT

log "Output: $OUT_DIR"
log "TPS=$TARGET_TPS duration=${LOAD_DURATION}s"

log "Pre-clean"
docker compose -f "$COMPOSE" down -v > /dev/null 2>&1 || true

log "Deploying 2v-2b"
if ! docker compose -f "$COMPOSE" up -d > "$OUT_DIR/compose-up.log" 2>&1; then
    log "FAIL: compose up"
    exit 1
fi

log "Waiting for init"
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

# All 4 real ports
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
    -label "baseline" \
    > "$OUT_DIR/loadtest.log" 2>&1 &
LOAD_PID=$!

# Sample docker stats every 5s while load runs
(
    while kill -0 "$LOAD_PID" 2>/dev/null; do
        ts=$(date '+%H:%M:%S')
        docker stats --no-stream --format "{{.Name}} cpu={{.CPUPerc}} mem={{.MemUsage}}" 2>/dev/null \
            | grep -E "acc-.*-2v-2b" \
            | sed "s/^/[$ts] /"
        sleep 5
    done
) > "$OUT_DIR/docker-stats-timeline.txt" 2>&1 &
STATS_PID=$!

log "Load test PID $LOAD_PID; sampling docker stats every 5s"

wait "$LOAD_PID" 2>/dev/null
LOAD_RC=$?
kill "$STATS_PID" 2>/dev/null || true

log "Load test exit code: $LOAD_RC"
log "Load test summary:"
grep -E "^  (Submitted|Failed|Peak TPS|Average TPS):" "$OUT_DIR/loadtest.log" 2>/dev/null | tee -a "$OUT_DIR/run.log" || true

# Peak memory per container
log "Peak memory seen (from timeline):"
awk '{for (i=1;i<=NF;i++) if ($i ~ /^mem=/) { split($i,a,"="); split(a[2],b,"/"); print $1, b[1] }}' \
    "$OUT_DIR/docker-stats-timeline.txt" \
    | awk '{gsub("MiB","",$2); gsub("GiB","000",$2); if ($2+0 > peak[$1]) peak[$1]=$2} END {for (k in peak) printf "  %s: peak ≈ %.0f MiB\n", k, peak[k]}' \
    | tee -a "$OUT_DIR/run.log"

log "Complete. Results in $OUT_DIR"
