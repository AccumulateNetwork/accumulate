#!/usr/bin/env bash
# Stop a soak that has stalled, instead of letting it burn hours proving the
# same thing over and over.
#
# Run 20260822T015342Z spent two hours with a dead Directory and three
# unrecoverable validators after it had already produced its evidence. Nothing
# after the first minute of a stall added anything: the goroutine dumps were
# taken, the diagnostic line was in the log, and the remaining 11 hours of the
# target were only ever going to write 137MB/hour of the same warning.
#
#   RUN_DIR=runs/<id> ./stallkill.sh          # watch, and stop the run on a stall
#   STALL_KILL_SECS=600 RUN_DIR=... ./stallkill.sh   # more patience
#
# Evidence first, then stop: this takes a final capture before it touches
# anything, and it stops the run the way a clean finish does — signal the load
# generator, let soak.sh write its own verdict and collect its evidence, and
# only then take the network down.
set -uo pipefail
here="$(cd "$(dirname "$0")" && pwd)"

RUN_DIR="${RUN_DIR:-$here/runs/latest}"
MON="${MON_URL:-http://127.0.0.1:8099/data}"
POLL="${STALL_POLL:-10}"
# Four minutes, not one. The load generator's bootstrap wait alone freezes
# every height for about five minutes, and while the idle guard below now
# recognises that, a threshold this side of a minute leaves no room for any
# other legitimate quiet stretch — a chaos pause, a slow catch-up, a heal
# storm. A run is expensive to restart and cheap to let breathe.
KILL_SECS="${STALL_KILL_SECS:-240}"
# Never reach past this network. The ASP mainnet fleet shares a compose
# directory name, so an unpinned `down` would treat those containers as
# orphans and delete them (#4124).
export COMPOSE_PROJECT_NAME="${COMPOSE_PROJECT_NAME:-disoak}"

log() { echo "$(date -u +%FT%TZ) $*"; }

log "stallkill: watching $MON — stop the run after ${KILL_SECS}s stalled"

while :; do
  sleep "$POLL"

  # The run ended on its own (duration reached, or it failed). Nothing left to
  # guard, and soak.sh does not kill this watchdog — so leave under our own
  # power rather than polling a dead monitor forever.
  if ! pgrep -f 'bash ./soak.sh' >/dev/null 2>&1; then
    log "stallkill: the run has ended; standing down without touching anything"
    exit 0
  fi

  d="$(curl -sf -m 8 "$MON" 2>/dev/null)" || continue

  read -r worst names blocks empties <<< "$(printf '%s' "$d" | python3 -c '
import json,sys
try: d=json.load(sys.stdin)
except Exception: print("0 - 0 0"); raise SystemExit
p=d.get("progress") or {}
s=[(v.get("stalledFor") or 0,k) for k,v in p.items() if v.get("state")=="stalled"]
lf=d.get("life") or {}
print("%d %s %d %d" % (max([x[0] for x in s]) if s else 0,
                       ",".join(sorted(k for _,k in s)) or "-",
                       lf.get("blocks") or 0, lf.get("blocksEmpty") or 0))
' 2>/dev/null)"
  [ -z "${worst:-}" ] && continue

  # An IDLE network is not a wedged one, and the monitor cannot tell them
  # apart: it measures the ledger index, which only moves when a block has
  # content. While the load generator sits in its five-minute bootstrap wait
  # the network commits empty rounds, the index does not move, and every
  # height reads as stalled. This watchdog killed a perfectly healthy run that
  # way four minutes in.
  #
  # Block production is the discriminator. Idle: blocks keep being produced and
  # every one of them is empty. Wedged: no blocks at all, because the executor
  # is stuck collecting a certificate's batches.
  if [ "${blocks:-0}" -gt "${prev_blocks:-0}" ]; then
    made=$(( blocks - ${prev_blocks:-0} ))
    made_empty=$(( empties - ${prev_empties:-0} ))
    if [ "$made_empty" -ge "$made" ]; then
      [ -n "${idle_noted:-}" ] || log "idle, not stalled: $made blocks produced since the last check, all empty — not counting this against the threshold"
      idle_noted=1
      prev_blocks=$blocks; prev_empties=$empties
      continue
    fi
  fi
  idle_noted=""
  prev_blocks=$blocks; prev_empties=$empties

  [ "$worst" -lt "$KILL_SECS" ] && continue

  reason="stalled ${worst}s: $names"
  log "STOPPING: $reason"

  # 1. Evidence first. wedgewatch may have spent its captures already; this one
  #    is the state at the moment we decided to stop.
  if [ -x "$here/wedgewatch.sh" ]; then
    RUN_DIR="$RUN_DIR" "$here/wedgewatch.sh" --now 2>&1 | sed 's/^/  /'
  fi

  # 2. Say why in the run's own record, so the short elapsed time in INDEX.md
  #    is not mistaken for a crash or a cancelled run.
  {
    echo
    echo "## Stopped early by stallkill"
    echo
    echo "- stopped (UTC): $(date -u +%FT%TZ)"
    echo "- reason: $reason (threshold ${KILL_SECS}s)"
    echo
    echo "The run was ended once a partition had been stalled past the"
    echo "threshold. Evidence was captured before stopping; see the probe-*"
    echo "directory written at that moment."
  } >> "$RUN_DIR/manifest.md" 2>/dev/null

  # 3. Stop the load generator. soak.sh is waiting on it, so this makes the
  #    script run its ordinary exit path — verdict, node logs, stream summary,
  #    INDEX row — rather than dying mid-record.
  lg=$(pgrep -f 'loadgen -endpoints' | tr '\n' ' ')
  if [ -n "$lg" ]; then
    log "signalling loadgen: $lg"
    kill $lg 2>/dev/null
  else
    log "no loadgen found; soak.sh may already be finishing"
  fi

  # 4. Wait for soak.sh to finish recording before removing the containers —
  #    it reads them for the final logs and stream summary.
  for _ in $(seq 1 120); do
    pgrep -f 'bash ./soak.sh' >/dev/null 2>&1 || break
    sleep 5
  done
  if pgrep -f 'bash ./soak.sh' >/dev/null 2>&1; then
    log "soak.sh still running after 10m; taking the network down anyway"
  fi

  # 5. Down. Pinned project, so this can only ever reach the soak network.
  log "tearing down (project $COMPOSE_PROJECT_NAME)"
  docker compose -f "$here/../docker-compose.yml" down -v --remove-orphans >/dev/null 2>&1
  log "stallkill: run stopped"
  exit 0
done
