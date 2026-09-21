#!/bin/bash
# Bring up the DAG-BFT network WITH the monitor, as one action.
#
# There is no supported way to run this network without the monitor. That is
# a standing requirement, stated in synth-heal/DEPLOY-REMOTE.md and repeated
# by its owner: an unobserved network burning CPU is not a test, it is just
# heat — and a failure during an unmonitored window is invisible, which is
# worse than no run at all. The 2026-08 DI diagnosis sessions ran the network
# unmonitored roughly five times and every one of those windows is a hole in
# the record. Use this script, or soak/soak.sh, never a bare `compose up`.
set -euo pipefail
here="$(cd "$(dirname "$0")" && pwd)"

# This network shares a directory name ("docker") with the ASP mainnet fleet at
# core/staking/deploy/docker, so Compose derives the SAME default project name
# for both. The `down --remove-orphans` teardown below would then treat running
# asp-v00* mainnet containers as orphans and delete them. Pin the project so
# teardown can only ever reach this network.
export COMPOSE_PROJECT_NAME="${COMPOSE_PROJECT_NAME:-disoak}"
compose="docker compose -f $here/docker-compose.yml"

# Build BEFORE up. `up -d` silently reuses a stale image, and every conclusion
# drawn from such a run is about the wrong build (#4103).
$compose build
$compose up -d

# How many containers to expect comes from the topology, not a literal. The
# hardcoded 13 was written for the 3-BVN network; against the 2-BVN one it can
# never be reached, so this loop span forever on a network that was already up.
#
# The nodes `up` STARTS, plus the bootstrap — not every node declared. The
# added follower (#4364) is declared and sits behind a compose profile that the
# `up -d` above does not activate; counting it made this one more than can ever
# be healthy, so the loop never ended and the monitor below never started.
want=$(python3 -c 'import sys,json; sys.path.insert(0,"'"$here"'"); import topology; print(topology.started_count() + 1)')
echo "waiting for health ($want containers)..."
until [ "$(docker ps --filter name=acc- --filter health=healthy -q | wc -l)" -ge "$want" ]; do sleep 5; done
docker ps --filter name=acc- --format '{{.Names}} {{.Status}}'

# The monitor is not optional and not a separate step.
mkdir -p "$here/soak/runs/adhoc-$(date -u +%Y%m%dT%H%M%SZ)"
RUN_DIR="$here/soak/runs/adhoc-$(date -u +%Y%m%dT%H%M%SZ)" nohup "$here/soak/soakmon.py" \
  > "$here/soak/soakmon-adhoc.log" 2>&1 &
for _ in $(seq 1 20); do
  curl -sf -m3 http://127.0.0.1:8099/data >/dev/null 2>&1 && break
  sleep 3
done
if curl -sf -m3 http://127.0.0.1:8099/data >/dev/null 2>&1; then
  echo "monitor: http://127.0.0.1:8099 (tunnel: ssh -N -L 8099:127.0.0.1:8099 <host>)"
else
  echo "MONITOR DID NOT COME UP — tearing the network back down rather than running blind" >&2
  $compose down -v --remove-orphans
  exit 1
fi
