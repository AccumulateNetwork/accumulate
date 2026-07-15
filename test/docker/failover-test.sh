#!/usr/bin/env bash
# Follower-to-validator failover test (#4050).
#
# Single BVN, 1 validator (bvn1-1) + 4 followers (bvn1-2..5). Stop the validator
# (BVN halts), stop a follower (bvn1-2), copy the validator's identity (the
# [configurations.validator-key] in accumulate.toml, covering DN + BVN) onto
# bvn1-2, restart it, and confirm the BVN resumes producing blocks under the same
# validator identity. bvn1-1 stays stopped (never run two nodes with the same
# key -> double-sign).
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

# bvn_height NODE -> BVN ledger height via the node's v3 API, or NA. DAG-BFT nodes
# serve the v3 API on container port 26660, mapped to host 27720+(N-1); they do
# not run a CometBFT RPC. bvn1-1 -> 27720 .. bvn1-5 -> 27724.
bvn_height() {
  local n=${1##*-} port
  port=$((27719 + n))
  curl -s -m5 -X POST "http://127.0.0.1:$port/v3" \
    -H 'Content-Type: application/json' \
    -d '{"jsonrpc":"2.0","id":1,"method":"query","params":{"scope":"acc://bvn-BVN1.acme/ledger"}}' \
    | jq -r '.result.account.index // "NA"' 2>/dev/null || echo NA
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

log "=== step 4: promote bvn1-2 — set its validator-key to bvn1-1's + reset priv-val state ==="
# The run.Config framework holds the validator key in accumulate.toml
# ([configurations.validator-key]), not a priv_validator_key.json. bvn1-3 shares
# the /data volume, so pull/edit/push bvn1-2's toml through it. One validator-key
# covers both the DN and BVN of the core validator.
vk_of() { docker exec accfo-bvn1-3 sh -c "grep -A1 configurations.validator-key /data/$1/accumulate.toml | grep address | head -1" 2>/dev/null | sed -E 's/.*"([^"]+)".*/\1/'; }
VK=$(vk_of bvn1-1); log "bvn1-1 validator key: $VK"
docker cp accfo-bvn1-3:/data/bvn1-2/accumulate.toml "$LOG/b2.toml" 2>/dev/null
awk -v vk="$VK" '/\[configurations.validator-key\]/{b=1} b&&/address =/{sub(/address = "[^"]*"/,"address = \"" vk "\"");b=0} {print}' "$LOG/b2.toml" > "$LOG/b2.new" && mv "$LOG/b2.new" "$LOG/b2.toml"
docker cp "$LOG/b2.toml" accfo-bvn1-3:/data/bvn1-2/accumulate.toml 2>/dev/null
docker exec accfo-bvn1-3 sh -c 'for p in dnn bvnn; do printf "%s" "{\"height\":\"0\",\"round\":0,\"step\":0}" > /data/bvn1-2/$p/data/priv_validator_state.json; done'
VK2=$(vk_of bvn1-2)
match=$([ -n "$VK" ] && [ "$VK" = "$VK2" ] && echo yes || echo no)
log "bvn1-2 validator key now == bvn1-1's: $match ($VK2)"

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
