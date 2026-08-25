#!/usr/bin/env bash
# Walk the loadgen up a rate ladder, and STOP CLIMBING when the network stops
# keeping up.
#
#   RUN_DIR=runs/<id> ./ladder.sh                     # 100 250 500 750 1000
#   RUN_DIR=runs/<id> STEPS="50 100 200" STEP_SECS=600 ./ladder.sh
#
# Why this exists. The rate knee was previously found by running a whole soak
# at one target, watching it collapse, and starting another soak a rate lower —
# five runs to bracket one number, each of them a fresh genesis and a fresh
# bootstrap. Worse, a run that collapsed at t+20min then SAT collapsed for the
# remaining hours: run 20260822T015342Z spent two hours with a dead Directory
# after it had produced every piece of evidence it was going to.
#
# The loadgen's control API can change the rate without a restart (a restart
# re-bootstraps the entire account universe, which is why this was never done
# by hand). So one run can climb, notice the knee, step back to the last rate
# that held, and spend its remaining hours proving THAT rate sustainable. Two
# results from one night instead of half of one.
#
# The judgement is deliberately conservative: a step "holds" only if the
# network actually kept up with it. Achieving 300/s against a 750 target is
# not a 300/s result — it is a network in overload that happens to be
# dequeuing at 300, and continuing to climb from there measures nothing. That
# distinction is the entire point, because the 12-node history is full of it:
# an 800-tps target achieved 47/s while a 250-tps target achieved 220/s.
set -uo pipefail
here="$(cd "$(dirname "$0")" && pwd)"

RUN_DIR="${RUN_DIR:-$here/runs/latest}"
CONTROL="${CONTROL:-http://127.0.0.1:8091/control}"
MONITOR="${MONITOR:-http://127.0.0.1:8099/data}"
STEPS="${STEPS:-100 250 500 750 1000}"
STEP_SECS="${STEP_SECS:-2700}"          # 45 min per rung
SETTLE_SECS="${SETTLE_SECS:-300}"       # ignore the first 5 min of a rung
# A rung holds if the achieved rate reaches this fraction of its target. 0.8 is
# loose on purpose: the generator's own bookkeeping (funding, reconcile) eats a
# few percent, and a rung that lands at 0.85 is a real result at that rate.
HOLD_FRACTION="${HOLD_FRACTION:-0.8}"

log="$RUN_DIR/ladder.log"
csv="$RUN_DIR/ladder.csv"
say() { echo "$(date -u +%FT%TZ) $*" | tee -a "$log"; }

[ -d "$RUN_DIR" ] || { echo "no such run dir: $RUN_DIR" >&2; exit 1; }
[ -f "$csv" ] || echo "startedUtc,endedUtc,targetTps,achievedTps,generated,rejected,skipped,status,rssMaxMiB,rssMaxNode,verdict" > "$csv"

# --- readings ---------------------------------------------------------------
# Both of these must fail SOFT. A monitor blip must not be read as a collapse
# and end the climb, and it must not be read as health either — the caller
# distinguishes "cannot tell" from "bad".

# Cumulative generated count and the instant it was taken. The loadgen's own
# `rate` field is an average over the whole run, so it lags a rate change by
# hours; the only honest instantaneous rate is d(generated)/d(t) across a
# window, which is what the rungs are measured with.
gen_sample() {
  python3 -c '
import json, sys
try:
    d = json.load(open(sys.argv[1]))
except Exception:
    sys.exit(1)
print("%d %d %d %d" % (d.get("generated", 0), d.get("updatedUnix", 0),
                       d.get("rejected", 0), d.get("skipped", 0)))
' "$RUN_DIR/loadgen-stats.json" 2>/dev/null
}

# Network verdict and the worst node RSS, straight from the monitor.
health() {
  curl -sf -m 8 "$MONITOR" 2>/dev/null | python3 -c '
import json, sys
try:
    d = json.load(sys.stdin)
except Exception:
    print("unknown 0 -"); sys.exit(0)
ns = (d.get("scrape") or {}).get("nodes") or {}
print("%s %s %s" % (d.get("status") or "unknown",
                    ns.get("rssMaxMiB") or 0,
                    ns.get("rssMaxNode") or "-"))
' 2>/dev/null || echo "unknown 0 -"
}

set_tps() {
  local t="$1" out
  out=$(curl -sf -m 8 -X POST -d "{\"tps\": $t}" "$CONTROL" 2>/dev/null)
  if [ -z "$out" ]; then
    say "CONTROL FAILED setting tps=$t — the loadgen control API did not answer"
    return 1
  fi
  return 0
}

# --- wait for the generator -------------------------------------------------
# The ladder must not start stepping during the bootstrap phase: the first
# minutes are the 100-sub-treasury funding burst, whose rate says nothing about
# the network's capacity at the configured target.
say "ladder starting: steps=[$STEPS] step=${STEP_SECS}s settle=${SETTLE_SECS}s hold>=${HOLD_FRACTION}x"
for _ in $(seq 1 120); do
  curl -sf -m 5 "$CONTROL" >/dev/null 2>&1 && break
  sleep 10
done
if ! curl -sf -m 5 "$CONTROL" >/dev/null 2>&1; then
  say "loadgen control API never came up at $CONTROL — ladder aborting, the run continues at its start rate"
  exit 1
fi
say "control API up"

# Wait for the generator to leave bootstrap, but never forever: if the phase
# never flips, climbing anyway is better than a run that stays at its start
# rate all night because one field was not what this script expected.
for _ in $(seq 1 90); do
  phase=$(python3 -c '
import json,sys
try: print(json.load(open(sys.argv[1])).get("phase",""))
except Exception: print("")' "$RUN_DIR/loadgen-stats.json" 2>/dev/null)
  [ "$phase" = "generating" ] && break
  sleep 20
done
say "generator phase=${phase:-unknown}; beginning the climb"

# --- climb ------------------------------------------------------------------
best=""; knee=""
for target in $STEPS; do
  set_tps "$target" || { say "ABORTING ladder at $target — control unreachable"; break; }
  say "--- rung ${target} tps: settling ${SETTLE_SECS}s ---"
  sleep "$SETTLE_SECS"

  read -r g0 t0 r0 s0 <<<"$(gen_sample)"
  started=$(date -u +%FT%TZ)
  measure=$(( STEP_SECS - SETTLE_SECS ))
  [ "$measure" -lt 60 ] && measure=60
  sleep "$measure"
  read -r g1 t1 r1 s1 <<<"$(gen_sample)"
  ended=$(date -u +%FT%TZ)
  read -r status rss rssnode <<<"$(health)"

  if [ -z "${g1:-}" ] || [ -z "${g0:-}" ] || [ "${t1:-0}" -le "${t0:-0}" ]; then
    say "rung $target: CANNOT MEASURE (loadgen stats unreadable) — holding here rather than climbing blind"
    knee="$target"; break
  fi

  achieved=$(python3 -c "print('%.1f' % ((($g1)-($g0))/max(1,($t1)-($t0))))")
  ok=$(python3 -c "print('yes' if $achieved >= $HOLD_FRACTION * $target else 'no')")

  # A stalled or down network fails the rung regardless of the arithmetic: a
  # partition that stopped producing blocks can still show a healthy dequeue
  # rate for a while as the backlog drains.
  verdict="held"
  [ "$ok" = no ] && verdict="short"
  case "$status" in
    stalled|down) verdict="unhealthy" ;;
  esac

  echo "$started,$ended,$target,$achieved,$((g1-g0)),$((r1-r0)),$((s1-s0)),$status,$rss,$rssnode,$verdict" >> "$csv"
  say "rung $target: achieved=${achieved}/s rejected=+$((r1-r0)) skipped=+$((s1-s0)) status=$status rssMax=${rss}MiB($rssnode) -> $verdict"

  if [ "$verdict" = held ]; then
    best="$target"
    continue
  fi
  knee="$target"
  say "KNEE at $target tps (achieved ${achieved}/s, status=$status)"
  break
done

# --- hold -------------------------------------------------------------------
if [ -n "$knee" ] && [ -n "$best" ]; then
  say "stepping back to the last rate that held: $best tps, and holding there for the rest of the run"
  set_tps "$best"
elif [ -n "$knee" ]; then
  # The very first rung failed. Do not climb, do not step below the operator's
  # floor — leave it and let the run record why.
  say "the FIRST rung ($knee) did not hold; leaving the rate where it is. This is a result, not a harness fault."
elif [ -n "$best" ]; then
  say "every rung held, up to and including $best tps — holding there for the rest of the run"
fi
say "ladder done: best=${best:-none} knee=${knee:-none}"
