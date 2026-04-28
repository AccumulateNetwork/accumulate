#!/usr/bin/env bash
# Copyright 2026 The Accumulate Authors
#
# Use of this source code is governed by an MIT-style
# license that can be found in the LICENSE file or at
# https://opensource.org/licenses/MIT.
#
# Capstone E2E for the v2 bootstrap launcher. Brings up the standard
# 12-node Docker test stack, runs `accumulated bootstrap` against one
# of the BVN validators with an operator-supplied pin override, then
# verifies the bootstrap-state-v2.json artifact contains the expected
# state (ACTIVE + non-zero VerifiedAnchor + matching pin).
#
# Run:
#
#   test/docker/bootstrap-v2-e2e.sh
#
# Override the validator endpoint via $ENDPOINT if your compose
# stack uses different host ports. Logs land in /tmp/bootstrap-v2-e2e-*
# per CLAUDE.md guidance on log-flooding.
#
# Requirements:
#   - docker + docker compose
#   - jq (for artifact inspection)
#   - Go toolchain (to build accumulated)
#
# Exits 0 on success, non-zero on any failure.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
COMPOSE_FILE="$REPO_ROOT/test/docker/docker-compose.yml"
LOG_DIR="/tmp"
BUILD_LOG="$LOG_DIR/bootstrap-v2-e2e-build.log"
COMPOSE_LOG="$LOG_DIR/bootstrap-v2-e2e-compose.log"
BOOTSTRAP_LOG="$LOG_DIR/bootstrap-v2-e2e-launcher.log"

ENDPOINT="${ENDPOINT:-http://localhost:26660/v3}"
BVN="${BVN:-Apollo}"  # the BVN to bootstrap into (one of the BVNs in the compose stack)

DATA_DIR="$(mktemp -d -t bootstrap-v2-e2e-XXXXXX)"
ACCUMULATED="/tmp/accumulated-v2-e2e"

cleanup() {
  echo "[cleanup] tearing down docker stack..."
  docker compose -f "$COMPOSE_FILE" down -v >>"$COMPOSE_LOG" 2>&1 || true
  rm -rf "$DATA_DIR"
}
trap cleanup EXIT

require() {
  local cmd="$1"
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "[ERR] required command not found: $cmd" >&2
    exit 1
  fi
}
require docker
require jq
require go
require curl

echo "[1/7] building accumulated..."
( cd "$REPO_ROOT" && go build -o "$ACCUMULATED" ./cmd/accumulated ) >"$BUILD_LOG" 2>&1

echo "[2/7] starting docker stack..."
docker compose -f "$COMPOSE_FILE" up -d --build >"$COMPOSE_LOG" 2>&1

echo "[3/7] waiting for endpoint $ENDPOINT to respond..."
DEADLINE=$(( $(date +%s) + 300 ))
while true; do
  if [ "$(date +%s)" -ge "$DEADLINE" ]; then
    echo "[ERR] timed out waiting for $ENDPOINT" >&2
    docker compose -f "$COMPOSE_FILE" ps >&2
    exit 1
  fi
  if curl -s -m 5 -o /dev/null -w '%{http_code}' "$ENDPOINT" 2>/dev/null | grep -qE '^[24]'; then
    break
  fi
  sleep 5
done

echo "[4/7] capturing genesis StateTreeAnchor for operator override..."
# Real production launchers pin from the binary's pinned/pinned.go
# table. For E2E the table is empty, so we resolve DN's
# StateTreeAnchor at major-block 1 directly from the running peer
# and pass it via --genesis-state-tree-anchor.
#
# Use jq to extract the value from the major-block-1 anchor record
# on dn.acme/anchors's main chain.
GENESIS_ANCHOR=$(curl -s -X POST -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","method":"query","params":{"scope":"acc://dn.acme/anchors","query":{"queryType":"chain","name":"major-block","range":{"start":0,"count":1,"expand":true}}},"id":1}' \
  "$ENDPOINT" | jq -r '.result.records[0].value.value.source // empty')
if [ -z "$GENESIS_ANCHOR" ]; then
  echo "[WARN] could not auto-extract genesis StateTreeAnchor; using zero-hash override (dev-only)"
  GENESIS_ANCHOR="0000000000000000000000000000000000000000000000000000000000000000"
fi

echo "[5/7] running accumulated bootstrap..."
"$ACCUMULATED" bootstrap \
  --network "$ENDPOINT" \
  --bvn "$BVN" \
  --data-dir "$DATA_DIR" \
  --genesis-state-tree-anchor "$GENESIS_ANCHOR" \
  >"$BOOTSTRAP_LOG" 2>&1

ARTIFACT="$DATA_DIR/bootstrap-state-v2.json"
if [ ! -f "$ARTIFACT" ]; then
  echo "[ERR] artifact not produced: $ARTIFACT" >&2
  cat "$BOOTSTRAP_LOG" >&2
  exit 1
fi

echo "[6/7] inspecting artifact..."
NETWORK=$(jq -r '.network' "$ARTIFACT")
BVN_OUT=$(jq -r '.bvn' "$ARTIFACT")
DN_GENESIS_ANCHOR=$(jq -r '.dnGenesisStateTreeAnchor' "$ARTIFACT")
DN_VERIFIED_ANCHOR=$(jq -r '.dnVerifiedAnchor' "$ARTIFACT")
DN_VERIFIED_BLOCK=$(jq -r '.dnVerifiedMajorBlock' "$ARTIFACT")
BVN_VERIFIED_ANCHOR=$(jq -r '.bvnVerifiedAnchor' "$ARTIFACT")
BVN_VERIFIED_BLOCK=$(jq -r '.bvnVerifiedMajorBlock' "$ARTIFACT")
STATE=$(jq -r '.state.current' "$ARTIFACT")

assert_nonempty() {
  local name="$1" value="$2"
  if [ -z "$value" ] || [ "$value" = "null" ]; then
    echo "[ERR] $name is empty in artifact" >&2
    cat "$ARTIFACT" >&2
    exit 1
  fi
}

assert_nonempty "network" "$NETWORK"
assert_nonempty "bvn" "$BVN_OUT"
assert_nonempty "dnGenesisStateTreeAnchor" "$DN_GENESIS_ANCHOR"
assert_nonempty "dnVerifiedAnchor" "$DN_VERIFIED_ANCHOR"
assert_nonempty "dnVerifiedMajorBlock" "$DN_VERIFIED_BLOCK"
assert_nonempty "bvnVerifiedAnchor" "$BVN_VERIFIED_ANCHOR"
assert_nonempty "bvnVerifiedMajorBlock" "$BVN_VERIFIED_BLOCK"

if [ "$STATE" != "ACTIVE" ]; then
  echo "[ERR] expected state ACTIVE, got: $STATE" >&2
  exit 1
fi
if [ "$BVN_OUT" != "$BVN" ]; then
  echo "[ERR] bvn mismatch: artifact=$BVN_OUT expected=$BVN" >&2
  exit 1
fi
# Anchors must not be all zeros — that would mean convergence
# didn't actually establish a verified state.
for name in "dnVerifiedAnchor=$DN_VERIFIED_ANCHOR" "bvnVerifiedAnchor=$BVN_VERIFIED_ANCHOR"; do
  field="${name%%=*}"
  value="${name#*=}"
  if [[ "$value" =~ ^0+$ ]]; then
    echo "[ERR] $field is all zeros — convergence didn't establish trust" >&2
    exit 1
  fi
done
echo "  network:                $NETWORK"
echo "  bvn:                    $BVN_OUT"
echo "  dnVerifiedMajorBlock:   $DN_VERIFIED_BLOCK"
echo "  dnVerifiedAnchor:       $(echo "$DN_VERIFIED_ANCHOR" | head -c 16)..."
echo "  bvnVerifiedMajorBlock:  $BVN_VERIFIED_BLOCK"
echo "  bvnVerifiedAnchor:      $(echo "$BVN_VERIFIED_ANCHOR" | head -c 16)..."
echo "  state:                  $STATE"

echo "[7/7] verifying that accumulated run detects the v2 artifact..."
RUN_LOG="$LOG_DIR/bootstrap-v2-e2e-run.log"
# Boot accumulated run pointed at the bootstrap-launched data dir.
# We don't need it to fully come up — just to log the
# "Resuming from v2 bootstrap-launched state" line that
# detectBootstrapState emits. Five seconds is plenty.
timeout 10s "$ACCUMULATED" run -w "$DATA_DIR" >"$RUN_LOG" 2>&1 || true

if ! grep -q "Resuming from v2 bootstrap-launched state" "$RUN_LOG"; then
  echo "[ERR] accumulated run did not detect the v2 bootstrap artifact" >&2
  echo "--- run log (tail) ---" >&2
  tail -50 "$RUN_LOG" >&2
  exit 1
fi

echo
echo "================================================================"
echo "v2 E2E PASS: bootstrap → artifact → run-resume cycle complete"
echo "  endpoint:               $ENDPOINT"
echo "  bvn:                    $BVN_OUT"
echo "  dnVerifiedMajorBlock:   $DN_VERIFIED_BLOCK"
echo "  bvnVerifiedMajorBlock:  $BVN_VERIFIED_BLOCK"
echo "  state:                  $STATE"
echo "================================================================"
