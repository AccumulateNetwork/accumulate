#!/usr/bin/env bash
# Docker test for #4064/#4067: reproduce a wedged synthetic stream and prove
# receiver-pull healing recovers it over a real libp2p network.
#
# The load generator is the driver's MIXED workload — every user transaction
# type that produces a cross-partition synthetic — so this covers every heal
# path, not just token deposits. The verdict fails if any expected synthetic
# type was never produced.
#
#   ./run-test.sh                 # build image if needed, run, tear down
#   KEEP=1 ./run-test.sh          # leave the network up afterward
#   DROP='*:%16' ./run-test.sh    # recurring drops instead of one per node
#   WORKLOAD=transfers ./run-test.sh   # the original lite->lite-only repro
#   COUNT=128 ./run-test.sh       # longer run
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
repo="$(cd "$here/../../.." && pwd)"
cd "$here"
compose="docker compose -f docker-compose.yml"

cleanup() { [ -n "${KEEP:-}" ] || $compose down -v --remove-orphans >/dev/null 2>&1 || true; }
trap cleanup EXIT

if ! docker image inspect acc-synthheal:test >/dev/null 2>&1; then
  echo "== Building image acc-synthheal:test =="
  docker build -t acc-synthheal:test -f "$repo/Dockerfile" "$repo"
fi

echo "== Starting network =="
$compose up -d

echo "== Waiting for the node JSON-RPC API =="
up=""
for _ in $(seq 1 60); do
  if curl -sf -X POST http://localhost:26660/v3 \
       -H 'content-type: application/json' \
       -d '{"jsonrpc":"2.0","id":1,"method":"network-status","params":{"partition":"Directory"}}' \
       >/dev/null 2>&1; then
    up=1; break
  fi
  sleep 3
done
[ -n "$up" ] || { echo "node API never came up"; $compose logs bvn1 | tail -30; exit 1; }
echo "API is serving; letting the network produce a few blocks..."
sleep 15

echo "== Driving the ${WORKLOAD:-mixed} workload (drop pattern ${DROP:-*:1}) =="
rc=0
go run "$repo/test/docker/synth-heal/driver" -endpoint http://localhost:26660 \
  -workload "${WORKLOAD:-mixed}" -count "${COUNT:-64}" -timeout "${TIMEOUT:-20m}" || rc=$?

echo
echo "== Evidence =="
echo "-- drop (wedge formed) --"
# "synthetic envelope" is the *:N form, "sequenced envelope" the *:%K form.
$compose logs 2>/dev/null | grep -iE "dropping (synthetic|sequenced) envelope" | head -3 || echo "(none)"
echo "-- heal counters via ConsensusStatus, every validator (receiver-pull fired) --"
# The jitter picks WHICH validator pulls (occasionally two overlap — harmless,
# duplicates are idempotent), so ask every node for its own counters.
heals=""
for c in $(docker ps --filter name=acc-bvn --format '{{.Names}}'); do
  h=$(docker exec "$c" sh -c '
    nid=$(curl -s -X POST http://localhost:26660/v3 -H "content-type: application/json" -d "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"node-info\",\"params\":{}}" | grep -oE "\"peerID\":\"[^\"]+\"" | cut -d"\"" -f4)
    for part in Directory BVN1 BVN2 BVN3; do
      curl -s -X POST http://localhost:26660/v3 -H "content-type: application/json" -d "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"consensus-status\",\"params\":{\"partition\":\"$part\",\"nodeID\":\"$nid\"}}" | grep -oE "\"(syntheticHeals|anchorHeals)\":[0-9]+" | sed "s/^/$part /"
    done' 2>/dev/null || true)
  [ -n "$h" ] && { echo "  $c:"; echo "$h" | sed 's/^/    /'; heals=1; }
done
[ -n "$heals" ] || echo "(no heals recorded)"

if [ "$rc" -eq 0 ] && [ -n "$heals" ]; then
  echo; echo "RESULT: PASS (stream wedged and healed via receiver-pull)"
  rc=0
elif [ "$rc" -eq 0 ]; then
  echo; echo "RESULT: INCONCLUSIVE (workload delivered but no heal was recorded)"
  rc=1
else
  echo; echo "RESULT: FAIL (driver exit $rc)"
fi
exit "$rc"
