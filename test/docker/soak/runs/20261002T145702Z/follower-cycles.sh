#!/bin/bash
# Repeated follower joins (Paul 2026-09-29: "run multiple tests"). Each cycle
# removes all three late followers, clears their databases, starts them at
# once, and times each partition to "it is ACTIVE". Then they run 10 minutes
# and any drop out of agreement is counted before the next cycle.
cd /home/paul/worktrees/issue-4205-lead/test/docker
R=soak/runs/20261002T145702Z
C="COMPOSE_PROJECT_NAME=disoak docker compose -f docker-compose.yml"
OUT=$R/follower-cycles.md
declare -A DIR=([bvn1-fol1]=bvn1-5 [bvn2-fol1]=bvn2-5 [bvn3-fol2]=bvn3-6)
CYCLES=${1:-3}
[ -f $OUT ] || printf "| cycle | follower | partition | started | ACTIVE after (s) | drops in 10 min |\n|---|---|---|---|---|---|\n" > $OUT
for cyc in $(seq ${2:-1} $((${2:-1} + CYCLES - 1))); do
  for s in "${!DIR[@]}"; do sg docker -c "docker rm -f acc-$s" >/dev/null 2>&1; done
  # One line per container: the monitor reads one container per line
  # (topology._LIFE), so three on one line showed only the first.
  for s in "${!DIR[@]}"; do echo "$(date -u +%FT%TZ) remove-follower acc-$s (follower-cycles.sh cycle $cyc)" >> $R/chaos.log; done
  clear=""; for d in "${DIR[@]}"; do clear+=" /root/.accumulate/$d/dnn/data/accumulate.db /root/.accumulate/$d/bvnn/data/accumulate.db /root/.accumulate/$d/consensus"; done
  sg docker -c "$C run --rm --no-deps --entrypoint sh bvn1-fol1 -c 'rm -rf$clear; for d in ${DIR[*]}; do f=/root/.accumulate/\$d/accumulate.toml; grep -q join-running-network \$f || sed -i \"/type = \\\"coreValidator\\\"/a\\\\  join-running-network = true\" \$f; done'" >/dev/null 2>&1
  sg docker -c "$C --profile late-follower create --no-recreate ${!DIR[*]}" >/dev/null 2>&1
  t0=$(date +%s); T=$(date -u +%FT%TZ)
  for s in "${!DIR[@]}"; do echo "$T add-follower acc-$s (follower-cycles.sh cycle $cyc; databases and consensus/ cleared; join-running-network)" >> $R/chaos.log; done
  sg docker -c "docker start acc-bvn1-fol1 acc-bvn2-fol1 acc-bvn3-fol2" >/dev/null 2>&1
  declare -A done_at=()
  while :; do
    now=$(date +%s); all=1
    for s in "${!DIR[@]}"; do
      for p in dn.acme bvn; do
        k="$s/$p"; [ -n "${done_at[$k]}" ] && continue
        if sg docker -c "docker logs acc-$s 2>&1" | sed 's/\x1b\[[0-9;]*m//g' | grep "it is ACTIVE" | grep -q "partition=acc://$p"; then done_at[$k]=$((now - t0)); else all=0; fi
      done
    done
    [ $all = 1 ] && break
    [ $((now - t0)) -gt 1200 ] && break
    sleep 5
  done
  sleep 600
  for s in "${!DIR[@]}"; do
    for p in dn.acme bvn; do
      drops=$(sg docker -c "docker logs acc-$s 2>&1" | sed 's/\x1b\[[0-9;]*m//g' | grep "not executing in agreement" | grep -c "partition=acc://$p")
      echo "| $cyc | $s | $p | $T | ${done_at[$s/$p]:-NOT ACTIVE in 1200} | $drops |" >> $OUT
    done
  done
  unset done_at
done
echo "done $(date -u +%FT%TZ)" >> $OUT
