#!/usr/bin/env bash
# Reproduce and diagnose the API-node OOM of #4085.
#
#   ./oom-debug.sh                  # 4h at 10 tps
#   DURATION=8h TPS=2 ./oom-debug.sh   # reproduce the original conditions
#
# The 24h soak lost bvn1 to the kernel OOM killer at T+7h58m with no heap
# profile surviving, so the whole point here is evidence: pull a heap profile
# from the API node on a fixed schedule and record per-container RSS, so the
# growth curve and the growing allocation site are both recoverable whether or
# not the run reaches a kill.
#
# Profiles are diffed against the first one, which is what identifies the leak:
#   go tool pprof -base heap-0001.pb.gz heap-0050.pb.gz
set -uo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
repo="$(cd "$here/../../.." && pwd)"
cd "$here"

duration="${DURATION:-4h}"
tps="${TPS:-10}"
heap_every="${HEAP_EVERY:-180}"     # seconds between heap profiles
api_port="${API_PORT:-27660}"
pprof_port="${PPROF_PORT:-6060}"
api="http://localhost:${api_port}"

stamp="$(date -u +%Y%m%dT%H%M%SZ)"
run="$here/soak/runs/oom-$stamp"
mkdir -p "$run/heap"

export DROP_SPEC=""                 # no fault injection; this is a memory study

if docker info >/dev/null 2>&1; then
  compose() { docker compose -f docker-compose.yml -f docker-compose.pprof.yml "$@"; }
  dock()    { docker "$@"; }
else
  compose() { sg docker -c "cd '$here' && docker compose -f docker-compose.yml -f docker-compose.pprof.yml $*"; }
  dock()    { sg docker -c "docker $*"; }
fi

log() { echo "[$(date -u +%H:%M:%S)] $*" | tee -a "$run/debug.log"; }

cat > "$run/manifest.txt" <<EOF
started_utc   $stamp
purpose       reproduce API-node OOM (#4085) with heap profiling
duration      $duration
tps           $tps
heap_every    ${heap_every}s
mem_limit     2g (unchanged - reproducing the same ceiling)
git_commit    $(git -C "$repo" rev-parse HEAD 2>/dev/null)
git_describe  $(git -C "$repo" describe --tags 2>/dev/null)
EOF
log "run dir: $run"
cat "$run/manifest.txt" | tee -a "$run/debug.log"

compose down -v --remove-orphans >/dev/null 2>&1
compose up -d >>"$run/debug.log" 2>&1

log "waiting for API"
for _ in $(seq 1 80); do
  curl -sf -X POST "$api/v3" -H 'content-type: application/json' \
    -d '{"jsonrpc":"2.0","id":1,"method":"network-status","params":{"partition":"Directory"}}' \
    >/dev/null 2>&1 && break
  sleep 3
done

log "waiting for pprof on :$pprof_port"
for _ in $(seq 1 40); do
  curl -sf "http://localhost:${pprof_port}/debug/pprof/" >/dev/null 2>&1 && break
  sleep 3
done
curl -sf "http://localhost:${pprof_port}/debug/pprof/" >/dev/null 2>&1 \
  && log "pprof is up" || log "WARNING: pprof not reachable - profiles will be missing"

log "starting loadgen: ${tps} tps for ${duration}"
go run "$repo/tools/cmd/loadgen" -endpoint "$api" -tps "$tps" -duration "$duration" \
  -bootstrap 0 -faucet-seed FAUCET -grace 5m -logtostderr \
  -stats-file "$run/loadgen-stats.json" \
  >"$run/loadgen.log" 2>&1 &
lg_pid=$!

echo "ts,seq,bvn1_mb,total_mb,goroutines,heap_alloc_mb,heap_sys_mb,alive" > "$run/mem.csv"

secs=$(python3 -c "
import re
d='$duration'; m=re.fullmatch(r'(\d+)([hms])',d)
print(int(m.group(1))*{'h':3600,'m':60,'s':1}[m.group(2)] if m else 14400)" 2>/dev/null)
case "${secs:-}" in ''|*[!0-9]*) secs=14400 ;; esac
deadline=$(( $(date +%s) + secs ))
log "profiling every ${heap_every}s until $(date -u -d "@$deadline" +%FT%TZ)"

seq_n=0
while [ "$(date +%s)" -lt "$deadline" ]; do
  sleep "$heap_every"
  seq_n=$(( seq_n + 1 ))
  tag=$(printf "%04d" "$seq_n")

  # Heap profile. Keep going if it fails: a failure here is itself the signal
  # that the node has died, and the preceding profiles are what matter.
  if curl -sf -m 30 "http://localhost:${pprof_port}/debug/pprof/heap" \
       -o "$run/heap/heap-$tag.pb.gz" 2>/dev/null; then
    alive=1
  else
    alive=0
  fi

  # Go runtime counters, cheap and readable without pprof
  gor=$(curl -sf -m 15 "http://localhost:${pprof_port}/debug/pprof/goroutine?debug=1" 2>/dev/null \
        | head -1 | grep -oE "[0-9]+" | head -1)
  read -r halloc hsys < <(curl -sf -m 15 "http://localhost:${pprof_port}/debug/pprof/heap?debug=1" 2>/dev/null \
        | awk '/^# HeapAlloc/ {a=$4} /^# HeapSys/ {s=$4} END {printf "%.0f %.0f", a/1048576, s/1048576}')

  bvn1=$(dock stats --no-stream --format "'{{.Name}} {{.MemUsage}}'" 2>/dev/null | tr -d "'" \
         | awk '/acc-bvn1 /{print $2}' | sed 's/MiB//;s/GiB/*1024/' | awk '{print ($0 ~ /\*/) ? $0 : $0}' \
         | python3 -c "
import sys
v=sys.stdin.read().strip()
print(int(eval(v)) if v else 0)" 2>/dev/null)
  total=$(dock stats --no-stream --format "'{{.MemUsage}}'" 2>/dev/null | tr -d "'" \
         | awk -F'/' '{print $1}' | sed 's/MiB//;s/GiB/*1024/' | awk '{s+=$1} END{printf "%.0f", s}')

  echo "$(date -u +%FT%TZ),$seq_n,${bvn1:-0},${total:-0},${gor:-0},${halloc:-0},${hsys:-0},$alive" >> "$run/mem.csv"
  log "seq=$tag bvn1=${bvn1:-?}MB total=${total:-?}MB goroutines=${gor:-?} heapAlloc=${halloc:-?}MB heapSys=${hsys:-?}MB alive=$alive"

  if [ "$alive" = "0" ]; then
    log "pprof unreachable - checking whether bvn1 died"
    if ! dock ps --format "'{{.Names}}'" 2>/dev/null | tr -d "'" | grep -qx "acc-bvn1"; then
      log "acc-bvn1 IS GONE. Kernel record:"
      journalctl -k --since "-10 min" 2>/dev/null \
        | grep -E "Killed process|Memory cgroup out of memory" | tail -5 | tee -a "$run/debug.log"
      log "last heap profile: $(ls -1 "$run/heap" | tail -1)"
      break
    fi
  fi
done

kill "$lg_pid" 2>/dev/null
log "profiles captured: $(ls -1 "$run/heap" 2>/dev/null | wc -l)"
log "diff the first against the last:"
log "  go tool pprof -base $run/heap/heap-0001.pb.gz $run/heap/$(ls -1 "$run/heap" 2>/dev/null | tail -1)"
log "done"
