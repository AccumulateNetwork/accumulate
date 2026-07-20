#!/usr/bin/env bash
# Docker test for #4067: drive a MIXED transaction workload across partition
# boundaries and prove every expected synthetic type is produced and healed.
#
# This is the synth-heal harness with a broader workload: instead of only
# lite->lite token sends, the driver exercises ADI creation, ADI token
# transfers, cross-partition authority transactions (SignatureRequest /
# CreditPayment — the MessageForTransaction heal path), data writes and credit
# purchases, all targeting foreign partitions.
#
#   ./run-test.sh            # build image if needed, run, tear down
#   KEEP=1 ./run-test.sh     # leave the network up afterward for inspection
#
# It uses its own compose project (synthmix), container names (acc-mx-*), image
# tag (acc-synthmix:test) and host ports (26670+) so it can run alongside the
# synth-heal harness and its soak without colliding.
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
repo="$(cd "$here/../../.." && pwd)"
cd "$here"
compose="docker compose -p synthmix -f docker-compose.yml"

cleanup() { [ -n "${KEEP:-}" ] || $compose down -v --remove-orphans >/dev/null 2>&1 || true; }
trap cleanup EXIT

if ! docker image inspect acc-synthmix:test >/dev/null 2>&1; then
  echo "== Building image acc-synthmix:test =="
  docker build -t acc-synthmix:test -f "$repo/Dockerfile" "$repo"
fi

echo "== Starting network =="
$compose up -d

echo "== Waiting for the node JSON-RPC API =="
up=""
for _ in $(seq 1 60); do
  if curl -sf -X POST http://localhost:26670/v3 \
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

echo "== Driving the mixed workload (every 16th sequence is dropped) =="
rc=0
go run "$repo/test/docker/synth-heal/driver" \
  -endpoint http://localhost:26670 -workload mixed -count "${COUNT:-64}" -timeout "${TIMEOUT:-20m}" || rc=$?

echo
echo "== Evidence =="
echo "-- drop (wedge formed) --"
$compose logs 2>/dev/null | grep -iE "dropping (synthetic|sequenced) envelope" | head -5 || echo "(none)"
echo "-- heal counters via ConsensusStatus, every validator (receiver-pull fired) --"
heals=""
for c in $(docker ps --filter name=acc-mx-bvn --format '{{.Names}}'); do
  h=$(docker exec "$c" sh -c '
    nid=$(curl -s -X POST http://localhost:26660/v3 -H "content-type: application/json" -d "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"node-info\",\"params\":{}}" | grep -oE "\"peerID\":\"[^\"]+\"" | cut -d"\"" -f4)
    for part in Directory BVN1 BVN2 BVN3; do
      curl -s -X POST http://localhost:26660/v3 -H "content-type: application/json" -d "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"consensus-status\",\"params\":{\"partition\":\"$part\",\"nodeID\":\"$nid\"}}" | grep -oE "\"(syntheticHeals|anchorHeals)\":[0-9]+" | sed "s/^/$part /"
    done' 2>/dev/null || true)
  [ -n "$h" ] && { echo "  $c:"; echo "$h" | sed 's/^/    /'; heals=1; }
done
[ -n "$heals" ] || echo "(no heals recorded)"

if [ "$rc" -eq 0 ] && [ -n "$heals" ]; then
  echo; echo "RESULT: PASS (mixed workload delivered; healing recorded)"
  rc=0
elif [ "$rc" -eq 0 ]; then
  echo; echo "RESULT: INCONCLUSIVE (workload delivered but no heal was recorded)"
  rc=1
else
  echo; echo "RESULT: FAIL (driver exit $rc)"
fi
exit "$rc"
