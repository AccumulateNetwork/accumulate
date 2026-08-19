#!/bin/bash
# Launch a soak on a remote host WITH the dashboard on the operator's screen.
#
#   ./run-remote.sh <ssh-host> [DURATION] [TPS] [note...]
#   ./run-remote.sh thelio-fast 12h 1 "first 12h DI soak (#4100)"
#
# This script exists so that the monitor requirement does not depend on anyone
# remembering it. The sequence is fixed and each step gates the next:
#
#   1. start the run on the host (soak.sh itself refuses to apply load until
#      its monitor answers — that gate is server-side)
#   2. open the SSH tunnel from THIS machine
#   3. wait until the dashboard answers locally
#   4. open the operator's browser at it
#
# If the tunnel or dashboard cannot be reached from here, the remote run is
# KILLED and the network torn down. A run nobody can watch does not happen.
set -uo pipefail

host="${1:?usage: run-remote.sh <ssh-host> [DURATION] [TPS] [note...]}"
duration="${2:-12h}"
tps="${3:-1}"
shift $(( $# > 3 ? 3 : $# )); note="${*:-unnamed run}"

remote_dir='~/go/src/gitlab.com/AccumulateNetwork/accumulate/test/docker/soak'

echo "== starting soak on $host: DURATION=$duration TPS=$tps note='$note'"
ssh -o BatchMode=yes -o ConnectTimeout=10 "$host" \
  "bash -lc 'cd $remote_dir && setsid nohup env DURATION=$duration TPS=$tps ./soak.sh \"$note\" > /tmp/soak-launch.log 2>&1 < /dev/null &'" \
  || { echo "launch failed"; exit 1; }

echo "== opening tunnel"
pkill -f "ssh .*-L 8099:127.0.0.1:8099" 2>/dev/null || true
ssh -f -N -L 8099:127.0.0.1:8099 \
  -o BatchMode=yes -o ConnectTimeout=10 -o ServerAliveInterval=30 -o ExitOnForwardFailure=yes \
  "$host" || { echo "tunnel failed — killing the remote run"; ssh "$host" "bash -lc 'pkill -f soak.sh; pkill -f soakmon; cd $remote_dir/.. && docker compose down -v --remove-orphans'"; exit 1; }

echo "== waiting for the dashboard (network build + start can take a few minutes)"
for _ in $(seq 1 120); do
  curl -sf -m3 http://127.0.0.1:8099/data >/dev/null 2>&1 && break
  sleep 5
done
if ! curl -sf -m3 http://127.0.0.1:8099/data >/dev/null 2>&1; then
  echo "dashboard never became reachable from this machine — killing the remote run"
  ssh "$host" "bash -lc 'pkill -f soakmon; pkill -f \"tools/cmd/loadgen\"; cd $remote_dir/.. && docker compose down -v --remove-orphans'"
  exit 1
fi

echo "== dashboard is live; opening the browser"
if command -v xdg-open >/dev/null; then
  xdg-open http://127.0.0.1:8099 >/dev/null 2>&1 &
elif command -v open >/dev/null; then
  open http://127.0.0.1:8099
else
  echo "no browser opener found; dashboard is at http://127.0.0.1:8099"
fi
echo "== soak running under observation. Run dir on $host: $remote_dir/runs/latest"
