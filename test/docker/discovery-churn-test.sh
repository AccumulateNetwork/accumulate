#!/usr/bin/env bash
# Discovery-churn acceptance test for #4047 (reliable cross-partition synthetic
# delivery). This is the conformance gate the simulator cannot provide — it
# exercises real per-peer routing under churn.
#
# Scenario:
#   1. Wipe volumes and bring up a clean 3-BVN consensus-load network (nodes
#      wired to the bootstrap, so the #4047 delivery backstop is active).
#   2. Drive cross-partition load (loadmix) and wait until actors are ACTIVE
#      (past funding) so synthetics actually flow to every partition.
#   3. Induce a FULL outage on one BVN: kill ALL its nodes, hold, then restart,
#      forcing producers to re-discover live peers for that partition.
#   4. Measure each DIRECTED channel correctly: a partition's synthetic ledger
#      reports `produced` (what it SENT to a peer) and `delivered` (what it
#      EXECUTED from a peer) — opposite directions. So stuck on src->dst is
#      src.produced[dst] - dst.delivered[src], read from each partition's OWN
#      node (a remote ledger is empty when queried through another partition).
#
# PASS: backlog to the downed BVN rises during the outage, then drains to ~0
#       within the recovery window once it returns — delivery re-discovered live
#       peers via the registry backstop, no manual healing.
# FAIL: backlog stays elevated (the open wound — delivery never recovered).
#
# Usage: ./test/docker/discovery-churn-test.sh [TPS] [CHURN_BVN] [RECOVER_SECS]

set -uo pipefail
cd "$(dirname "$0")/../.."

TPS=${1:-50}
CHURN_BVN=${2:-bvn2}           # the destination partition we take fully down
RECOVER_SECS=${3:-240}         # window to drain the backlog after restart
WARMUP_SECS=${WARMUP_SECS:-300} # time for loadmix actors to become active
OUTAGE_SECS=${OUTAGE_SECS:-90}  # how long the partition stays fully down
COMPOSE="docker compose -p acc-cl -f test/docker/docker-compose.consensus-load.yml"
LOG=/tmp/churn-test
mkdir -p "$LOG"
# Faucet AS1 key for seed FAUCET (matches `faucet-key FAUCET` / genesis).
FAUCET_KEY="AS12ieB4D9CLbJ2ShfD7erRZVxqMudKCkRDbuBqw25fwwi7Bugn9B"

# First-node API port per partition (4 nodes each, +4 apart).
declare -A PORT=( [bvn1]=27680 [bvn2]=27684 [bvn3]=27688 )
NODE0=http://localhost:${PORT[bvn1]}
PARTS=(bvn1 bvn2 bvn3)
SOURCES=(); for p in "${PARTS[@]}"; do [ "$p" != "$CHURN_BVN" ] && SOURCES+=("$p"); done

log() { echo "[$(date +%T)] $*"; }

# field PORT PARTITION PEER FIELD — one ledger field for a directed channel,
# read from the partition's OWN node. Prints NA if unreadable (node down).
field() {
  curl -s -m5 "http://localhost:$1/v3" -H 'Content-Type: application/json' \
    -d "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"query\",\"params\":{\"scope\":\"acc://bvn-$2.acme/synthetic\",\"query\":{\"queryType\":\"default\"}}}" \
  | python3 -c "
import json,sys
try:
  r=json.load(sys.stdin)
  for s in r['result']['account'].get('sequence',[]):
    if '$3'.lower() in s.get('url','').lower(): print(s.get('$4',0)); break
  else: print('NA')
except Exception: print('NA')" 2>/dev/null
}

# backlog_to_churn — synthetics produced toward CHURN_BVN by the sources that
# CHURN_BVN has not yet delivered. Reads delivered from CHURN_BVN's own node;
# while it is down that side reads NA and we report the produced totals only.
backlog_to_churn() {
  local total=0 readable=1 line=""
  for s in "${SOURCES[@]}"; do
    local prod deliv
    prod=$(field "${PORT[$s]}" "$s" "$CHURN_BVN" produced)
    deliv=$(field "${PORT[$CHURN_BVN]}" "$CHURN_BVN" "$s" delivered)
    line+=" ${s}->${CHURN_BVN}:prod=${prod}"
    if [ "$deliv" = NA ] || [ "$prod" = NA ]; then readable=0; line+=",deliv=NA"
    else line+=",deliv=${deliv}"; total=$((total + prod - deliv)); fi
  done
  if [ "$readable" = 0 ]; then echo "DOWN |$line"; else echo "$total |$line"; fi
}

active_actors() { grep -oE "active=[0-9]+" "$LOG/load.log" 2>/dev/null | tail -1 | grep -oE "[0-9]+"; }

cleanup() { log "leaving network up (tear down: $COMPOSE down -v)"; }
trap cleanup EXIT

log "=== clean slate: down -v + up --build ==="
$COMPOSE down -v >/dev/null 2>&1
GIT_DESCRIBE=$(git describe --tags --always 2>/dev/null || echo dc) GIT_COMMIT=$(git rev-parse --short HEAD) \
  $COMPOSE up -d --build > "$LOG/up.log" 2>&1 || { echo "up failed; see $LOG/up.log"; exit 1; }

log "waiting 90s for consensus..."; sleep 90
echo "running nodes: $(docker ps --filter name=acc-cl-bvn --filter status=running -q | wc -l)"

log "=== start ${TPS} TPS cross-partition load ==="
/tmp/debug loadmix "$NODE0" "$FAUCET_KEY" --tps "$TPS" --duration 25m --report-interval 30s > "$LOG/load.log" 2>&1 &
LOAD_PID=$!

log "=== warmup ${WARMUP_SECS}s (wait for actors to go active) ==="
deadline=$(( $(date +%s) + WARMUP_SECS ))
while [ "$(date +%s)" -lt "$deadline" ]; do
  sleep 30; a=$(active_actors); log "  active actors=${a:-0}, backlog=$(backlog_to_churn)"
  [ "${a:-0}" -ge 5 ] && { log "actors active — proceeding"; break; }
done

log "baseline backlog to ${CHURN_BVN}: $(backlog_to_churn)"

log "=== full outage: kill ALL ${CHURN_BVN} nodes for ${OUTAGE_SECS}s ==="
docker kill "acc-cl-${CHURN_BVN}-1" "acc-cl-${CHURN_BVN}-2" "acc-cl-${CHURN_BVN}-3" "acc-cl-${CHURN_BVN}-4" >/dev/null 2>&1
sleep "$OUTAGE_SECS"
log "during outage: $(backlog_to_churn)"
log "=== restart ${CHURN_BVN} (force peer re-discovery) ==="
docker start "acc-cl-${CHURN_BVN}-1" "acc-cl-${CHURN_BVN}-2" "acc-cl-${CHURN_BVN}-3" "acc-cl-${CHURN_BVN}-4" >/dev/null 2>&1

log "=== recovery window (${RECOVER_SECS}s): backlog must drain to ~0 ==="
deadline=$(( $(date +%s) + RECOVER_SECS ))
last="?"
while [ "$(date +%s)" -lt "$deadline" ]; do
  sleep 20; bl=$(backlog_to_churn); log "  $bl"
  n=${bl%% *}
  if [ "$n" != DOWN ]; then last=$n; [ "$n" -le 2 ] && break; fi
done

kill "$LOAD_PID" 2>/dev/null
log "=== RESULT: final backlog to ${CHURN_BVN} = ${last} ==="
if [ "$last" != "?" ] && [ "${last:-99}" -le 2 ]; then
  log "PASS: synthetics drained after a full ${CHURN_BVN} outage — delivery re-discovered live peers (#4047)"
  exit 0
fi
log "FAIL: ${last} synthetics still stuck to ${CHURN_BVN} after ${RECOVER_SECS}s"
exit 1
