#!/usr/bin/env bash
# Capture a goroutine dump of every node the moment the network wedges.
#
# On 2026-08-21 a fresh DI network stopped producing blocks on all four
# partitions while consensus kept running: votes, headers, certificates and
# `Bullshark: committing leader chain` all firing, no block or execution
# messages, and ZERO "Failed to process committed certificate" on any of the
# 13 nodes. So processCommittedCertificate was not erroring — it was never
# called, or never returned. A goroutine dump settles that in one step. The
# network was torn down before anyone took one (#4125), and by then the node
# logs had rotated down to the last 20 seconds, so the onset was gone too.
#
# This closes that gap without a human in the loop. soakmon already classifies
# a partition as stalled (#4123); this watches that classification and dumps.
#
#   RUN_DIR=runs/<id> ./wedgewatch.sh          # follow soakmon, dump on wedge
#   RUN_DIR=runs/<id> ./wedgewatch.sh --now    # dump right now, then exit
#
# It NEVER stops the network. Evidence first; the operator decides what to do
# with a wedged network that is still standing there to be poked.
set -uo pipefail
here="$(cd "$(dirname "$0")" && pwd)"

RUN_DIR="${RUN_DIR:-$here/runs/latest}"
MON="${MON_URL:-http://127.0.0.1:8099/data}"
POLL="${WEDGE_POLL:-10}"          # seconds between checks
WEDGE_SECS="${WEDGE_SECS:-120}"   # a partition must be stalled this long
MAX="${WEDGE_MAX:-3}"             # captures per run, so disk cannot run away
COOLDOWN="${WEDGE_COOLDOWN:-900}" # seconds between captures
PPROF_PORT="${PPROF_PORT:-6060}"

log() { echo "$(date -u +%FT%TZ) $*"; }

# One capture: every node's goroutines, plus enough context to read them.
# $1 = why, $2 = directory prefix. A hand-run probe must NOT land as wedge-*:
# the manifest verdict counts wedge-* dirs, and a baseline taken on a healthy
# network would read as "this run wedged once".
capture() {
  local why="$1"
  local pfx="${2:-wedge}"
  local ts; ts="$(date -u +%Y%m%dT%H%M%SZ)"
  local out="$RUN_DIR/$pfx-$ts"
  mkdir -p "$out" || { log "cannot create $out"; return 1; }
  log "CAPTURE[$pfx]: $why — capturing to $out"
  echo "$why" > "$out/reason.txt"

  # The monitor sample that triggered this, before anything else moves.
  curl -sf -m 10 "$MON" > "$out/soakmon-at-wedge.json" 2>/dev/null

  local cs; cs="$(docker ps --filter name=acc- --format '{{.Names}}' | sort)"
  echo "$cs" > "$out/containers.txt"
  docker ps -a --format '{{.Names}}\t{{.Status}}' > "$out/docker-ps.txt" 2>/dev/null
  docker stats --no-stream --format '{{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}' \
    > "$out/docker-stats.txt" 2>/dev/null

  # Goroutines from every node, in parallel — a wedge can move, and 13 serial
  # 10s curls would smear the snapshot across two minutes.
  local c
  for c in $cs; do
    (
      # debug=2 is the readable stack dump; the binary profile is what
      # `go tool pprof` wants. Take both, they disagree about nothing and
      # cost nothing.
      docker exec "$c" curl -s -m 15 \
        "http://localhost:$PPROF_PORT/debug/pprof/goroutine?debug=2" \
        > "$out/$c.goroutines.txt" 2>/dev/null
      docker exec "$c" curl -s -m 15 \
        "http://localhost:$PPROF_PORT/debug/pprof/goroutine" \
        > "$out/$c.goroutine.pb.gz" 2>/dev/null
      # A wedged executor holding a lock shows up here and nowhere else.
      docker exec "$c" curl -s -m 15 \
        "http://localhost:$PPROF_PORT/debug/pprof/block?debug=1" \
        > "$out/$c.block.txt" 2>/dev/null
      docker exec "$c" curl -s -m 10 \
        "http://localhost:26670/metrics" > "$out/$c.metrics.txt" 2>/dev/null
    ) &
  done
  wait

  # Drop empties so the directory shows at a glance which nodes answered.
  find "$out" -type f -empty -delete 2>/dev/null

  local got; got=$(ls "$out"/*.goroutines.txt 2>/dev/null | wc -l)
  local want; want=$(echo "$cs" | wc -w)
  log "CAPTURE[$pfx]: goroutine dumps from $got of $want nodes"
  if [ "$got" -eq 0 ]; then
    log "CAPTURE[$pfx]: no node answered pprof on :$PPROF_PORT — is the image older than the every-node-pprof change?"
  fi

  # A one-line read of where each node's executor is sitting. This is the
  # question #4125 asks, so answer it in the capture rather than making the
  # next reader grep 13 files.
  {
    echo "# processCommittedCertificate / batch-collection frames per node"
    for f in "$out"/*.goroutines.txt; do
      [ -e "$f" ] || continue
      printf '%-24s %s\n' "$(basename "$f" .goroutines.txt)" \
        "$(grep -cE 'processCommittedCertificate|collectBatches|CollectBatch' "$f" 2>/dev/null) frame(s); $(grep -c '^goroutine ' "$f" 2>/dev/null) goroutines"
    done
  } > "$out/SUMMARY.txt" 2>/dev/null
  cat "$out/SUMMARY.txt"
  log "CAPTURE[$pfx]: complete — network left UP on purpose"
}

if [ "${1:-}" = "--now" ]; then
  capture "manual capture (--now) — network state not asserted" probe
  exit 0
fi

log "wedgewatch: following $MON — dump after ${WEDGE_SECS}s stalled, max $MAX, run dir $RUN_DIR"
n=0
last=0
while :; do
  sleep "$POLL"
  [ "$n" -ge "$MAX" ] && continue   # keep the loop alive, stop spending disk

  d="$(curl -sf -m 8 "$MON" 2>/dev/null)" || continue
  # Which partitions are stalled, and for how long. "unknown" is NOT a wedge:
  # chaos pauses a node and the API it answers on goes with it.
  read -r worst names <<< "$(printf '%s' "$d" | python3 -c '
import json,sys
try: d=json.load(sys.stdin)
except Exception: print("0 -"); raise SystemExit
p=d.get("progress") or {}
s=[(v.get("stalledFor") or 0,k) for k,v in p.items() if v.get("state")=="stalled"]
print("%d %s" % (max([x[0] for x in s]) if s else 0, ",".join(sorted(k for _,k in s)) or "-"))
' 2>/dev/null)"
  [ -z "${worst:-}" ] && continue

  if [ "$worst" -ge "$WEDGE_SECS" ]; then
    now=$(date +%s)
    if [ $(( now - last )) -ge "$COOLDOWN" ] || [ "$last" -eq 0 ]; then
      capture "partitions stalled ${worst}s: $names"
      last=$(date +%s); n=$(( n + 1 ))
      [ "$n" -ge "$MAX" ] && log "wedgewatch: $MAX captures taken, no more will be written"
    fi
  fi
done
