#!/bin/bash
# The whole-network restart as a harness step (PLAN R6; Paul, 2026-10-02):
#
#   ./restart-network.sh <run-dir> graceful   # the normal form: docker stop (drain), then start all together
#   ./restart-network.sh <run-dir> kill       # the chaos case: docker rm -f (SIGKILL), recreate all together
#
# Every validator, the comparison follower and the late followers go down
# before any comes back, so each takes the whole-restart exit. The step is
# recorded in the run's chaos.log with the heights before it, and the
# containers' image ids are unchanged: this restarts what was running, it
# never rebuilds. A kill is what tore two stores on 20261002T145702Z
# (#4507, BlockchainDB#99); the torn-store check (join.CheckStore) is what
# makes the kill case a test rather than a gamble.
set -u
R=${1:?run dir}; MODE=${2:-graceful}
cd "$(dirname "$0")/.." || exit 1
C="COMPOSE_PROJECT_NAME=${COMPOSE_PROJECT_NAME:-disoak} docker compose -f docker-compose.yml"
names=$(sg docker -c "docker ps --format '{{.Names}}'" | grep "^acc-" | grep -v "^acc-bootstrap$" | sort | tr '\n' ' ')
svcs=$(echo "$names" | sed 's/acc-//g')
H=$(curl -s --max-time 5 http://127.0.0.1:8099/data 2>/dev/null | python3 -c "import json,sys; print(json.load(sys.stdin).get('heights'))" 2>/dev/null)
case $MODE in
  graceful)
    echo "$(date -u +%FT%TZ) DELIBERATE WHOLE-NETWORK RESTART (graceful): docker stop of every node, then start all together: $names; heights before: $H" >> "$R/chaos.log"
    sg docker -c "docker stop -t 30 $names" >/dev/null && sg docker -c "docker start $names" >/dev/null ;;
  kill)
    echo "$(date -u +%FT%TZ) DELIBERATE WHOLE-NETWORK RESTART (KILL, chaos case): docker rm -f of every node, recreated at once on their images: $names; heights before: $H" >> "$R/chaos.log"
    sg docker -c "docker rm -f $names" >/dev/null \
      && sg docker -c "$C --profile late-follower create --no-recreate $svcs" >/dev/null 2>&1 \
      && sg docker -c "docker start $names" >/dev/null ;;
  *) echo "mode: graceful | kill" >&2; exit 2 ;;
esac
echo "$(date -u +%FT%TZ) restart complete ($MODE): $(echo $names | wc -w) containers started" >> "$R/chaos.log"
