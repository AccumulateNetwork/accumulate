#!/usr/bin/env bash
# Prove that what is deployed to server6 actually heals.
#
# Runs ONE dual-partition validator at v2-vandenberg on the v1.4.6 release
# image, wedges the DN <-> Cyclops synthetic stream by dropping a message, and
# checks the stream recovers on its own. See network.yml for why the sibling
# harness does not answer this question.
#
#   ./run-test.sh              # build/verify, run, tear down
#   KEEP=1 ./run-test.sh       # leave it up for inspection
#   MS_IMAGE=acc-release:v1.4.6 ./run-test.sh
#   MS_DROP='*:3' ./run-test.sh
set -uo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
repo="$(cd "$here/../../../.." && pwd)"
cd "$here"

image="${MS_IMAGE:-acc-release:v1.4.6}"
export MS_IMAGE="$image"
compose="docker compose -f docker-compose.yml"

cleanup() { [ -n "${KEEP:-}" ] || $compose down -v --remove-orphans >/dev/null 2>&1 || true; }
trap cleanup EXIT

docker image inspect "$image" >/dev/null 2>&1 || {
  echo "image $image not found — build it first:"
  echo "  make -B TAGS=production,mainnet GIT_DESCRIBE=v1.4.6 && docker build -t $image ."
  exit 2
}

echo "== Image under test =="
docker run --rm --entrypoint /bin/accumulated "$image" version 2>/dev/null | head -2
echo "-- healing present? --"
docker run --rm --entrypoint /bin/sh "$image" -c \
  'echo "  reconcile(#4073): $(strings /bin/accumulated | grep -c "Reconcile: pulled messages")";
   echo "  hold(#4070):      $(strings /bin/accumulated | grep -c "hold synthetic for anchor")";
   echo "  healing flags:    $(strings /bin/accumulated | grep -ci enable-synthetic-healing)"'

echo
echo "== Starting a mainnet-shaped network (1 node, DN+Cyclops, v2-vandenberg) =="
$compose up -d || { echo "compose up failed"; exit 1; }

echo "== Waiting for the DN JSON-RPC API =="
up=""
for _ in $(seq 1 80); do
  if curl -sf -X POST http://localhost:26660/v3 -H 'content-type: application/json' \
       -d '{"jsonrpc":"2.0","id":1,"method":"network-status","params":{"partition":"Directory"}}' >/dev/null 2>&1; then
    up=1; break
  fi
  sleep 3
done
[ -n "$up" ] || { echo "node API never came up"; $compose logs node | tail -40; exit 1; }

echo "-- confirming the network really is at Vandenberg --"
curl -s -X POST http://localhost:26660/v3 -H 'content-type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"network-status","params":{"partition":"Directory"}}' \
  | grep -oE '"executorVersion":"[^"]+"' | head -1
sleep 15

echo
echo "== Driving cross-partition sends (the first synthetic is dropped) =="
rc=0
go run "$repo/test/docker/synth-heal/mainnet-shape/driver" -endpoint http://localhost:26660 -count 5 -timeout 420s || rc=$?

echo
echo "== Evidence =="
echo "-- the wedge was actually created --"
$compose logs 2>/dev/null | grep -i "dropping synthetic envelope" | head -3 || echo "  (none — no drop, so nothing to heal)"
echo "-- reconcile activity (#4073) --"
$compose logs 2>/dev/null | grep -iE "Reconcile: pulled messages|reconcile" | tail -5 || echo "  (none)"
echo "-- heal counters on the node --"
nid=$(curl -s -X POST http://localhost:26660/v3 -H 'content-type: application/json' \
      -d '{"jsonrpc":"2.0","id":1,"method":"node-info","params":{}}' \
      | grep -oE '"peerID":"[^"]+"' | cut -d'"' -f4)
heals=""
for part in Directory Cyclops; do
  out=$(curl -s -X POST http://localhost:26660/v3 -H 'content-type: application/json' \
        -d "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"consensus-status\",\"params\":{\"partition\":\"$part\",\"nodeID\":\"$nid\"}}" \
        | grep -oE '"(syntheticHeals|anchorHeals)":[0-9]+')
  [ -n "$out" ] && { echo "  $part: $out"; heals=1; }
done
[ -n "$heals" ] || echo "  (counters absent — omitempty means absent == zero, so this alone is not proof of failure)"

echo
if [ "$rc" -eq 0 ] && [ -n "$heals" ]; then
  echo "RESULT: PASS — stream wedged at Vandenberg and healed itself on the deployed build"
elif [ "$rc" -eq 0 ]; then
  echo "RESULT: PASS (delivery) / INCONCLUSIVE (heal) — every send was delivered, but no"
  echo "        heal counter incremented. Either the drop never landed, or delivery"
  echo "        succeeded by a path other than healing. Check the drop line above."
  rc=1
else
  echo "RESULT: FAIL — driver exit $rc. Sends did not all deliver, so the wedge was not repaired."
fi
exit "$rc"
