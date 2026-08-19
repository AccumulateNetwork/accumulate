#!/usr/bin/env bash
# Chaos soak: 3 BVNs x 4 validators + bootstrap, cross-partition load, induced
# drops. Chaos restarts re-arm each node's drop hooks, so drops recur throughout.
#
#   DURATION=24h TPS=2 ./soak.sh "why I am running this"
#
# EVERY RUN WRITES TO ITS OWN DIRECTORY under runs/<UTC timestamp>/ and NOTHING
# IS EVER OVERWRITTEN. Earlier versions of this script truncated soak.log and
# monitor.csv on every start and appended undated lines to chaos.log, so a run's
# evidence was destroyed by the next run and what survived could not be dated.
# Two 20h results were nearly lost that way. Each run dir captures the exact
# commit, the exact config files, and the verdict, so a result stays readable
# long after the tree has moved on.
set -uo pipefail
here="$(cd "$(dirname "$0")" && pwd)"; repo="$(cd "$here/../../.." && pwd)"
DURATION="${DURATION:-24h}"; TPS="${TPS:-2}"

# Parse Go-style durations so short runs work. The old parser did
# `sed 's/h//' * 3600`, which silently produced a shell arithmetic error for
# anything but whole hours — so DURATION=5m ran chaos with end=now and no chaos
# at all, while the loadgen honoured the 5m. Short runs are how a targeted fix
# gets proven, so they have to work.
case "$DURATION" in
  *h) duration_seconds=$(( ${DURATION%h} * 3600 )) ;;
  *m) duration_seconds=$(( ${DURATION%m} * 60 )) ;;
  *s) duration_seconds=${DURATION%s} ;;
  *)  duration_seconds=$(( DURATION * 3600 )) ;;   # bare number = hours, as before
esac
# Chaos every ~10 min is meaningless in a 5-minute run; scale the interval so a
# short run still exercises disruption.
if [ "$duration_seconds" -le 1800 ]; then
  CHAOS_MIN=${CHAOS_MIN:-25}; CHAOS_JITTER=${CHAOS_JITTER:-20}
else
  CHAOS_MIN=${CHAOS_MIN:-480}; CHAOS_JITTER=${CHAOS_JITTER:-240}
fi
# A 5m grace on a 5m run doubles the wall clock for no benefit; scale it.
if [ "$duration_seconds" -le 1800 ]; then
  LG_GRACE=${LG_GRACE:-45s}; LG_TIMEOUT=${LG_TIMEOUT:-20m}
else
  LG_GRACE=${LG_GRACE:-5m}; LG_TIMEOUT=${LG_TIMEOUT:-26h}
fi
# The 100-sub-treasury bootstrap front-loads cross-partition funding traffic
# regardless of -tps, which keeps every channel busy. Reproducing an idle-stream
# stall (#4073) needs channels that actually go quiet, so allow it to be turned
# off. Default keeps the realistic funding spread.
LG_BOOTSTRAP=${LG_BOOTSTRAP:-100}
NOTE="${1:-}"

runs="$here/runs"
run_id="$(date -u +%Y%m%dT%H%M%SZ)"
rd="$runs/$run_id"
mkdir -p "$rd/config" || { echo "cannot create $rd"; exit 1; }
ln -sfn "$rd" "$runs/latest"

log="$rd/soak.log"; chaos="$rd/chaos.log"; mon="$rd/monitor.csv"
manifest="$rd/manifest.md"; runjson="$rd/run.json"
compose="docker compose -f $here/../docker-compose.yml"

# ---- provenance -------------------------------------------------------------
# Capture what is being tested BEFORE starting, because the tree will move on.
git_head=$(git -C "$repo" rev-parse HEAD 2>/dev/null || echo unknown)
git_desc=$(git -C "$repo" describe --tags --always --dirty 2>/dev/null || echo unknown)
git_branch=$(git -C "$repo" rev-parse --abbrev-ref HEAD 2>/dev/null || echo unknown)
git_dirty=$(git -C "$repo" status --porcelain 2>/dev/null | wc -l)
exec_ver=$(grep -E '^\s*executorVersion:' "$here/../docker-network.yml" | head -1 | sed 's/.*: *//; s/"//g')
# From v1.4.5 healing has no configuration — the conductor always heals. Older
# trees injected enable-*-healing into accumulate.toml, so keep reading it: a
# run against an older image must still record what it was configured with.
# Strip comments first: the compose has an explanatory header mentioning
# "enable-synthetic-healing = true", and matching that made a v1.4.5 run — which
# has no healing config at all — report the flag as set. A provenance record
# that quietly reports the opposite of the truth is worse than none.
heal_flags=$(sed 's/#.*//' "$here/docker-compose.yml" \
  | grep -oE 'enable-[a-z-]*healing = [a-z]+' | sort -u | paste -sd'; ' -)
heal_flags="${heal_flags:-unconditional (DI conductor, #4105)}"
# The compose declares these as "${DROP_SYN-<default>}", so the value that
# actually reaches the nodes depends on the environment. Record the EFFECTIVE
# value — recording the template would make two differently-configured runs look
# identical in the manifest.
composed_default() { # $1=env var name, $2=compose key
  # Strip only the leading "KEY: " — a greedy .*: would eat into the value,
  # whose own patterns contain colons (e.g. "*:%499+3").
  grep -oE "$2: *\"[^\"]*\"" "$here/docker-compose.yml" | head -1 \
    | sed "s/^$2: *//; s/\"//g; s/^\${[A-Za-z_][A-Za-z0-9_]*-//; s/}$//"
}
drop_synth="${DROP_SYN:-$(composed_default DROP_SYN ACC_DEBUG_DROP_SYNTHETIC)}"
drop_anchor="${DROP_ANC:-$(composed_default DROP_ANC ACC_DEBUG_DROP_ANCHOR)}"
soak_image="${SOAK_IMAGE:-docker-bvn1-val1}"
image_id=$(docker image inspect --format '{{.Id}}' "$soak_image" 2>/dev/null || echo unknown)
n_bvn=$(grep -cE '^\s*- id: "BVN' "$here/../docker-network.yml")
n_node=$(grep -cE '^\s*- listenAddress:' "$here/../docker-network.yml")

# Freeze the exact config. A diff against these is the only reliable way to know
# what changed between two runs.
cp "$here/docker-compose.yml" "$here/../docker-network.yml" "$0" "$rd/config/" 2>/dev/null
git -C "$repo" diff > "$rd/config/uncommitted.patch" 2>/dev/null

{
  echo "# Soak run $run_id"
  echo
  [ -n "$NOTE" ] && { echo "**Purpose:** $NOTE"; echo; }
  echo "| field | value |"
  echo "|---|---|"
  echo "| started (UTC) | $(date -u +%FT%TZ) |"
  echo "| commit | \`$git_head\` |"
  echo "| describe | \`$git_desc\` |"
  echo "| branch | \`$git_branch\` |"
  echo "| uncommitted files | $git_dirty $([ "$git_dirty" -gt 0 ] && echo '(see config/uncommitted.patch)') |"
  echo "| image | \`$soak_image\` |"
  echo "| image id | \`$image_id\` |"
  echo "| executor version | **$exec_ver** |"
  echo "| healing | $heal_flags |"
  echo "| synthetic drops | \`$drop_synth\` |"
  echo "| anchor drops | \`${drop_anchor:-none}\` |"
  echo "| topology | $n_bvn BVNs, $n_node nodes + bootstrap |"
  echo "| target duration | $DURATION |"
  echo "| target TPS | $TPS |"
  echo
  echo "Config as run is frozen in \`config/\`. Results appended below on exit."
} > "$manifest"

printf '{"runId":"%s","startedUtc":"%s","image":"%s","imageId":"%s","commit":"%s","describe":"%s","branch":"%s","uncommittedFiles":%s,"executorVersion":"%s","healing":"%s","dropSynthetic":"%s","dropAnchor":"%s","bvns":%s,"nodes":%s,"duration":"%s","tps":"%s","note":"%s"}\n' \
  "$run_id" "$(date -u +%FT%TZ)" "$soak_image" "$image_id" "$git_head" "$git_desc" "$git_branch" "$git_dirty" \
  "$exec_ver" "$heal_flags" "$drop_synth" "${drop_anchor:-none}" "$n_bvn" "$n_node" \
  "$DURATION" "$TPS" "$NOTE" > "$runjson"

echo "== soak start $(date -u) duration=$DURATION tps=$TPS ==" | tee "$log"
echo "   run dir: $rd" | tee -a "$log"
echo "   commit:  $git_desc ($git_head)" | tee -a "$log"
echo "   image:   $soak_image ($image_id)" | tee -a "$log"
echo "   version: $exec_ver | healing: $heal_flags" | tee -a "$log"

$compose down -v --remove-orphans >/dev/null 2>&1
# Build BEFORE up. `up -d` reuses an existing image silently, and every
# conclusion drawn from such a run is about the wrong build (#4103).
$compose build >/dev/null 2>&1 || { echo "compose build failed" | tee -a "$log"; exit 1; }
$compose up -d >/dev/null 2>&1 || { echo "up failed" | tee -a "$log"; exit 1; }

# Record the image actually running, so a rebuild later cannot be confused for this run.
docker image inspect --format '{{.Id}} {{.RepoTags}}' "$soak_image" \
  > "$rd/config/image.txt" 2>/dev/null

up=""; for _ in $(seq 1 90); do
  curl -sf -X POST http://localhost:26660/v3 -H 'content-type: application/json' \
    -d '{"jsonrpc":"2.0","id":1,"method":"network-status","params":{"partition":"Directory"}}' >/dev/null 2>&1 && { up=1; break; }
  sleep 5
done
[ -n "$up" ] || { echo "network never came up" | tee -a "$log"; exit 1; }
sleep 30

# Load generator (host): drives the full menu of user transaction types against
# an ever-growing account set. -faucet-seed FAUCET matches init's genesis faucet.
# Rotate across all 12 nodes so one chaos-disrupted node neither rejects traffic
# nor carries the whole load.
EPS=$(for p in $(seq 26660 26671); do printf "http://localhost:%d," "$p"; done | sed 's/,$//')
nohup go run "$repo/tools/cmd/loadgen" -endpoints "$EPS" \
  -faucet-seed FAUCET -tps "$TPS" -duration "$DURATION" -timeout "$LG_TIMEOUT" \
  -bootstrap "$LG_BOOTSTRAP" \
  -grace "$LG_GRACE" -max-stranded 20 -stats-file "$rd/loadgen-stats.json" >> "$log" 2>&1 &
DRIVER=$!

# Observability. Launch these WITH the run, never by hand afterwards — the
# coarse monitor below only samples height/heals/CPU every 5 min and cannot see
# a wedged stream, which is usually the thing being tested. soakmon serves the
# per-stream gap/wedge/heal detail that seizewatch trips on; without it a
# seizure is invisible until the loadgen strands transactions much later.
if [ -x "$here/soakmon.py" ]; then
  # RUN_DIR so the dashboard reads THIS run's loadgen stats and chaos log.
  nohup env RUN_DIR="$rd" "$here/soakmon.py" > "$rd/soakmon.log" 2>&1 &
  MON=$!
  for _ in $(seq 1 20); do
    curl -sf -m3 http://127.0.0.1:8099/data >/dev/null 2>&1 && break
    sleep 3
  done
  curl -sf -m3 http://127.0.0.1:8099/data >/dev/null 2>&1 \
    && echo "   soakmon: http://127.0.0.1:8099" | tee -a "$log" \
    || echo "   WARNING: soakmon did not come up — no wedge detection this run" | tee -a "$log"
fi
if [ -x "$here/seizewatch.sh" ]; then
  nohup "$here/seizewatch.sh" > "$rd/seizewatch.out" 2>&1 &
  SEIZE=$!
fi

# Chaos: every ~10 min disturb ONE random node (quorum 3/4 preserved).
# Full ISO dates — time-of-day alone cannot be attributed to a run.
( end=$(( $(date +%s) + duration_seconds ))
  nodes=$(docker ps --filter name=acc-bvn --format '{{.Names}}')
  while [ "$(date +%s)" -lt "$end" ]; do
    sleep $(( CHAOS_MIN + RANDOM % CHAOS_JITTER ))
    n=$(echo "$nodes" | shuf -n1); r=$((RANDOM % 10))
    if [ "$r" -lt 4 ]; then
      echo "$(date -u +%FT%TZ) restart $n" >> "$chaos"; docker restart "$n" >/dev/null 2>&1
    elif [ "$r" -lt 8 ]; then
      p=$((60 + RANDOM % 120))
      echo "$(date -u +%FT%TZ) pause $n ${p}s" >> "$chaos"
      docker pause "$n" >/dev/null 2>&1; sleep "$p"; docker unpause "$n" >/dev/null 2>&1
    else
      echo "$(date -u +%FT%TZ) skip" >> "$chaos"
    fi
  done ) &
CHAOS=$!

# Monitor: heights + total heals every 5 min
echo "time,dnHeight,heals,cpuPct" > "$mon"
( while kill -0 $DRIVER 2>/dev/null; do
    h=$(curl -s -X POST http://localhost:26660/v3 -H 'content-type: application/json' \
      -d '{"jsonrpc":"2.0","id":1,"method":"query","params":{"scope":"acc://dn.acme/ledger"}}' \
      | grep -oE '"index":[0-9]+' | head -1 | cut -d: -f2)
    heals=0
    for c in $(docker ps --filter name=acc-bvn --format '{{.Names}}'); do
      x=$(docker exec "$c" sh -c '
        nid=$(curl -s -X POST http://localhost:26660/v3 -H "content-type: application/json" -d "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"node-info\",\"params\":{}}" | grep -oE "\"peerID\":\"[^\"]+\"" | cut -d"\"" -f4)
        for part in Directory BVN1 BVN2 BVN3; do
          curl -s -X POST http://localhost:26660/v3 -H "content-type: application/json" -d "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"consensus-status\",\"params\":{\"partition\":\"$part\",\"nodeID\":\"$nid\"}}" | grep -oE "\"(syntheticHeals|anchorHeals)\":[0-9]+" | cut -d: -f2
        done' 2>/dev/null | paste -sd+ - | bc 2>/dev/null)
      heals=$((heals + ${x:-0}))
    done
    cpu=$(docker stats --no-stream --format '{{.CPUPerc}}' 2>/dev/null | tr -d '%' | awk '{s+=$1} END {printf "%.0f", s}')
    echo "$(date -u +%FT%T),${h:-?},$heals,${cpu:-?}" >> "$mon"
    sleep ${MON_INTERVAL:-$([ "$duration_seconds" -le 1800 ] && echo 20 || echo 300)}
  done ) &

wait $DRIVER; rc=$?

# Keep the network running after the load stops. Recovery of a TAIL loss can only
# be observed once the loss has aged past reconcileGraceBlocks, and while load
# continues most losses are recovered by the ordinary gap healer long before
# that. An idle tail is the only window in which the interval reconcile is the
# mechanism actually doing the work — without it a run ends with stragglers that
# were simply too young, which reads as "the fix did nothing".
if [ "${IDLE_AFTER:-0}" -gt 0 ]; then
  echo "== load finished; idling ${IDLE_AFTER}s so tail losses age past the grace ==" | tee -a "$log"
  sleep "$IDLE_AFTER"
fi
kill $CHAOS ${MON:-} ${SEIZE:-} 2>/dev/null
ended=$(date -u +%FT%TZ)
echo "== soak finished $(date -u) driver-exit=$rc ==" | tee -a "$log"

# Capture evidence that only exists while the containers are alive. The
# interval reconcile (#4073) logs each pull, and those logs die with the
# containers — so a run that proved the fix would otherwise leave no trace.
$compose logs --no-color > "$rd/node-logs.txt" 2>/dev/null
grep "Reconcile: pulled messages" "$rd/node-logs.txt" > "$rd/reconcile-pulls.txt" 2>/dev/null
reconcile_pulls=$(wc -l < "$rd/reconcile-pulls.txt" 2>/dev/null || echo 0)
# Final produced-vs-received across every channel, the check that sees a stall.
if [ -x "$here/streams.py" ]; then
  "$here/streams.py" > "$rd/streams-final.txt" 2>&1
  stalled_end=$(grep -oE 'stalled channels: [0-9]+' "$rd/streams-final.txt" | grep -oE '[0-9]+' | head -1)
fi
stalled_end="${stalled_end:-unknown}"

# ---- verdict ----------------------------------------------------------------
elapsed_h=$(python3 -c "
import json
try:
    s=json.load(open('$rd/loadgen-stats.json')); print(round(s.get('elapsedSec',0)/3600,2))
except Exception: print('?')" 2>/dev/null)
first_h=$(sed -n '2p' "$mon" | cut -d, -f2); last_h=$(tail -1 "$mon" | cut -d, -f2)
first_x=$(sed -n '2p' "$mon" | cut -d, -f3); last_x=$(tail -1 "$mon" | cut -d, -f3)
n_chaos=$(wc -l < "$chaos" 2>/dev/null || echo 0)

{
  echo
  echo "## Result"
  echo
  echo "| field | value |"
  echo "|---|---|"
  echo "| ended (UTC) | $ended |"
  echo "| elapsed | ${elapsed_h}h |"
  echo "| driver exit | $rc $([ "$rc" -eq 0 ] && echo '(clean)' || echo '(FAILED)') |"
  echo "| dn height | ${first_h:-?} -> ${last_h:-?} |"
  echo "| heals | ${first_x:-?} -> ${last_x:-?} |"
  echo "| chaos events | $n_chaos |"
  echo "| monitor samples | $(( $(wc -l < "$mon") - 1 )) |"
  echo "| seizure | $(grep -q SEIZED "$rd/seizewatch.out" 2>/dev/null && grep SEIZED "$rd/seizewatch.out" | tail -1 || echo 'none detected') |"
  echo "| reconcile pulls (#4073) | $reconcile_pulls |"
  echo "| stalled channels at end | $stalled_end |"
  echo
  echo "Raw: \`soak.log\`, \`monitor.csv\`, \`chaos.log\`, \`loadgen-stats.json\`."
} >> "$manifest"

# Accumulating index — one line per run, newest last, never rewritten.
idx="$runs/INDEX.md"
[ -f "$idx" ] || printf '# Soak runs\n\nEvery run appends one row. Details in `<runId>/manifest.md`.\n\n| run | commit | executor | healing | elapsed | exit | dn height | heals | note |\n|---|---|---|---|---|---|---|---|---|\n' > "$idx"
printf '| [%s](%s/manifest.md) | `%s` | %s | %s | %sh | %s | %s→%s | %s→%s | %s |\n' \
  "$run_id" "$run_id" "$git_desc" "$exec_ver" "$heal_flags" "$elapsed_h" "$rc" \
  "${first_h:-?}" "${last_h:-?}" "${first_x:-?}" "${last_x:-?}" "$NOTE" >> "$idx"

echo
echo "Run recorded: $rd/manifest.md"
echo "Index:        $idx"
tail -3 "$log"
exit $rc
