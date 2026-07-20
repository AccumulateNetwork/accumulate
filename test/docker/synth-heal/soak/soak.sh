#!/usr/bin/env bash
# 24h+ chaos soak: 3 BVNs x 4 validators + bootstrap, 2 TPS cross-partition
# load, induced errors. Chaos restarts re-arm each node's synthetic-drop hook
# (ACC_DEBUG_DROP_SYNTHETIC), so drops recur throughout the run.
#   DURATION=24h TPS=2 ./soak.sh
set -uo pipefail
here="$(cd "$(dirname "$0")" && pwd)"; repo="$(cd "$here/../../../.." && pwd)"
DURATION="${DURATION:-24h}"; TPS="${TPS:-2}"
log="$here/soak.log"; chaos="$here/chaos.log"; mon="$here/monitor.csv"
compose="docker compose -f $here/docker-compose.yml"

echo "== soak start $(date -u) duration=$DURATION tps=$TPS ==" | tee "$log"
$compose down -v --remove-orphans >/dev/null 2>&1
$compose up -d >/dev/null 2>&1 || { echo "up failed"; exit 1; }
for _ in $(seq 1 90); do
  curl -sf -X POST http://localhost:26660/v3 -H 'content-type: application/json' \
    -d '{"jsonrpc":"2.0","id":1,"method":"network-status","params":{"partition":"Directory"}}' >/dev/null 2>&1 && break
  sleep 5
done
up=""; for _ in $(seq 1 60); do curl -sf -X POST http://localhost:26660/v3 -H "content-type: application/json" -d "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"network-status\",\"params\":{\"partition\":\"Directory\"}}" >/dev/null 2>&1 && { up=1; break; }; sleep 5; done; [ -n "$up" ] || { echo "network never came up" | tee -a "$log"; exit 1; }; sleep 30

# Load driver (host)
nohup go run "$repo/test/docker/synth-heal/driver" -endpoint http://localhost:26660 \
  -tps "$TPS" -duration "$DURATION" -timeout 26h >> "$log" 2>&1 &
DRIVER=$!

# Chaos: every ~10 min disturb ONE random node (quorum 3/4 preserved)
( end=$(( $(date +%s) + $(( $(echo "$DURATION" | sed 's/h//') * 3600 )) ))
  nodes=$(docker ps --filter name=acc-s --format '{{.Names}}')
  while [ "$(date +%s)" -lt "$end" ]; do
    sleep $(( 480 + RANDOM % 240 ))
    n=$(echo "$nodes" | shuf -n1); r=$((RANDOM % 10))
    if [ "$r" -lt 4 ]; then
      echo "$(date -u +%T) restart $n" >> "$chaos"; docker restart "$n" >/dev/null 2>&1
    elif [ "$r" -lt 8 ]; then
      p=$((60 + RANDOM % 120))
      echo "$(date -u +%T) pause $n ${p}s" >> "$chaos"
      docker pause "$n" >/dev/null 2>&1; sleep "$p"; docker unpause "$n" >/dev/null 2>&1
    else
      echo "$(date -u +%T) skip" >> "$chaos"
    fi
  done ) &
CHAOS=$!

# Monitor: heights + total heals every 5 min
echo "time,dnHeight,heals" > "$mon"
( while kill -0 $DRIVER 2>/dev/null; do
    h=$(curl -s -X POST http://localhost:26660/v3 -H 'content-type: application/json' \
      -d '{"jsonrpc":"2.0","id":1,"method":"query","params":{"scope":"acc://dn.acme/ledger"}}' \
      | grep -oE '"index":[0-9]+' | head -1 | cut -d: -f2)
    heals=0
    for c in $(docker ps --filter name=acc-s --format '{{.Names}}'); do
      x=$(docker exec "$c" sh -c '
        nid=$(curl -s -X POST http://localhost:26660/v3 -H "content-type: application/json" -d "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"node-info\",\"params\":{}}" | grep -oE "\"peerID\":\"[^\"]+\"" | cut -d"\"" -f4)
        for part in Directory BVN1 BVN2 BVN3; do
          curl -s -X POST http://localhost:26660/v3 -H "content-type: application/json" -d "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"consensus-status\",\"params\":{\"partition\":\"$part\",\"nodeID\":\"$nid\"}}" | grep -oE "\"(syntheticHeals|anchorHeals)\":[0-9]+" | cut -d: -f2
        done' 2>/dev/null | paste -sd+ - | bc 2>/dev/null)
      heals=$((heals + ${x:-0}))
    done
    echo "$(date -u +%FT%T),${h:-?},$heals" >> "$mon"
    up=""; for _ in $(seq 1 60); do curl -sf -X POST http://localhost:26660/v3 -H "content-type: application/json" -d "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"network-status\",\"params\":{\"partition\":\"Directory\"}}" >/dev/null 2>&1 && { up=1; break; }; sleep 5; done; [ -n "$up" ] || { echo "network never came up" | tee -a "$log"; exit 1; }; sleep 300
  done ) &

wait $DRIVER; rc=$?
kill $CHAOS 2>/dev/null
echo "== soak finished $(date -u) driver-exit=$rc ==" | tee -a "$log"
tail -3 "$log"; tail -3 "$mon"; wc -l "$chaos"
exit $rc
