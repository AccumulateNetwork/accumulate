#!/usr/bin/env bash
# Load-test ramp: 16 workers across 4 BVN1 validators, target TPS rises
# from START until breakage. Each rung also checks the BVN synthetic
# ledger before/after — if delivered falls behind received, that's the
# real bottleneck even when submission looks clean.
#
# Usage: ./test/docker/loadtest-ramp.sh [START] [STEP] [DUR] [MAX]
#   START  default 5
#   STEP   default 5
#   DUR    default 30s
#   MAX    default 1000

set -e
cd "$(dirname "$0")/../.."

START=${1:-5}
STEP=${2:-5}
DUR=${3:-30s}
MAX=${4:-1000}

LOG=/tmp/loadramp.log
ENDPOINTS=http://localhost:26680,http://localhost:26681,http://localhost:26682,http://localhost:26683
WORKERS_PER=4

log() { echo "[$(date +%T)] $*" | tee -a "$LOG"; }
fail() { log "FAIL: $*"; exit 1; }

synth_state() {
  curl -s http://localhost:26680/v3 -H 'Content-Type: application/json' \
    -d '{"jsonrpc":"2.0","id":1,"method":"query","params":{"scope":"acc://bvn-bvn1.acme/synthetic","query":{"queryType":"default"}}}' \
    | python3 -c "
import json,sys
r=json.load(sys.stdin)
for s in r['result']['account']['sequence']:
    print(f\"produced={s.get('produced',0)} received={s.get('received',0)} delivered={s.get('delivered',0)}\")
" 2>/dev/null | head -1
}

dn_height() {
  curl -s http://localhost:26690/status 2>/dev/null \
    | python3 -c "import json,sys; print(json.load(sys.stdin)['result']['sync_info']['latest_block_height'])" 2>/dev/null
}

: > "$LOG"

log "=== build tools ==="
go build -o debug ./tools/cmd/debug/ >>"$LOG" 2>&1 || fail "build debug"

log "=== verify network alive ==="
H=$(dn_height)
[ -n "$H" ] || fail "network not responding"
log "DN height: $H"
log "synth ledger before: $(synth_state)"

LAST_OK=0
log "=== ramp from ${START} TPS by ${STEP} (duration ${DUR}, max ${MAX}, workers=$((WORKERS_PER*4))) ==="

for ((tps=$START; tps<=$MAX; tps+=$STEP)); do
  log ""
  log "--- TPS=$tps ---"
  S_BEFORE=$(synth_state)
  H_BEFORE=$(dn_height)
  log "  before: synth=$S_BEFORE  dn-h=$H_BEFORE"

  RC=0
  ./debug loadparallel "$ENDPOINTS" \
    --tps "$tps" --duration "$DUR" \
    --workers-per-endpoint "$WORKERS_PER" \
    --tokens-per-worker 10000000000 --credits-per-worker 100 \
    --report-interval 5s 2>&1 | tee -a "$LOG" || RC=$?

  S_AFTER=$(synth_state)
  H_AFTER=$(dn_height)
  log "  after:  synth=$S_AFTER  dn-h=$H_AFTER"

  if [ "$RC" -ne 0 ]; then
    log "TPS=$tps BROKE on submit/error checks (rc=$RC) — stopping"
    break
  fi

  # Synthetic-delivery health check: did delivered keep up with received?
  REC=$(echo "$S_AFTER" | sed -E 's/.*received=([0-9]+).*/\1/')
  DEL=$(echo "$S_AFTER" | sed -E 's/.*delivered=([0-9]+).*/\1/')
  if [ -n "$REC" ] && [ -n "$DEL" ] && [ "$REC" -gt 0 ]; then
    LAG=$((REC - DEL))
    PCT=$((100*DEL/REC))
    if [ "$LAG" -gt 100 ] && [ "$PCT" -lt 90 ]; then
      log "TPS=$tps SYNTHETIC DELIVERY BEHIND (received=$REC delivered=$DEL lag=$LAG = $PCT%) — stopping"
      break
    fi
  fi

  LAST_OK=$tps
  log "TPS=$tps OK"
  sleep 3
done

log ""
log "=== ramp complete ==="
log "last passing TPS: $LAST_OK"
log "log: $LOG"
