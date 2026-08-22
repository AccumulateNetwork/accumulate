#!/usr/bin/env bash
# Stop a soak that has stopped being worth running.
#
# Two conditions end a run:
#
#   1. A partition stalled past STALL_KILL_SECS. Run 20260822T015342Z spent two
#      hours with a dead Directory and three unrecoverable validators after it
#      had already produced its evidence; the rest was only ever going to write
#      137MB/hour of the same warning.
#
#   2. The monitor is unreachable. soak.sh gates the start of load on soakmon
#      answering, but that is a STARTUP check, not a liveness guarantee — in
#      run 20260822T052535Z soakmon passed the gate, died, and the run kept
#      generating load against a network nobody was watching. Every watcher
#      treated the dead endpoint as "try again later", so nothing noticed. A
#      blind watchdog is not a watchdog, and an unobserved soak produces
#      evidence nobody can trust.
#
#   RUN_DIR=runs/<id> ./stallkill.sh
#   STALL_KILL_SECS=600 RUN_DIR=... ./stallkill.sh    # more patience
#   STALL_KILL_SECS=0                                 # disabled (soak.sh reads this)
#
# Evidence first, then stop the way a clean finish does: capture, signal the
# load generator so soak.sh writes its own verdict, wait for that, then down.
set -uo pipefail
here="$(cd "$(dirname "$0")" && pwd)"

RUN_DIR="${RUN_DIR:-$here/runs/latest}"
MON="${MON_URL:-http://127.0.0.1:8099/data}"
POLL="${STALL_POLL:-10}"
# Four minutes, not one. The load generator's bootstrap wait alone freezes every
# height for about five minutes; the idle guard below recognises that, but other
# legitimate quiet stretches exist — a chaos pause, a slow catch-up, a heal
# storm. A run is expensive to restart and cheap to let breathe.
KILL_SECS="${STALL_KILL_SECS:-240}"
MON_DEAD_SECS="${MON_DEAD_SECS:-120}"
# Never reach past this network. The ASP mainnet fleet shares a compose
# directory name, so an unpinned `down` would treat those containers as
# orphans and delete them (#4124).
export COMPOSE_PROJECT_NAME="${COMPOSE_PROJECT_NAME:-disoak}"

log() { echo "$(date -u +%FT%TZ) $*"; }

# stop_run: evidence first, then end the run the way a clean finish does.
stop_run() {
  local reason="$1"

  # 1. Evidence. wedgewatch may have spent its captures already; this one is
  #    the state at the moment we decided to stop.
  if [ -x "$here/wedgewatch.sh" ]; then
    RUN_DIR="$RUN_DIR" "$here/wedgewatch.sh" --now 2>&1 | sed 's/^/  /'
  fi

  # 2. Say why in the run's own record, so a short elapsed time in INDEX.md is
  #    not mistaken for a crash or a cancelled run.
  {
    echo
    echo "## Stopped early by stallkill"
    echo
    echo "- stopped (UTC): $(date -u +%FT%TZ)"
    echo "- reason: $reason"
    echo
    echo "Evidence was captured before stopping; see the probe-* directory"
    echo "written at that moment."
  } >> "$RUN_DIR/manifest.md" 2>/dev/null

  # 3. Stop the load generator. soak.sh is waiting on it, so this makes the
  #    script run its ordinary exit path — verdict, node logs, stream summary,
  #    INDEX row — rather than dying mid-record.
  local lg
  lg=$(pgrep -f 'loadgen -endpoints' | tr '\n' ' ')
  if [ -n "$lg" ]; then
    log "signalling loadgen: $lg"
    kill $lg 2>/dev/null
  else
    log "no loadgen found; soak.sh may already be finishing"
  fi

  # 4. Wait for soak.sh to finish recording before removing the containers —
  #    it reads them for the final logs and stream summary.
  local i
  for i in $(seq 1 120); do
    pgrep -f 'bash \./soak\.sh' >/dev/null 2>&1 || break
    sleep 5
  done

  # 5. Down. Pinned project, so this can only ever reach the soak network.
  log "tearing down (project $COMPOSE_PROJECT_NAME)"
  docker compose -f "$here/../docker-compose.yml" down -v --remove-orphans >/dev/null 2>&1
  log "stallkill: run stopped"
}

log "stallkill: watching $MON — stop after ${KILL_SECS}s stalled, or ${MON_DEAD_SECS}s unmonitored"

blind=0
prev_blocks=0
prev_empties=0
idle_noted=""

while :; do
  sleep "$POLL"

  # The run ended on its own. soak.sh does not kill this watchdog (it would
  # leave the containers up when we are the ones ending the run), so leave
  # under our own power.
  if ! pgrep -f 'bash \./soak\.sh' >/dev/null 2>&1; then
    log "stallkill: the run has ended; standing down without touching anything"
    exit 0
  fi

  # A blind watchdog is not a watchdog.
  if ! d="$(curl -sf -m 8 "$MON" 2>/dev/null)"; then
    blind=$(( blind + 1 ))
    if [ "$(( blind * POLL ))" -ge "$MON_DEAD_SECS" ]; then
      log "STOPPING: monitor unreachable for $(( blind * POLL ))s — refusing to keep running unobserved"
      stop_run "monitor unreachable for $(( blind * POLL ))s"
      exit 0
    fi
    continue
  fi
  blind=0

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
  # content. While the load generator sits in its bootstrap wait the network
  # commits empty rounds and every height reads as stalled. This watchdog
  # killed a healthy run that way four minutes in.
  #
  # Block production is the discriminator. Idle: blocks keep being produced and
  # all of them are empty. Wedged: no blocks at all, because the executor is
  # stuck collecting a certificate's batches.
  if [ "${blocks:-0}" -gt "$prev_blocks" ]; then
    made=$(( blocks - prev_blocks ))
    made_empty=$(( empties - prev_empties ))
    prev_blocks=$blocks; prev_empties=$empties
    if [ "$made_empty" -ge "$made" ]; then
      [ -n "$idle_noted" ] || log "idle, not stalled: $made blocks produced since the last check, all empty — not counting this against the threshold"
      idle_noted=1
      continue
    fi
  else
    prev_blocks=$blocks; prev_empties=$empties
  fi
  idle_noted=""

  [ "$worst" -lt "$KILL_SECS" ] && continue

  log "STOPPING: stalled ${worst}s: $names"
  stop_run "stalled ${worst}s: $names (threshold ${KILL_SECS}s)"
  exit 0
done
