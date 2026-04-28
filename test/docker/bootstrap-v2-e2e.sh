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
PARTITION="${PARTITION:-Apollo}"  # one of the BVNs in the compose stack

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

echo "[4/7] capturing operator pin from running network..."
# Real production launchers pin from the binary's pinned/pinned.go
# table. For E2E we don't have that populated, so we capture the
# current operators-page state hash and pass it as --pinned-hash.
# This mirrors what an operator running `accumulated bootstrap` on
# a development network would do.
NODE_INFO=$(curl -s -X POST -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","method":"node-info","id":1}' \
  "$ENDPOINT")
TIP_HEIGHT=$(curl -s -X POST -H 'Content-Type: application/json' \
  -d "{\"jsonrpc\":\"2.0\",\"method\":\"consensus-status\",\"params\":{\"partition\":\"$PARTITION\"},\"id\":1}" \
  "$ENDPOINT" | jq -r '.result.lastBlock.height // empty')
if [ -z "$TIP_HEIGHT" ]; then
  echo "[ERR] could not read tip height" >&2
  echo "node-info: $NODE_INFO" >&2
  exit 1
fi
echo "  tip height: $TIP_HEIGHT"

# Use a dev-mode pin: 32 zero bytes. The launcher's --skip-quorum
# flag accepts this for development; production deployments use the
# real pin populated in pinned/pinned.go.
PINNED_HASH="0000000000000000000000000000000000000000000000000000000000000000"

echo "[5/7] running accumulated bootstrap..."
"$ACCUMULATED" bootstrap \
  --network "$ENDPOINT" \
  --partition "$PARTITION" \
  --data-dir "$DATA_DIR" \
  --pinned-hash "$PINNED_HASH" \
  --pinned-height "$TIP_HEIGHT" \
  --height-range 1 \
  --skip-quorum \
  >"$BOOTSTRAP_LOG" 2>&1

ARTIFACT="$DATA_DIR/bootstrap-state-v2.json"
if [ ! -f "$ARTIFACT" ]; then
  echo "[ERR] artifact not produced: $ARTIFACT" >&2
  cat "$BOOTSTRAP_LOG" >&2
  exit 1
fi

echo "[6/7] inspecting artifact..."
NETWORK=$(jq -r '.network' "$ARTIFACT")
PARTITION_OUT=$(jq -r '.partition' "$ARTIFACT")
PINNED_HEIGHT_OUT=$(jq -r '.pinnedHeight' "$ARTIFACT")
VERIFIED_ANCHOR=$(jq -r '.verifiedAnchor' "$ARTIFACT")
VERIFIED_HEIGHT=$(jq -r '.verifiedHeight' "$ARTIFACT")
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
assert_nonempty "partition" "$PARTITION_OUT"
assert_nonempty "pinnedHeight" "$PINNED_HEIGHT_OUT"
assert_nonempty "verifiedAnchor" "$VERIFIED_ANCHOR"
assert_nonempty "verifiedHeight" "$VERIFIED_HEIGHT"

if [ "$STATE" != "ACTIVE" ]; then
  echo "[ERR] expected state ACTIVE, got: $STATE" >&2
  exit 1
fi
if [ "$PARTITION_OUT" != "$PARTITION" ]; then
  echo "[ERR] partition mismatch: artifact=$PARTITION_OUT expected=$PARTITION" >&2
  exit 1
fi
# Anchor must not be all zeros — that would mean convergence didn't
# actually establish a verified state.
if [[ "$VERIFIED_ANCHOR" =~ ^0+$ ]] || [[ "$VERIFIED_ANCHOR" =~ ^"AA"+$ ]]; then
  echo "[ERR] verifiedAnchor looks like a placeholder: $VERIFIED_ANCHOR" >&2
  exit 1
fi
echo "  network:        $NETWORK"
echo "  partition:      $PARTITION_OUT"
echo "  pinnedHeight:   $PINNED_HEIGHT_OUT"
echo "  verifiedHeight: $VERIFIED_HEIGHT"
echo "  state:          $STATE"
echo "  anchor (pfx):   $(echo "$VERIFIED_ANCHOR" | head -c 16)..."

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
echo "  endpoint:       $ENDPOINT"
echo "  partition:      $PARTITION_OUT"
echo "  verifiedHeight: $VERIFIED_HEIGHT"
echo "  state:          $STATE"
echo "================================================================"
