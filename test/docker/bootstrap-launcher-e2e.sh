#!/usr/bin/env bash
# Copyright 2026 The Accumulate Authors
#
# Use of this source code is governed by an MIT-style
# license that can be found in the LICENSE file or at
# https://opensource.org/licenses/MIT.
#
# Capstone E2E test for the minimum-data bootstrap launcher (issue
# #3976, parent #3953). Brings up the standard Docker test network,
# runs `accumulated bootstrap` against one of the validators, and
# verifies the resulting bootstrap-state.json artifact. Optionally
# starts a follower container that resumes from the artifact.
#
# Manual to run:
#
#   test/docker/bootstrap-launcher-e2e.sh
#
# CI-friendly because it exits non-zero on any failure. Requires:
#   - docker + docker compose
#   - jq (for artifact inspection)
#   - Go toolchain (to build accumulated)
#
# Output is logged to /tmp/bootstrap-e2e-*.log to stay clear of the
# AI agent context per CLAUDE.md guidance.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
COMPOSE_FILE="$REPO_ROOT/test/docker/docker-compose.yml"
DATA_DIR="$(mktemp -d -t bootstrap-e2e-XXXXXX)"
LOG_DIR="/tmp"
BOOTSTRAP_LOG="$LOG_DIR/bootstrap-e2e-launcher.log"
COMPOSE_LOG="$LOG_DIR/bootstrap-e2e-compose.log"

cleanup() {
  echo "[cleanup] Tearing down docker stack..."
  docker compose -f "$COMPOSE_FILE" down -v >>"$COMPOSE_LOG" 2>&1 || true
  rm -rf "$DATA_DIR"
}
trap cleanup EXIT

echo "[1/6] Building accumulated binary..."
( cd "$REPO_ROOT" && go build -o /tmp/accumulated-e2e ./cmd/accumulated ) >"$LOG_DIR/bootstrap-e2e-build.log" 2>&1

echo "[2/6] Starting docker stack..."
docker compose -f "$COMPOSE_FILE" up -d --build >"$COMPOSE_LOG" 2>&1

echo "[3/6] Waiting for network to produce blocks..."
DEADLINE=$(( $(date +%s) + 300 ))
while true; do
  if [ "$(date +%s)" -ge "$DEADLINE" ]; then
    echo "[ERR] timed out waiting for network to produce blocks" >&2
    docker compose -f "$COMPOSE_FILE" ps >&2
    exit 1
  fi
  # Probe one of the validators. The compose stack publishes RPC ports
  # in the 26656-26800 range; adjust if your setup differs.
  if curl -s -m 5 http://localhost:26660/v3 >/dev/null 2>&1; then
    break
  fi
  sleep 5
done

echo "[4/6] Running bootstrap launcher against validator..."
/tmp/accumulated-e2e bootstrap \
  --network http://localhost:26660/v3 \
  --data-dir "$DATA_DIR" \
  --partition Directory \
  --skip-proof \
  >"$BOOTSTRAP_LOG" 2>&1

ARTIFACT="$DATA_DIR/bootstrap-state.json"
if [ ! -f "$ARTIFACT" ]; then
  echo "[ERR] artifact not produced: $ARTIFACT" >&2
  cat "$BOOTSTRAP_LOG" >&2
  exit 1
fi

echo "[5/6] Inspecting artifact..."
PIN_BLOCK=$(jq -r '.pinBlock.minorBlockIndex' "$ARTIFACT")
NETWORK=$(jq -r '.network' "$ARTIFACT")
STATE=$(jq -r '.state.current' "$ARTIFACT")
PARTITION=$(jq -r '.pinBlock.partition' "$ARTIFACT")

if [ -z "$PIN_BLOCK" ] || [ "$PIN_BLOCK" = "null" ] || [ "$PIN_BLOCK" -le 0 ]; then
  echo "[ERR] invalid pin block: $PIN_BLOCK" >&2
  exit 1
fi
if [ "$STATE" != "BOOTING" ]; then
  echo "[ERR] expected state BOOTING after bootstrap, got: $STATE" >&2
  exit 1
fi
if [ "$PARTITION" != "Directory" ]; then
  echo "[ERR] expected partition Directory, got: $PARTITION" >&2
  exit 1
fi

echo "[ok] artifact: network=$NETWORK partition=$PARTITION pinBlock=$PIN_BLOCK state=$STATE"

echo "[6/6] Verifying that accumulated run detects the artifact..."
# This mounts the data dir into a follower container; the run startup
# logs should mention "Resuming from bootstrap-launched state". We
# read the first 5 seconds of run output to confirm.
RUN_LOG="$LOG_DIR/bootstrap-e2e-run.log"
timeout 10s /tmp/accumulated-e2e run --work-dir "$DATA_DIR" >"$RUN_LOG" 2>&1 || true

if ! grep -q "Resuming from bootstrap-launched state" "$RUN_LOG"; then
  echo "[ERR] accumulated run did not detect the bootstrap artifact" >&2
  echo "--- run log ---" >&2
  tail -50 "$RUN_LOG" >&2
  exit 1
fi

echo
echo "================================================================"
echo "E2E PASS: bootstrap → artifact → run-resume cycle complete"
echo "  network:   $NETWORK"
echo "  partition: $PARTITION"
echo "  pin block: $PIN_BLOCK"
echo "  state:     $STATE"
echo "================================================================"
