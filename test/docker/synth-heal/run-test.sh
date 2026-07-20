#!/usr/bin/env bash
# Docker test for #4064: reproduce a wedged synthetic stream and prove
# receiver-pull healing recovers it over a real libp2p network.
#
#   ./run-test.sh            # build image if needed, run, tear down
#   KEEP=1 ./run-test.sh     # leave the network up afterward for inspection
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

echo "== Driving cross-partition sends (first synthetic is dropped) =="
rc=0
go run "$repo/test/docker/synth-heal/driver" -endpoint http://localhost:26660 -count 5 -timeout 240s || rc=$?

echo
echo "== Evidence =="
echo "-- drop (wedge formed) --"
$compose logs 2>/dev/null | grep -i "dropping synthetic envelope" | head -3 || echo "(none)"
echo "-- heal counters via ConsensusStatus (receiver-pull fired) --"
heals=""
nid=$(curl -s -X POST http://localhost:26660/v3 -H 'content-type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"node-info","params":{}}' | grep -oE '"peerID":"[^"]+"' | cut -d'"' -f4)
for part in Directory BVN1 BVN2 BVN3; do
  h=$(curl -s -X POST http://localhost:26660/v3 -H 'content-type: application/json' \
    -d "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"consensus-status\",\"params\":{\"partition\":\"$part\",\"nodeID\":\"$nid\"}}" \
    | grep -oE '"(syntheticHeals|anchorHeals)":[0-9]+' || true)
  [ -n "$h" ] && { echo "  $part: $h"; heals=1; }
done
[ -n "$heals" ] || echo "(no heals recorded)"

if [ "$rc" -eq 0 ] && [ -n "$heals" ]; then
  echo; echo "RESULT: PASS (stream wedged and healed via receiver-pull)"
  rc=0
elif [ "$rc" -eq 0 ]; then
  echo; echo "RESULT: INCONCLUSIVE (deposits delivered but no heal was recorded)"
  rc=1
else
  echo; echo "RESULT: FAIL (driver exit $rc)"
fi
exit "$rc"
