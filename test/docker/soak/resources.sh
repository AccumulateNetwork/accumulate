#!/usr/bin/env bash
# Live CPU / memory view of the soak network for a terminal (tmux pane, ssh).
#
#   ./resources.sh            # refreshes every 2 s until Ctrl-C
#   ./resources.sh 5          # refresh interval in seconds
#   ./resources.sh --once     # one frame, for scripts and logs
#
# Host first (load, memory), then every acc-* container from `docker stats`:
# CPU% is of ONE core (a 12-node network on a 32-core box can legitimately
# read 1200% in total); MEM is the cgroup usage against the compose mem_limit
# (soak.conf ACC_MEM_LIMIT, 2048m by default), and GOMEMLIMIT sits below that,
# so a node near 100% of LIMIT is a node the Go runtime is already fighting.
# NET and BLOCK are cumulative since the container started.
interval=${1:-2}
frame() {
  printf '%s   host load %s\n' "$(date -u +%FT%TZ)" "$(cut -d' ' -f1-3 /proc/loadavg)"
  free -h | awk 'NR==1{printf "%-8s %8s %8s %8s %10s\n","",$1,$2,$3,$6} NR==2{printf "%-8s %8s %8s %8s %10s\n","host mem",$2,$3,$4,$7}'
  echo
  names=$(docker ps --format '{{.Names}}' | grep -E '^acc-' | sort)
  if [ -z "$names" ]; then echo "no acc-* containers running"; return; fi
  # shellcheck disable=SC2086
  docker stats --no-stream --format 'table {{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}\t{{.MemPerc}}\t{{.NetIO}}\t{{.BlockIO}}' $names \
    | awk 'NR==1{print; next} {print; cpu+=$2; mem+=$6; n++} END{printf "%-15s %6.0f%%    (sum of CPU%% over %d containers; MEM%% avg %.1f%%)\n","total",cpu,n,mem/n}'
}
if [ "$1" = "--once" ]; then frame; exit 0; fi
while true; do
  out=$(frame 2>&1)
  clear; printf '%s\n\n(refresh %ss, Ctrl-C to stop)\n' "$out" "$interval"
  sleep "$interval"
done
