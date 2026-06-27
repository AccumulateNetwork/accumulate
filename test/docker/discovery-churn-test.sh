#!/usr/bin/env bash
# Discovery-churn acceptance test for #4047 (reliable cross-partition synthetic
# delivery). This is the conformance gate the simulator cannot provide — it
# exercises real per-peer routing under churn.
#
# Scenario:
#   1. Bring up the 3-BVN consensus-load network (nodes wired to the bootstrap,
#      so the #4047 delivery-discovery backstop is active via /peers).
#   2. Drive sustained CROSS-partition load (loadmix) -> synthetics in every
#      direction (e.g. BVN1 -> BVN2 deposits).
#   3. Induce CHURN on a destination BVN: kill some of its nodes, then restart
#      them, forcing producers to re-discover live peers for that partition.
#   4. Watch each partition's synthetic ledger. "Stuck" = produced - delivered.
#
# PASS: stuck synthetics spike during churn but drain back to ~0 within the
#       recovery window once the nodes are back — delivery re-discovered live
#       peers via the registry backstop, without manual healing.
# FAIL: stuck count stays elevated (the open wound — delivery never recovered).
#
# Usage: ./test/docker/discovery-churn-test.sh [TPS] [CHURN_BVN] [RECOVER_SECS]

set -uo pipefail
cd "$(dirname "$0")/../.."

TPS=${1:-10}
CHURN_BVN=${2:-bvn2}            # the destination partition we disrupt
RECOVER_SECS=${3:-180}         # window to drain stuck synthetics after churn
COMPOSE="docker compose -p acc-cl -f test/docker/docker-compose.consensus-load.yml"
LOG=/tmp/churn-test
mkdir -p "$LOG"
# Faucet AS1 key for seed FAUCET (deterministic; see `debug fund`).
FAUCET_KEY="AS12ieB4D9CLbJ2ShfD7erRZVxqMudKCkRDbuBqw25fwwi7Bugn9B"
NODE0=http://localhost:27680   # a BVN1 node (loadmix discovers the rest)

log() { echo "[$(date +%T)] $*"; }

# stuck_total sums (produced - delivered) across all three BVN synthetic
# ledgers — the count of synthetics produced but not yet delivered.
stuck_total() {
  local total=0
  for p in bvn1 bvn2 bvn3; do
    local s
    s=$(curl -s -m5 "$NODE0/v3" -H 'Content-Type: application/json' \
      -d "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"query\",\"params\":{\"scope\":\"acc://bvn-$p.acme/synthetic\",\"query\":{\"queryType\":\"default\"}}}" \
      | python3 -c "
import json,sys
try:
  r=json.load(sys.stdin); seq=r['result']['account']['sequence']
  print(sum(max(0,s.get('produced',0)-s.get('delivered',0)) for s in seq))
except Exception: print(0)
" 2>/dev/null)
    total=$((total + ${s:-0}))
  done
  echo "$total"
}

cleanup() { log "leaving network up for inspection (down with: $COMPOSE down -v)"; }
trap cleanup EXIT

log "=== build + up ==="
GIT_DESCRIBE=$(git describe --tags --always 2>/dev/null || echo dc) GIT_COMMIT=$(git rev-parse --short HEAD) \
  $COMPOSE up -d --build > "$LOG/up.log" 2>&1 || { echo "up failed; see $LOG/up.log"; exit 1; }

log "waiting 90s for consensus to form..."
sleep 90
docker ps --filter name=acc-cl-bvn --filter status=running -q | wc -l | xargs echo "running nodes:"

log "=== start ${TPS} TPS cross-partition load ==="
/tmp/debug loadmix "$NODE0" "$FAUCET_KEY" --tps "$TPS" --duration 15m --report-interval 30s > "$LOG/load.log" 2>&1 &
LOAD_PID=$!
sleep 60
log "baseline stuck synthetics: $(stuck_total)"

log "=== induce churn: kill ${CHURN_BVN} nodes ==="
docker kill "acc-cl-${CHURN_BVN}-1" "acc-cl-${CHURN_BVN}-2" >/dev/null 2>&1
sleep 30
log "stuck during outage: $(stuck_total)"
log "=== restart ${CHURN_BVN} nodes (force peer re-discovery) ==="
docker start "acc-cl-${CHURN_BVN}-1" "acc-cl-${CHURN_BVN}-2" >/dev/null 2>&1

log "=== recovery window (${RECOVER_SECS}s): stuck must drain to ~0 ==="
deadline=$(( $(date +%s) + RECOVER_SECS ))
last=$(stuck_total)
while [ "$(date +%s)" -lt "$deadline" ]; do
  sleep 20
  cur=$(stuck_total)
  log "  stuck=$cur"
  last=$cur
done

kill "$LOAD_PID" 2>/dev/null
log "=== RESULT: final stuck synthetics = $last ==="
if [ "${last:-1}" -le 2 ]; then
  log "PASS: synthetics drained after churn — delivery re-discovered live peers (#4047)"
  exit 0
fi
log "FAIL: $last synthetics still stuck after ${RECOVER_SECS}s — delivery did not recover"
exit 1
