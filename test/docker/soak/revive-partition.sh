#!/bin/bash
# Revive a partition that lost a quorum of its validators' states, by state
# copy -- the stopgap the spec names (executor.md, "A partition that loses a
# quorum of its validators' states"; PLAN R4-C; #4512):
#
#   ./revive-partition.sh <run-dir> <partition> <source-val> <lost-val>...
#   e.g. ./revive-partition.sh soak/runs/X bvn2 1 3 4
#
# Every validator of the partition is stopped first (a live copy is no copy),
# the source's partition store and consensus checkpoints are copied over the
# lost nodes' inside the config volume, and all are started together so each
# takes the whole-restart exit on the copied block and signs from there. The
# Directory nodes of the same containers are untouched. Recorded in chaos.log.
set -u
R=${1:?run dir}; P=${2:?partition, e.g. bvn2}; SRC=${3:?source validator number}; shift 3
LOST=("$@"); [ ${#LOST[@]} -ge 1 ] || { echo "name at least one lost validator" >&2; exit 2; }
cd "$(dirname "$0")/.." || exit 1
C="COMPOSE_PROJECT_NAME=${COMPOSE_PROJECT_NAME:-disoak} docker compose -f docker-compose.yml"
vals=$(sg docker -c "docker ps -a --format '{{.Names}}'" | grep "^acc-$P-val" | sort | tr '\n' ' ')
echo "$(date -u +%FT%TZ) REVIVE $P by state copy: stop $vals; copy $P-$SRC's $P store and consensus/$P to ${LOST[*]/#/$P-}; start all together (#4512, R4-C)" >> "$R/chaos.log"
sg docker -c "docker stop -t 30 $vals" >/dev/null || exit 1
cmd=""
for n in "${LOST[@]}"; do
  cmd+="rm -rf /root/.accumulate/$P-$n/bvnn/data/accumulate.db /root/.accumulate/$P-$n/consensus/$P && "
  cmd+="cp -a /root/.accumulate/$P-$SRC/bvnn/data/accumulate.db /root/.accumulate/$P-$n/bvnn/data/accumulate.db && "
  cmd+="cp -a /root/.accumulate/$P-$SRC/consensus/$P /root/.accumulate/$P-$n/consensus/$P && "
done
cmd+="echo copied"
sg docker -c "$C run --rm --no-deps --entrypoint sh $P-val$SRC -c '$cmd'" 2>&1 | tail -1
sg docker -c "docker start $vals" >/dev/null
echo "$(date -u +%FT%TZ) revive complete: $vals started" >> "$R/chaos.log"
