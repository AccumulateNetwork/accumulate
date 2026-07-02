#!/usr/bin/env bash
# Acceptance test for #4049 (boot a synced node without the genesis snapshot) on
# a real 3-BVN x 4-validator dual-node docker network.
#
# Timing: the snapshot delete / marker write happens in the ABCI Info()
# handshake, which only runs at startup. So the flow is:
#   1. up            -> InitChain, produce blocks past genesis (snap still present)
#   2. restart (#2)  -> Info sees past-genesis: writes marker, deletes the .snap,
#                       strips the state-DB AppState on the *next* boot
#   3. restart (#3)  -> boots with the .snap already gone (served from the marker)
#
# PASS: after step 2 every node has deleted both genesis .snap files and written
#       the markers; after step 3 all nodes are healthy (booted with no snapshot).

set -uo pipefail
cd "$(dirname "$0")/../.."

COMPOSE="docker compose -p acc49 -f test/docker/docker-compose.net4049.yml"
NODES=(bvn1-1 bvn1-2 bvn1-3 bvn1-4 bvn2-1 bvn2-2 bvn2-3 bvn2-4 bvn3-1 bvn3-2 bvn3-3 bvn3-4)
LOG=/tmp/net4049; mkdir -p "$LOG"
log() { echo "[$(date +%T)] $*"; }

running() { docker ps --filter "name=acc49-bvn" --filter status=running -q | wc -l; }
wait_running() { for _ in $(seq 1 "$2"); do [ "$(running)" -ge "$1" ] && return 0; sleep 3; done; return 1; }

# snaps_left NODE -> number of *-genesis.snap files still in the node dir
snaps_left() { docker exec "acc49-$1" sh -c 'ls /data/'"$1"'/*.snap 2>/dev/null | wc -l' 2>/dev/null || echo NA; }
markers()    { docker exec "acc49-$1" sh -c 'find /data/'"$1"' -name "genesis-verified-*.json" 2>/dev/null | wc -l' 2>/dev/null || echo 0; }

log "=== clean slate + build + up (12 dual nodes) ==="
$COMPOSE down -v >/dev/null 2>&1
GIT_DESCRIBE=$(git describe --tags --always 2>/dev/null || echo 4049) GIT_COMMIT=$(git rev-parse --short HEAD) \
  $COMPOSE up -d --build > "$LOG/up.log" 2>&1 || { echo "up failed; see $LOG/up.log"; exit 1; }
wait_running 12 60 || { echo "nodes did not start; see: docker compose -p acc49 logs"; exit 1; }
log "12 nodes running"

log "=== step 1: let the network produce blocks past genesis (120s) ==="
sleep 120

log "=== step 2: restart all nodes (triggers marker write + snapshot delete) ==="
docker restart $(printf 'acc49-%s ' "${NODES[@]}") >/dev/null 2>&1
wait_running 12 60 || { echo "nodes did not come back after restart #2"; exit 1; }
sleep 30  # let each node's Info() run and delete

log "=== verify: snapshots deleted + markers written on every node ==="
fail=0
for n in "${NODES[@]}"; do
  s=$(snaps_left "$n"); m=$(markers "$n")
  log "  $n: snaps_left=$s markers=$m"
  { [ "$s" = 0 ] && [ "${m:-0}" -ge 1 ]; } || { fail=1; }
done
[ "$fail" = 0 ] || { echo "FAIL: some nodes still have genesis snapshots or no marker"; exit 1; }

log "=== step 3: restart again — must boot with the snapshot GONE ==="
docker restart $(printf 'acc49-%s ' "${NODES[@]}") >/dev/null 2>&1
wait_running 12 90 || { echo "FAIL: nodes did not boot without the snapshot"; exit 1; }
sleep 20
final=$(running)
log "nodes running after boot-without-snapshot: $final/12"

log "=== evidence in logs ==="
docker logs --since 3m acc49-bvn1-1 2>&1 | grep -iE "Stripped genesis AppState|Deleted genesis snapshot|Persisted genesis marker|snapshot no longer required" | head -5 || true

if [ "$final" -ge 12 ]; then
  log "PASS: all 12 nodes boot with the genesis snapshot deleted (#4049)"
  log "network left up (tear down: $COMPOSE down -v)"
  exit 0
fi
echo "FAIL: only $final/12 nodes booted without the snapshot"
exit 1
