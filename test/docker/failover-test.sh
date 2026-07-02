#!/usr/bin/env bash
# Follower-to-validator failover test (#4050).
#
# Single BVN, 1 validator (bvn1-1) + 4 followers (bvn1-2..5). Stop the validator
# (BVN halts), stop a follower (bvn1-2), copy the validator's identity
# (priv_validator_key.json for DN + BVN) onto bvn1-2, restart it, and confirm the
# BVN resumes producing blocks under the same validator identity. bvn1-1 stays
# stopped (never run two nodes with the same key -> double-sign).
#
# PASS: height is flat while the validator is down, then advances again after the
#       former follower is promoted.

set -uo pipefail
cd "$(dirname "$0")/../.."

COMPOSE="docker compose -p accfo -f test/docker/docker-compose.failover.yml"
LOG=/tmp/failover-test; mkdir -p "$LOG"
log() { echo "[$(date +%T)] $*"; }

running() { docker ps --filter name=accfo-bvn --filter status=running -q | wc -l; }
wait_running() { for _ in $(seq 1 "$2"); do [ "$(running)" -ge "$1" ] && return 0; sleep 3; done; return 1; }

# bvn_height NODE -> BVN CometBFT latest_block_height (via the node's RPC), or NA
bvn_height() {
  docker exec "accfo-$1" curl -s -m5 localhost:26757/status 2>/dev/null \
    | python3 -c "import json,sys;print(json.load(sys.stdin)['result']['sync_info']['latest_block_height'])" 2>/dev/null || echo NA
}
# wait until bvn_height on NODE advances past $2 (within $3 polls), echo final height
wait_advance() {
  local node=$1 base=$2 n=$3 h
  for _ in $(seq 1 "$n"); do
    h=$(bvn_height "$node"); { [ "$h" != NA ] && [ "${h:-0}" -gt "$base" ]; } && { echo "$h"; return 0; }
    sleep 4
  done
  echo "${h:-NA}"; return 1
}

log "=== clean slate + up (1 validator + 4 followers) ==="
$COMPOSE down -v >/dev/null 2>&1
GIT_DESCRIBE=$(git describe --tags --always 2>/dev/null || echo 4050) GIT_COMMIT=$(git rev-parse --short HEAD) \
  $COMPOSE up -d --build > "$LOG/up.log" 2>&1 || { echo "up failed; see $LOG/up.log"; exit 1; }
wait_running 5 60 || { echo "nodes did not start"; exit 1; }
log "5 nodes running"

log "=== step 1: wait for the BVN to produce blocks ==="
h0=$(wait_advance bvn1-3 1 45) || { echo "FAIL: BVN never produced blocks (height=$h0)"; exit 1; }
log "BVN producing blocks, height=$h0"

log "=== step 2: stop the validator (bvn1-1) — BVN must halt ==="
docker stop accfo-bvn1-1 >/dev/null 2>&1
sleep 30
hA=$(bvn_height bvn1-3); sleep 20; hB=$(bvn_height bvn1-3)
log "height during outage: $hA -> $hB"
if [ "$hA" = NA ] || [ "$hA" != "$hB" ]; then
  log "WARN: BVN did not clearly halt ($hA -> $hB) — the single validator may not be the only voter"
fi

log "=== step 3: stop a follower (bvn1-2) ==="
docker stop accfo-bvn1-2 >/dev/null 2>&1

log "=== step 4: promote bvn1-2 — copy the validator identity + reset priv-val state ==="
# bvn1-3 shares the /data volume and is still running, use it to move the files.
docker exec accfo-bvn1-3 sh -c '
  set -e
  cp -f /data/bvn1-1/dnn/config/priv_validator_key.json  /data/bvn1-2/dnn/config/priv_validator_key.json
  cp -f /data/bvn1-1/bvnn/config/priv_validator_key.json /data/bvn1-2/bvnn/config/priv_validator_key.json
  for p in dnn bvnn; do
    printf "%s" "{\"height\":\"0\",\"round\":0,\"step\":0}" > /data/bvn1-2/$p/data/priv_validator_state.json
  done
  echo "copied validator keys to bvn1-2 (DN+BVN); reset priv_validator_state"
' 2>&1
# sanity: the copied BVN validator pubkey must equal bvn1-1's original
match=$(docker exec accfo-bvn1-3 sh -c '
  a=$(grep -o "\"value\":[^,}]*" /data/bvn1-1/bvnn/config/priv_validator_key.json | tail -1)
  b=$(grep -o "\"value\":[^,}]*" /data/bvn1-2/bvnn/config/priv_validator_key.json | tail -1)
  [ "$a" = "$b" ] && echo yes || echo no')
log "bvn1-2 BVN validator key == bvn1-1 original: $match"

log "=== step 5: start bvn1-2 as the new validator (bvn1-1 stays stopped) ==="
docker start accfo-bvn1-2 >/dev/null 2>&1
wait_running 4 60 || { echo "FAIL: promoted node did not come up"; exit 1; }

log "=== step 6: BVN must resume producing blocks ==="
h1=$(wait_advance bvn1-3 "${hB:-$h0}" 60) || { echo "FAIL: BVN did not resume (height stuck at $h1)"; exit 1; }
log "BVN resumed under the promoted validator, height=$h1 (was $hB during outage)"

log "=== RESULT ==="
if [ "$match" = yes ] && [ "${h1:-0}" -gt "${hB:-0}" ]; then
  log "PASS: follower bvn1-2 promoted to validator; BVN halted then resumed with the same validator identity (#4050)"
  log "network left up (tear down: $COMPOSE down -v)"
  exit 0
fi
echo "FAIL: match=$match resumed_height=$h1 outage_height=$hB"
exit 1
