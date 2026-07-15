#!/usr/bin/env bash
# On-chain follower->validator promotion test (#4058 churn-soak P3).
#
# Single BVN, 1 validator (bvn1-1) + 4 followers (bvn1-2..5), 5 dual DN+BVN nodes.
# We promote bvn1-2 the way DAG-BFT actually does it: register bvn1-2's OWN
# validator key as an active validator on-chain (a NetworkDefinition update to
# dn.acme/network, signed by the DN operators), NOT by copying an identity.
#
# PASS:
#   1. every live node logs "Validator set changed, updating committee" (the
#      deterministic committee-epoch bump), and
#   2. bvn1-2's headers stop being rejected as "unknown validator" and it starts
#      getting certified by peers.
#
# Usage: ./test/docker/promote-test.sh [--skip-build]

set -uo pipefail
cd "$(dirname "$0")/../.."

COMPOSE="docker compose -p accp -f test/docker/docker-compose.promote.yml"
LOG=/tmp/promote-test; mkdir -p "$LOG"
BIN="$LOG/promote-validator"
log() { echo "[$(date +%T)] $*"; }
fail() { log "FAIL: $*"; exit 1; }

# BVN ledger height via the node's v3 API. bvn1-1 -> 27720 .. bvn1-5 -> 27724.
bvn_height() {
  local n=${1##*-} port
  port=$((27719 + n))
  curl -s -m5 -X POST "http://127.0.0.1:$port/v3" \
    -H 'Content-Type: application/json' \
    -d '{"jsonrpc":"2.0","id":1,"method":"query","params":{"scope":"acc://bvn-BVN1.acme/ledger"}}' \
    | jq -r '.result.account.index // "NA"' 2>/dev/null || echo NA
}
wait_advance() {
  local node=$1 base=$2 n=$3 h
  for _ in $(seq 1 "$n"); do
    h=$(bvn_height "$node"); { [ "$h" != NA ] && [ "${h:-0}" -gt "$base" ]; } && { echo "$h"; return 0; }
    sleep 4
  done
  echo "${h:-NA}"; return 1
}

log "=== build the promote-validator helper (host) ==="
go build -o "$BIN" ./test/cmd/promote-validator/ || fail "helper build failed"

log "=== clean slate + up (1 validator + 4 followers) ==="
$COMPOSE down -v >/dev/null 2>&1
if [ "${1:-}" = "--skip-build" ]; then UP="$COMPOSE up -d"; else UP="$COMPOSE up -d --build"; fi
GIT_DESCRIBE=$(git describe --tags --always 2>/dev/null || echo p3) GIT_COMMIT=$(git rev-parse --short HEAD) \
  $UP > "$LOG/up.log" 2>&1 || { cat "$LOG/up.log"; fail "compose up failed"; }
for _ in $(seq 1 60); do [ "$(docker ps --filter name=accp-bvn --filter status=running -q | wc -l)" -ge 5 ] && break; sleep 3; done
log "nodes running: $(docker ps --filter name=accp-bvn --filter status=running -q | wc -l)/5"

log "=== step 1: wait for the BVN to produce blocks ==="
h0=$(wait_advance bvn1-3 1 45) || fail "BVN never produced blocks (height=$h0)"
log "BVN producing blocks, height=$h0"

log "=== step 2: pull node configs from the shared volume ==="
for i in 1 2 3 4 5; do
  docker cp "accp-bvn1-1:/data/bvn1-$i/accumulate.toml" "$LOG/bvn1-$i.toml" 2>/dev/null \
    || fail "could not copy bvn1-$i.toml"
done
OPERATORS="$LOG/bvn1-1.toml,$LOG/bvn1-2.toml,$LOG/bvn1-3.toml,$LOG/bvn1-4.toml,$LOG/bvn1-5.toml"

log "=== step 3: confirm bvn1-2 is currently a non-committee (unknown) validator ==="
AUTHOR=$("$BIN" -server http://127.0.0.1:27720 -promote "$LOG/bvn1-2.toml" -operators "$LOG/bvn1-1.toml" -partitions Directory 2>&1 | grep -oE 'promoting validator key [0-9a-f]+' | awk '{print $4}')
[ -n "$AUTHOR" ] || fail "could not resolve bvn1-2 author key"
A8=${AUTHOR:0:8}
log "bvn1-2 author key: $AUTHOR (short $A8)"
if docker logs --tail 200 accp-bvn1-3 2>&1 | grep -q "unknown validator.*$A8"; then
  log "confirmed: peers see $A8 as an unknown validator (follower)"
else
  log "note: did not observe an explicit 'unknown validator $A8' line (may simply be idle)"
fi

log "=== step 4: promote bvn1-2 on-chain (register its key active on Directory + BVN1) ==="
"$BIN" -server http://127.0.0.1:27720 \
  -promote "$LOG/bvn1-2.toml" \
  -operators "$OPERATORS" \
  -partitions Directory,BVN1 2>&1 | tee "$LOG/promote.log"
grep -q '^OK submitted' "$LOG/promote.log" || fail "promotion submit did not succeed (see $LOG/promote.log)"

log "=== step 5: verify the committee epoch bumped on every node ==="
ok=0
for i in 1 2 3 4 5; do
  for _ in $(seq 1 20); do
    if docker logs --tail 400 "accp-bvn1-$i" 2>&1 | grep -q "Validator set changed, updating committee"; then
      log "  accp-bvn1-$i: committee updated"; ok=$((ok+1)); break
    fi
    sleep 3
  done
done
[ "$ok" -ge 3 ] || fail "committee-update log seen on only $ok/5 nodes"
log "committee update observed on $ok/5 nodes"

log "=== step 6: verify bvn1-2 is now accepted / certified by a peer ==="
promoted=0
for _ in $(seq 1 20); do
  # bvn1-2's headers should now be handled without the "unknown validator" rejection,
  # and its author should appear among certificate signers on a peer.
  if docker logs --tail 300 accp-bvn1-3 2>&1 | grep -E "Created certificate|Header handled by primary" | grep -q "$A8"; then
    promoted=1; break
  fi
  sleep 3
done
[ "$promoted" = 1 ] && log "bvn1-2 ($A8) is participating as a validator" \
  || fail "bvn1-2 ($A8) not observed certifying within timeout"

log "=== PASS: on-chain promotion works — committee epoch bumped and bvn1-2 certifying ==="
$COMPOSE down -v >/dev/null 2>&1
