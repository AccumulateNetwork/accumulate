#!/usr/bin/env bash
# Chaos soak: 3 BVNs x 4 validators + bootstrap, cross-partition load, and
# container disturbance on a cadence (restart or pause one BVN node). That is
# the fault model; nothing is dropped in-band. A restarted leader loses what it
# was dispatching, which is the only message loss this harness produces.
#
#   ./soak.sh "why I am running this"              # every knob from soak.conf
#   ./soak.sh -c my.conf "why I am running this"   # soak.conf, then my.conf on top
#
# Knobs live in soak.conf, a checked-in file frozen into every run directory —
# not in the launching shell. (Five runs on 2026-09-05 silently built a leveldb
# network because ACC_STORAGE happened to be unset; the storage backend now
# lives in ../docker-network.yml and nothing is read from the environment.)
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

# Configuration comes from files, not the launching shell: soak.conf beside
# this script holds every knob with its default, and `-c <file>` layers a
# second file on top. Both are frozen into the run directory. The purpose
# text is the one positional argument.
conf_override=""
if [ "${1:-}" = "-c" ]; then
  conf_override="$2"; shift 2
  [ -f "$conf_override" ] || { echo "no such config file: $conf_override"; exit 1; }
fi
# shellcheck source=soak.conf
. "$here/soak.conf"
[ -n "$conf_override" ] && . "$conf_override"
# The knobs the compose file and the node containers read
export COMPOSE_PROJECT_NAME ACC_BLOCK_INTERVAL ACC_MEM_LIMIT GOMEMLIMIT ACC_TX_TRACE
# Stop a background subshell AND the `sleep` it is parked in.
#
# THE FIX, in two parts, because the leftover has two causes (#4364):
#
# 1. Three sampler loops captured no PID at all — `( while … ) &` with no
#    `$!` — so teardown's kill list could not name them. They stop only when
#    their own `while kill -0 $DRIVER` next runs, which is AFTER the sleep:
#    up to STORAGE_STATS_INTERVAL, 300s in soak.conf. Every background job
#    this script starts now records its PID and is passed here.
# 2. A plain `kill` on such a subshell is still not enough. The subshell dies
#    at once, but the `sleep` it was waiting on is its CHILD: it is orphaned,
#    reparented, and runs out its full interval. That bare `sleep 300` is the
#    process Paul killed by hand after both runs on 2026-09-19. Verified on
#    this box, two identical `( while true; do sleep 300; done ) &` jobs:
#      plain kill: subshell gone | its 'sleep 300' ALIVE
#      stop_bg   : subshell gone | its 'sleep 300' gone
#
# So the order is STOP, children, TERM, CONT — four signals, and each one is
# there for a reason (reviewer M1 on #4364):
#   STOP    freeze the subshell first. Killing the `sleep` wakes it, and it
#           CAN run its loop body once more before the TERM lands ~1ms
#           later — possible in principle, seen once in 34 trials (reviewer
#           M1 on #4364: 0 of 33; harness-engineer: 1 of 1). On the chaos
#           loop that body is `docker pause`/`docker restart` — a
#           disturbance forked AT teardown, and `compose down` on a paused
#           container. The acceptance run for this issue is a chaos run, so
#           the window is closed rather than argued about.
#   pkill -P kill the `sleep`, so it is not orphaned when the parent goes.
#   TERM    queued while the process is stopped; it is not acted on yet.
#   CONT    resume, and the queued TERM kills it before any further command.
# Nothing here blocks: no `wait`, and the script sets no `-e`.
stop_bg() {
  for p in "$@"; do
    [ -n "$p" ] || continue
    kill -STOP "$p" 2>/dev/null || true
    pkill -P "$p" 2>/dev/null || true
    kill -TERM "$p" 2>/dev/null || true
    kill -CONT "$p" 2>/dev/null || true
  done
  return 0
}

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
# CHAOS_MIN/CHAOS_JITTER come from soak.conf (sourced above, which overrides
# anything in the environment); empty there means the duration-scaled default.
if [ "$duration_seconds" -le 1800 ]; then
  CHAOS_MIN=${CHAOS_MIN:-25}; CHAOS_JITTER=${CHAOS_JITTER:-20}
else
  CHAOS_MIN=${CHAOS_MIN:-480}; CHAOS_JITTER=${CHAOS_JITTER:-240}
fi
# A 5m grace on a 5m run doubles the wall clock for no benefit; scale it.
# The overall timeout is a ceiling on the load generator's context and MUST
# exceed the duration plus bootstrap plus grace: a fixed 20m here cut every
# thirty-minute run to twenty (20260917T212457Z stopped at 1171s of 1800 with
# "waiting for delivery" and a clean exit, and the record called it done).
if [ "$duration_seconds" -le 1800 ]; then
  LG_GRACE=${LG_GRACE:-45s}; LG_TIMEOUT=${LG_TIMEOUT:-$(( duration_seconds + 900 ))s}
else
  LG_GRACE=${LG_GRACE:-5m}; LG_TIMEOUT=${LG_TIMEOUT:-26h}
fi
# The 100-sub-treasury bootstrap front-loads cross-partition funding traffic
# regardless of -tps, which keeps every channel busy. Reproducing an idle-stream
# stall (#4073) needs channels that actually go quiet, so allow it to be turned
# off. Default keeps the realistic funding spread.
LG_BOOTSTRAP=${LG_BOOTSTRAP:-100}
# CHAOS=off turns disturbance off entirely, for runs that are measuring
# throughput or footprint rather than resilience. Default is on: a soak that
# never disturbs anything is not a soak.
CHAOS_ENABLED="${CHAOS:-on}"
NOTE="${1:-}"

runs="$here/runs"
run_id="$(date -u +%Y%m%dT%H%M%SZ)"
rd="$runs/$run_id"
mkdir -p "$rd/config" || { echo "cannot create $rd"; exit 1; }
ln -sfn "$rd" "$runs/latest"

log="$rd/soak.log"; chaos="$rd/chaos.log"; mon="$rd/monitor.csv"
manifest="$rd/manifest.md"; runjson="$rd/run.json"
# This network shares a directory name ("docker") with the ASP mainnet fleet at
# core/staking/deploy/docker, so Compose derives the SAME default project name
# for both. Every `down --remove-orphans` below would then treat the running
# asp-v00* mainnet containers as orphans and delete them. Pin the project so
# teardown can only ever reach this network.
export COMPOSE_PROJECT_NAME="${COMPOSE_PROJECT_NAME:-disoak}"
# One definition of where the compose file is. It used to be spelled
# "$here/docker-compose.yml" for provenance and "$here/../docker-compose.yml"
# for the commands — the script came from synth-heal, where it sat beside the
# script. Every run therefore froze no compose file at all and reported the
# healing flags and drop patterns as their fallbacks, so the manifest said
# "no drops" whether or not drops were configured (#4126).
compose_file="$here/../docker-compose.yml"
compose="docker compose -f $compose_file"
# The follower the chaos walk adds (#4364) is in the compose's late-follower
# profile, which `down` does not activate: a container of it left up — a run
# that ended between its add and its remove, or a killed run — would outlive
# the network and hold the config volume. Removed by name, its log kept.
rm_late_followers() {   # $1 = why, for the log
  local c
  for c in $(python3 "$here/followerchaos.py" late 2>/dev/null | awk '{print $2}'); do
    docker inspect "$c" >/dev/null 2>&1 || continue
    docker logs "$c" 2>&1 | sed "s/^/$c | /" > "$rd/node-logs-$c-$1.txt"
    docker rm -f -v "$c" >/dev/null 2>&1
    echo "$(date -u +%FT%TZ) $c removed ($1)" | tee -a "$log"
  done
  return 0
}

# ---- provenance -------------------------------------------------------------
# Capture what is being tested BEFORE starting, because the tree will move on.
git_head=$(git -C "$repo" rev-parse HEAD 2>/dev/null || echo unknown)
git_desc=$(git -C "$repo" describe --tags --always --dirty 2>/dev/null || echo unknown)
git_branch=$(git -C "$repo" rev-parse --abbrev-ref HEAD 2>/dev/null || echo unknown)
# The count and the patch have to be the same set, or the manifest says
# "uncommitted files | 1" beside a zero-byte patch and a reader cannot tell
# whether the tree was dirty or the capture broke (run 20260919T231856Z).
# `status --porcelain` counts tracked changes AND untracked files; `git
# diff` captured only UNSTAGED tracked ones. So: capture `git diff HEAD`,
# which is every tracked change staged or not, and count the two kinds
# apart. An untracked file is not in any patch and the row says so.
git_tracked=$(git -C "$repo" status --porcelain --untracked-files=no 2>/dev/null | wc -l)
git_untracked=$(git -C "$repo" ls-files --others --exclude-standard 2>/dev/null | wc -l)
git_dirty=$(( git_tracked + git_untracked ))
exec_ver=$(grep -E '^\s*executorVersion:' "$here/../docker-network.yml" | head -1 | sed 's/.*: *//; s/"//g')
# From v1.4.5 healing has no configuration — the conductor always heals. Older
# trees injected enable-*-healing into accumulate.toml, so keep reading it: a
# run against an older image must still record what it was configured with.
# Strip comments first: the compose has an explanatory header mentioning
# "enable-synthetic-healing = true", and matching that made a v1.4.5 run — which
# has no healing config at all — report the flag as set. A provenance record
# that quietly reports the opposite of the truth is worse than none.
heal_flags=$(sed 's/#.*//' "$compose_file" \
  | grep -oE 'enable-[a-z-]*healing = [a-z]+' | sort -u | paste -sd'; ' -)
heal_flags="${heal_flags:-unconditional (DI conductor, #4105)}"
# The fault model, stated once for the manifest and run.json. There are no
# drop hooks in the node (nothing reads ACC_DEBUG_DROP_*; git log -S finds the
# name only in old manifests), so the only honest statement is the disturbance
# cadence -- or "none".
if [ "$CHAOS_ENABLED" = off ]; then
  fault_model="none (CHAOS=off)"
else
  fault_kinds="restart or pause one BVN validator container"
  [ "${CHAOS_VALIDATORS:-on}" = off ] && fault_kinds=""
  if [ "${CHAOS_FOLLOWERS:-off}" = on ]; then
    fault_kinds="${fault_kinds:+$fault_kinds, alternating with }add-follower then remove-follower (CHAOS_FOLLOWER_CYCLES=${CHAOS_FOLLOWER_CYCLES:-0}, 0 unbounded)"
  fi
  fault_model="${fault_kinds:-nothing (CHAOS_VALIDATORS=off, CHAOS_FOLLOWERS=off)} every ${CHAOS_MIN}s + 0-${CHAOS_JITTER}s"
fi
# Compose names built images "<project>-<service>", and the project is pinned to
# $COMPOSE_PROJECT_NAME above. This default was "docker-bvn1-val1", the name
# from BEFORE the project was pinned (#4124) — so from that commit onward every
# manifest recorded the id of a stale leftover image while the network ran the
# freshly built one. The 2026-08-24 runs all reported an image built 2026-08-20.
# That is the #4103 failure in its purest form: provenance that names the wrong
# build makes every conclusion drawn from the run unattributable.
soak_image="${SOAK_IMAGE:-${COMPOSE_PROJECT_NAME}-bvn1-val1}"
image_id=$(docker image inspect --format '{{.Id}}' "$soak_image" 2>/dev/null || echo unknown)
if [ "$image_id" = unknown ]; then
  # Do not record "unknown" and carry on: an unidentifiable build is a run
  # nobody can reproduce or attribute, which is the one thing this file exists
  # to prevent. Named here, it costs a second; found later, it costs the run.
  echo "cannot identify the image \"$soak_image\" — refusing to run unattributable." | tee -a "$log"
  echo "  (compose builds <project>-<service>; project is \"$COMPOSE_PROJECT_NAME\". Set SOAK_IMAGE to override.)" | tee -a "$log"
  exit 1
fi
n_bvn=$(grep -cE '^\s*- id: "BVN' "$here/../docker-network.yml")
n_node=$(grep -cE '^\s*- listenAddress:' "$here/../docker-network.yml")

# Validators and followers, separately (#4365). They are different nodes with
# different roles in the measurement — a follower is never handed load and
# never disturbed — and a manifest that says "13 nodes" tells a reader
# neither how big the committees were nor that one node was not in them.
#
# The followers THIS RUN HAS: the ones `up` starts, and the late one only
# when the add-follower walk is on (CHAOS_FOLLOWERS=on) — never a follower
# that is merely declared. Counting every declared follower gave run
# 20260924T052134Z "2 follower" and 14 nodes for a network of 13, the second
# never started (#4389). A late follower is named as such, because it is not
# up from the start and the rows that read it say so.
read -r n_val n_fol FOL_LIST FOL_PORTS FOL_PARTS n_late <<<"$(python3 -c '
import sys
sys.path.insert(0, sys.argv[1])
import topology
f, late = topology.run_followers(sys.argv[2] == "on")
print(len(topology.validator_records()), len(f),
      ",".join(x["container"] for x in f) or "-",
      ",".join(str(x["port"]) for x in f) or "-",
      ";".join("/".join(x["partitions"]) for x in f) or "-", len(late))' "$here/.." \
      "$([ "$CHAOS_ENABLED" != off ] && echo "${CHAOS_FOLLOWERS:-off}" || echo off)" 2>/dev/null)"
n_val=${n_val:-$n_node}; n_fol=${n_fol:-0}; n_late=${n_late:-0}
FOL_LIST=${FOL_LIST:--}; FOL_PORTS=${FOL_PORTS:--}; FOL_PARTS=${FOL_PARTS:--}
# Nodes this run has, not nodes the file declares.
n_node=$(( n_val + n_fol ))
if [ "$n_fol" -gt 0 ]; then
  topo_desc="$n_bvn BVNs, $n_val validators + $n_fol follower ($FOL_LIST, partitions ${FOL_PARTS//\// })$([ "$n_late" -gt 0 ] && echo ", of which the last $n_late is started only by the add-follower disturbance") + bootstrap"
else
  topo_desc="$n_bvn BVNs, $n_val validators + bootstrap"
fi

# The partition list, derived once from docker-network.yml and reused by the
# monitor loop below. Everything that needs to know the shape of this network
# reads that one file (see ../topology.py); nothing restates it.
PARTS=$(python3 -c '
import json, sys
sys.path.insert(0, sys.argv[1])
import topology
print(" ".join(topology.partitions()))' "$here/.." 2>/dev/null)
# And verify the two files that jointly define the topology still agree: the
# host ports are a convention of docker-compose.yml derived from the node order
# in docker-network.yml, and a convention that has drifted is a monitor and a
# loadgen quietly pointed at ports nothing serves. Fail here, not at hour six.
# `problems()` refuses the shapes this harness cannot measure rather than
# mis-measuring them: a follower declared before a validator (it takes that
# validator's directory AND its host port), a node that is a validator on one
# partition and a follower on the other, a peerAddress that disagrees with the
# container name the compose uses.
topo_problem=$(python3 -c '
import sys
sys.path.insert(0, sys.argv[1])
import topology
print("; ".join([topology.check_ports_against_compose() or ""] + topology.problems()).strip("; "))' "$here/.." 2>&1)
if [ -z "$PARTS" ] || [ -n "$topo_problem" ]; then
  echo "topology preflight failed: ${topo_problem:-cannot read docker-network.yml}" | tee -a "$log"
  exit 1
fi

# Freeze the exact config. A diff against these is the only reliable way to know
# what changed between two runs.
cp "$compose_file" "$here/../docker-network.yml" "$0" "$here/soak.conf" "$rd/config/" 2>/dev/null
[ -n "$conf_override" ] && cp "$conf_override" "$rd/config/override.conf" 2>/dev/null
git -C "$repo" diff HEAD > "$rd/config/uncommitted.patch" 2>/dev/null

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
  echo "| uncommitted files | $(
    if [ "$git_dirty" -eq 0 ]; then echo 0
    else
      printf '%s: %s tracked' "$git_dirty" "$git_tracked"
      [ "$git_tracked" -gt 0 ] && printf ' (in `config/uncommitted.patch`)'
      [ "$git_untracked" -gt 0 ] && printf ', %s untracked (in no patch)' "$git_untracked"
      if [ "$git_tracked" -gt 0 ] && [ ! -s "$rd/config/uncommitted.patch" ]; then
        printf ' — WARNING: the patch is empty though %s tracked files differ; the capture failed' "$git_tracked"
      fi
    fi) |"
  echo "| image | \`$soak_image\` |"
  echo "| image id | \`$image_id\` |"
  echo "| executor version | **$exec_ver** |"
  echo "| healing | $heal_flags |"
  echo "| fault model | $fault_model |"
  echo "| topology | $topo_desc |"
  if [ "$n_fol" -gt 0 ]; then
    # Which form of "in no committee" ran. The key is IN the definition and
    # inactive, because `init network` writes one key and one config per node
    # listed in docker-network.yml — a key that is in no definition is a node
    # with no config, which is a Go change and not this harness's (#4365).
    # Filled in from the network's own NetworkDefinition once it is up.
    echo "| follower key | pending (read from network-status before load) |"
  fi
  echo "| partitions | $PARTS |"
  echo "| chaos | $CHAOS_ENABLED |"
  echo "| target duration | $DURATION |"
  echo "| target TPS | $TPS |"
  # `[a-z]+` matched nothing against `database: BlockchainDB` and the row
  # went out BLANK on run 20260919T231856Z — a provenance field that says
  # nothing about a run that was, verifiably from storage-stats.csv, on
  # BlockchainDB. The backend's name is mixed case and always has been
  # (#4165); match what the file can hold, and say so when it holds
  # nothing rather than printing an empty cell.
  echo "| storage | $(sed -nE 's/^database: *([A-Za-z0-9_.-]+).*/\1/p' "$here/../docker-network.yml" | head -1 | grep . || echo '— not measured (no `database:` line in docker-network.yml)') (docker-network.yml) |"
  echo "| block interval | ${ACC_BLOCK_INTERVAL:-1s} |"
  echo "| memory budget | mem_limit ${ACC_MEM_LIMIT:-1536m}, GOMEMLIMIT ${GOMEMLIMIT:-1200MiB} |"
  echo
  echo "Config as run is frozen in \`config/\` (soak.conf${conf_override:+ + override.conf}, the compose and network files). Results appended below on exit."
} > "$manifest"

printf '{"runId":"%s","startedUtc":"%s","image":"%s","imageId":"%s","commit":"%s","describe":"%s","branch":"%s","uncommittedFiles":%s,"uncommittedTracked":%s,"uncommittedUntracked":%s,"executorVersion":"%s","healing":"%s","faultModel":"%s","bvns":%s,"nodes":%s,"validators":%s,"followers":%s,"followerContainers":"%s","followerPorts":"%s","followerPartitions":"%s","followerKeyForm":"pending","partitions":"%s","chaos":"%s","duration":"%s","tps":"%s","note":"%s"}\n' \
  "$run_id" "$(date -u +%FT%TZ)" "$soak_image" "$image_id" "$git_head" "$git_desc" "$git_branch" "$git_dirty" "$git_tracked" "$git_untracked" \
  "$exec_ver" "$heal_flags" "$fault_model" "$n_bvn" "$n_node" "$n_val" "$n_fol" \
  "$FOL_LIST" "$FOL_PORTS" "$FOL_PARTS" \
  "$PARTS" "$CHAOS_ENABLED" "$DURATION" "$TPS" "$NOTE" > "$runjson"

echo "== soak start $(date -u) duration=$DURATION tps=$TPS ==" | tee "$log"
echo "   run dir: $rd" | tee -a "$log"
echo "   commit:  $git_desc ($git_head)" | tee -a "$log"
echo "   image:   $soak_image ($image_id)" | tee -a "$log"
echo "   version: $exec_ver | healing: $heal_flags" | tee -a "$log"

# ONE soak at a time. Every run shares the compose project, so a second
# launch's `down -v` below destroys the first run's network and replaces it
# with its own — which is exactly what happened to 20260829T141021Z. Refuse,
# loudly, while another soak's driver is alive or its containers are up; the
# operator decides which run survives.
# A pidfile, not pgrep: every pgrep-based test matched the launcher's own
# wrapper shells or this script's own forks (three refusals in a row).
pidfile=/tmp/disoak-soak.pid
other=$(cat "$pidfile" 2>/dev/null)
if [ -n "$other" ] && [ "$other" != "$$" ] && kill -0 "$other" 2>/dev/null \
   && grep -q "soak.sh" "/proc/$other/cmdline" 2>/dev/null; then
  echo "another soak is running (pid $other) — refusing to start." | tee -a "$log"
  echo "  stop it first, or SOAK_FORCE=1 in a -c override file to take the network over deliberately (the environment is not read)." | tee -a "$log"
  [ "${SOAK_FORCE:-0}" = 1 ] || exit 1
fi
live=$(docker ps --format '{{.Names}}' 2>/dev/null | grep -c '^acc-')
if [ "$live" -gt 0 ] && [ "${SOAK_FORCE:-0}" != 1 ]; then
  echo "$live acc-* containers are up from something else — refusing to start (SOAK_FORCE=1 in a -c override file to take over; the environment is not read)." | tee -a "$log"
  exit 1
fi
echo $$ > "$pidfile"
rm_late_followers "left over before this run"
$compose down -v --remove-orphans >/dev/null 2>&1

# Preflight the host ports the compose publishes. A single stray process on one
# of them makes `up` fail on ONLY that node — the rest come up, so the failure
# looked like a random "up failed" and left a partial network behind (#4158).
# A leaked `accumulated run devnet` squatting on 26680 cost an afternoon; name
# the holder so the next person spends a second, not an afternoon.
mapfile -t want_ports < <(grep -oE '"\s*[0-9]+\s*:\s*[0-9]+"|- [0-9]+:[0-9]+' "$compose_file" \
  | grep -oE '[0-9]+:' | tr -d ':' | sort -un)
port_conflict=0
for p in "${want_ports[@]}"; do
  holder=$(ss -ltnHp "sport = :$p" 2>/dev/null | grep -oE 'pid=[0-9]+' | head -1 | cut -d= -f2)
  if [ -n "$holder" ]; then
    echo "port $p is already held by pid $holder ($(ps -o args= -p "$holder" 2>/dev/null | cut -c1-80))" | tee -a "$log"
    port_conflict=1
  fi
done
[ "$port_conflict" -eq 0 ] || { echo "refusing to start: free the port(s) above and retry (#4158)" | tee -a "$log"; exit 1; }

# Build BEFORE up. `up -d` reuses an existing image silently, and every
# conclusion drawn from such a run is about the wrong build (#4103).
#
# The build names the late-follower profile. With no profile compose skips the
# services in one, so the follower add-follower starts (#4364) was never built
# from this tree: it ran whatever <project>-<its service> image an earlier run
# left, or on a fresh box one built mid-chaos with its output thrown away —
# the node whose join the run is there to prove, on a different binary from
# the validators it joins.
$compose --profile late-follower build >/dev/null 2>&1 || { echo "compose build failed" | tee -a "$log"; exit 1; }
# Its image id goes in the manifest beside the validators', read AFTER the
# build, and is frozen in config/. A run that will add the follower refuses an
# image it cannot name, for the reason the validators' check above gives; a
# run that adds none says unknown and goes on.
while read -r lf_svc lf_c _; do
  [ -n "$lf_svc" ] || continue
  lf_image="${COMPOSE_PROJECT_NAME}-$lf_svc"
  lf_id=$(docker image inspect --format '{{.Id}}' "$lf_image" 2>/dev/null | awk 'NR==1{print $1}')
  lf_id=${lf_id:-unknown}
  sed -i "/^| image id | /a | late follower image id ($lf_c) | \`$lf_image\` \`$lf_id\` (after the build) |" "$manifest"
  echo "$lf_id $lf_image $lf_c" >> "$rd/config/image-late-follower.txt"
  echo "   late follower image: $lf_image ($lf_id), container $lf_c" | tee -a "$log"
  if [ "$lf_id" = unknown ] && [ "${CHAOS_FOLLOWERS:-off}" = on ]; then
    echo "cannot identify the image \"$lf_image\" that add-follower would start — refusing to run unattributable." | tee -a "$log"
    echo "  (built by \`compose --profile late-follower build\`; if compose did not build it, the profile is not doing what this script assumes.)" | tee -a "$log"
    exit 1
  fi
done < <(python3 "$here/followerchaos.py" late 2>/dev/null)
# Surface the up error (a swallowed one hid the port conflict of #4158), and
# on ANY failure tear the project down before exiting — a failed `up` leaves
# the containers it already started running, i.e. an UNMONITORED network, which
# is exactly what must never linger. The project is pinned to $COMPOSE_PROJECT_NAME
# so this teardown can only ever reach this soak, never the asp-* mainnet fleet.
if ! $compose up -d >>"$log" 2>&1; then
  echo "up failed — see the error above; tearing down so nothing runs unmonitored" | tee -a "$log"
  $compose down -v --remove-orphans >/dev/null 2>&1
  exit 1
fi

# The manifest's memory line above prints this script's defaults, which are
# not compose's: run 20260903T121819Z recorded 1536m/1200MiB and ran at
# 2048m/1700MiB. Replace it with what the containers actually got (PLAN S0/S6).
eff_c=$(docker ps --format '{{.Names}}' | grep -E '^acc-bvn' | head -1)
if [ -n "$eff_c" ]; then
  eff_mem=$(docker inspect -f '{{.HostConfig.Memory}}' "$eff_c" 2>/dev/null)
  eff_gml=$(docker inspect -f '{{range .Config.Env}}{{println .}}{{end}}' "$eff_c" 2>/dev/null | sed -n 's/^GOMEMLIMIT=//p' | head -1)
  eff_mem_h=$([ -n "$eff_mem" ] && [ "$eff_mem" -gt 0 ] 2>/dev/null && echo "$((eff_mem / 1048576))MiB" || echo "unlimited")
  sed -i "s#^| memory budget | .*#| memory budget | mem_limit ${eff_mem_h}, GOMEMLIMIT ${eff_gml:-unset} (effective, from docker inspect) |#" "$manifest"
  echo "$(date -u +%FT%TZ) effective memory budget: mem_limit ${eff_mem_h}, GOMEMLIMIT ${eff_gml:-unset}" | tee -a "$log"
fi

# Record the image actually running, so a rebuild later cannot be confused for this run.
docker image inspect --format '{{.Id}} {{.RepoTags}}' "$soak_image" \
  > "$rd/config/image.txt" 2>/dev/null

# Rotation-proof log capture, from the first block onward. The containers use
# bounded logging, and the 20260819T234054Z post-mortem lost its first three
# hours to rotation — the onset of the collapse was undatable because the only
# capture ran once at teardown and inherited whatever rotation had left.
# Streaming into the run dir preserves everything; the file is large under
# failure storms (gigabytes) and is gitignored — summarize, don't commit it.
nohup docker compose -f "$here/../docker-compose.yml" logs -f --no-color \
  >> "$rd/node-logs-live.txt" 2>&1 &
LOGCAP=$!

up=""; for _ in $(seq 1 90); do
  curl -sf -X POST http://localhost:26680/v3 -H 'content-type: application/json' \
    -d '{"jsonrpc":"2.0","id":1,"method":"network-status","params":{"partition":"Directory"}}' >/dev/null 2>&1 && { up=1; break; }
  sleep 5
done
[ -n "$up" ] || { echo "network never came up" | tee -a "$log"; exit 1; }
sleep 30

# The NetworkDefinition, captured BEFORE load starts (#4365). It is the
# authority on which keys are in the definition and which of them are active
# on which partition, and the DAG-BFT committee is built from the active ones
# only (run/dagbft.go:415) — so a key with no active partition is exactly a
# node that is in the definition and in no committee. Captured now, not at
# teardown, because it is the state the run was launched with.
curl -s -m 10 -X POST http://localhost:26680/v3 -H 'content-type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"network-status","params":{"partition":"Directory"}}' \
  | python3 -c 'import json,sys; json.dump(json.load(sys.stdin).get("result") or {}, sys.stdout)' \
  > "$rd/network-definition.json" 2>/dev/null
if [ "$n_fol" -gt 0 ]; then
  # The follower's OWN key, looked up in the capture (M5) — not "is there
  # any inactive entry", which answers "absent from the definition", the
  # reassuring form, when the premise of the gate has failed.
  #
  # stderr goes to the run log, NOT to /dev/null (L2): an import error or a
  # changed JSON shape used to be reported as "network-status did not
  # answer", which is a different fault with a different fix.
  key_form=$(python3 -c '
import json, sys
sys.path.insert(0, sys.argv[2])
import followerlog
try:
    d = json.load(open(sys.argv[1]))
except Exception as e:
    sys.stderr.write("network-definition.json unreadable: %r\n" % (e,))
    d = None
# The key prefix comes from the follower own identity line, which it logs
# at startup; the network has been up for 30 s, so it is there.
try:
    with open(sys.argv[3], errors="replace") as f:
        pre = followerlog.read(f).key_prefix(sys.argv[4])
except Exception as e:
    sys.stderr.write("cannot read the follower key prefix: %r\n" % (e,))
    pre = None
v = followerlog.definition_check(d, pre)
form = v["followerKeyForm"]
if not form:
    print("— not measured (%s)" % (v["why"] or "unknown")); raise SystemExit
print("%s; active validators per partition: %s" % (
    form, ", ".join("%s %d" % kv for kv in sorted(v["active"].items()))))' \
    "$rd/network-definition.json" "$here" "$rd/node-logs-live.txt" "${FOL_LIST%%,*}" \
    2>>"$log")
  key_form=${key_form:-— not measured (the key-form capture produced nothing; see soak.log)}
  if grep -q '^| follower key | ' "$manifest"; then
    sed -i "s#^| follower key | .*#| follower key | ${key_form} |#" "$manifest"
    grep -q "^| follower key | ${key_form} |" "$manifest" \
      || echo "WARNING: the manifest's follower-key row was not replaced — it still reads \"pending\"" | tee -a "$log"
  else
    echo "WARNING: no follower-key row in the manifest to fill in" | tee -a "$log"
  fi
  python3 -c 'import json,sys; p=sys.argv[1]; d=json.load(open(p)); d["followerKeyForm"]=sys.argv[2]; json.dump(d, open(p,"w"))' \
    "$runjson" "$key_form" 2>>"$log"
  echo "$(date -u +%FT%TZ) follower key: $key_form" | tee -a "$log"
fi

# Observability FIRST, and it is a gate, not a hope. The monitor comes up
# before any load exists, and if it does not come up the run DOES NOT HAPPEN —
# the network is torn down and the script exits nonzero. The requirement is
# that a test is watched; a warning that continues unmonitored is exactly the
# behaviour that produced five unobserved runs during the #4103 diagnosis.
if [ ! -x "$here/soakmon.py" ]; then
  echo "soakmon.py missing or not executable — refusing to run unmonitored" | tee -a "$log"
  $compose down -v --remove-orphans >/dev/null 2>&1
  exit 1
fi
# RUN_DIR so the dashboard reads THIS run's loadgen stats and chaos log.
#
# Supervised, not launched-and-hoped-for. soakmon died mid-run twice on
# 2026-08-22 (runs 20260822T052535Z and 20260822T053653Z) and the gate below
# could not help: it is a STARTUP check, so the first run carried on generating
# load unobserved for eight minutes. stallkill now stops a blind run, but that
# costs the whole run for what may be a momentary loss. Restart it instead, and
# record every exit in the log so a repeating death is visible rather than
# silently papered over.
# A monitor from an earlier run that outlived its teardown holds the port,
# answers the gate below, and feeds every watcher a dead run's data: run
# 20260903T222843Z ran 22 minutes with no mem.csv and stallkill reading run
# 213153Z. A monitor that is not ours is a reason to stop, not to proceed.
if stale=$(pgrep -f "$here/soakmon.py" 2>/dev/null) && [ -n "$stale" ]; then
  echo "another soakmon is running (pid $stale) — an earlier run's monitor outlived its teardown; kill it and retry. Refusing to run against someone else's dashboard." | tee -a "$log"
  $compose down -v --remove-orphans >/dev/null 2>&1
  exit 1
fi
( while kill -0 $$ 2>/dev/null; do
    env RUN_DIR="$rd" REJOIN_MAX_BEHIND="${REJOIN_MAX_BEHIND:-10}" "$here/soakmon.py" >> "$rd/soakmon.log" 2>&1
    echo "$(date -u +%FT%TZ) soakmon exited rc=$? — restarting" >> "$rd/soakmon.log"
    sleep 2
  done ) &
MON=$!
for _ in $(seq 1 20); do
  curl -sf -m3 http://127.0.0.1:8099/data >/dev/null 2>&1 && break
  sleep 3
done
if ! curl -sf -m3 http://127.0.0.1:8099/data >/dev/null 2>&1; then
  echo "soakmon did not come up — refusing to run unmonitored; tearing down" | tee -a "$log"
  pkill -P "$MON" 2>/dev/null; kill "$MON" 2>/dev/null
  $compose down -v --remove-orphans >/dev/null 2>&1
  exit 1
fi
echo "   soakmon: http://127.0.0.1:8099 (gate passed)" | tee -a "$log"

# SHOW the dashboard, do not merely print its URL. The requirement is that a
# run is watched, and a localhost address in a log the operator has to notice,
# copy and paste is not being watched — five unobserved runs during the #4103
# diagnosis all had a printed URL. Best effort: a headless or remote invocation
# has no browser and must still be able to run.
if [ -z "${NO_OPEN:-}" ] && command -v xdg-open >/dev/null 2>&1; then
  (xdg-open "http://127.0.0.1:8099" >/dev/null 2>&1 &) 
  echo "   dashboard opened in the browser" | tee -a "$log"
fi

# Wedge watchdog. #4125 froze block production on all four partitions with
# consensus healthy, and was torn down before anyone took a goroutine dump —
# the single artifact that says whether the executor is parked in batch
# collection. This dumps every node the moment soakmon reports a stalled
# partition, and never touches the network itself.
# Read-back probe: samples committed entries as the run goes and re-reads them
# on a schedule, timing each read and recording the slowest per round with
# the entry's age. Report in readprobe-report.md at teardown.
if [ -x "$here/readprobe.py" ]; then
  nohup env RUN_DIR="$rd" "$here/readprobe.py" > "$rd/readprobe.log" 2>&1 &
  READPROBE=$!
  echo "   readprobe: armed (sample every ${PROBE_SAMPLE_EVERY:-20}s, re-read every ${PROBE_EVERY:-60}s)" | tee -a "$log"
fi

if [ -x "$here/wedgewatch.sh" ]; then
  nohup env RUN_DIR="$rd" "$here/wedgewatch.sh" > "$rd/wedgewatch.log" 2>&1 &
  WEDGE=$!
  echo "   wedgewatch: armed (dump after ${WEDGE_SECS:-120}s stalled)" | tee -a "$log"
else
  echo "   wedgewatch: MISSING — a wedge will go undiagnosed again (#4125)" | tee -a "$log"
fi
# Stop the run once a stall outlives its usefulness. Run 20260822T015342Z spent
# two hours with a dead Directory and three unrecoverable validators after it
# had already produced every piece of evidence it was going to; the remaining
# hours would only have written the same warning at 137MB/hour. Evidence is
# captured before it stops, and it stops the run the clean way — signal the
# loadgen, let this script write its verdict, then take the network down.
# STALL_KILL_SECS=0 disables it for a run that is meant to sit in a stall.
if [ -x "$here/stallkill.sh" ] && [ "${STALL_KILL_SECS:-240}" != "0" ]; then
  nohup env RUN_DIR="$rd" STALL_KILL_SECS="${STALL_KILL_SECS:-240}" SOAK_PID=$$ \
    "$here/stallkill.sh" > "$rd/stallkill.log" 2>&1 &
  STALLKILL=$!
  echo "   stallkill: armed (stop the run after ${STALL_KILL_SECS:-240}s stalled)" | tee -a "$log"
fi
echo "   load starts now" | tee -a "$log"

# Load generator (host): drives the full menu of user transaction types against
# an ever-growing account set. -faucet-seed FAUCET matches init's genesis faucet.
# Rotate across all 12 nodes so one chaos-disrupted node neither rejects traffic
# nor carries the whole load.
# Endpoints come from the topology, not a literal port range. `seq 26680 26691`
# was correct for exactly one network shape; after the cut to 2 BVNs it would
# have handed the loadgen four endpoints nothing is listening on. The generator
# does not fail on those — it rotates onto them and the submissions time out,
# so the only symptom is a third of the target rate going missing, which is
# indistinguishable from the network being unable to keep up. That is the exact
# question these runs exist to answer, so it must not be corrupted here.
EPS=$(python3 -c '
import json, sys
sys.path.insert(0, sys.argv[1])
import topology
print(",".join("http://localhost:%d" % p for p in topology.node_ports()))' "$here/.." 2>/dev/null)
if [ -z "$EPS" ]; then
  echo "cannot derive loadgen endpoints from the topology — refusing to run blind" | tee -a "$log"
  $compose down -v --remove-orphans >/dev/null 2>&1
  exit 1
fi
# The control API steers the running generator — rate and mix — without a
# restart (a restart re-bootstraps the account universe):
#   curl http://127.0.0.1:${LG_CONTROL_PORT:-8091}/control
#   curl -X POST -d '{"tps": 10}' http://127.0.0.1:${LG_CONTROL_PORT:-8091}/control
#   curl -X POST -d '{"mix": {"burn-tokens": 0}}' ...  (weight 0 disables)
LG_CONTROL_PORT="${LG_CONTROL_PORT:-8091}"
nohup go run "$repo/tools/cmd/loadgen" -endpoints "$EPS" \
  -faucet-seed FAUCET -tps "$TPS" -duration "$DURATION" -timeout "$LG_TIMEOUT" \
  -bootstrap "$LG_BOOTSTRAP" -control "127.0.0.1:$LG_CONTROL_PORT" \
  -submitters "${LG_SUBMITTERS:-64}" \
  -grace "$LG_GRACE" -max-stranded 20 -stats-file "$rd/loadgen-stats.json" >> "$log" 2>&1 &
DRIVER=$!
echo "   loadgen control API: http://127.0.0.1:$LG_CONTROL_PORT/control (POST {\"tps\": N} / {\"mix\": {...}})" | tee -a "$log"

if [ -x "$here/seizewatch.sh" ]; then
  nohup "$here/seizewatch.sh" > "$rd/seizewatch.out" 2>&1 &
  SEIZE=$!
fi

# Chaos: every ~10 min disturb ONE random node (quorum 3/4 preserved).
# Full ISO dates — time-of-day alone cannot be attributed to a run.
#
# Say so the moment it arms. An armed-but-sleeping chaos loop produced no
# file and no events for its whole first interval, which is indistinguishable
# from a broken one — and was reported as broken (run 20260824T051249Z, first
# interval 672s). Silence must never look like breakage.
if [ "$CHAOS_ENABLED" = off ]; then
  # A throughput measurement and a resilience measurement are different runs.
  # Chaos restarts and pauses move the achieved rate by more than the effects
  # being measured when the question is "where is the rate knee", so it gets a
  # real switch. Earlier probes did this by setting CHAOS_MIN=86400, which left
  # the log saying "armed: one disturbance every 86400s" — technically true,
  # and read by the next person as chaos having been on.
  echo "$(date -u +%FT%TZ) DISABLED for this run (CHAOS=off)" >> "$chaos"
  echo "   chaos: DISABLED (CHAOS=off) — this is a throughput run, not a resilience run" | tee -a "$log"
  CHAOS=""
else
echo "$(date -u +%FT%TZ) armed: one disturbance every ${CHAOS_MIN}s + 0-${CHAOS_JITTER}s jitter" >> "$chaos"
echo "   chaos: armed (every ~${CHAOS_MIN}s + jitter; first event follows the first interval)" | tee -a "$log"
( end=$(( $(date +%s) + duration_seconds ))
  # Every validator in turn, interleaved by partition -- bvn1-val1, bvn2-val1,
  # bvn3-val1, bvn1-val2, ... -- so consecutive disturbances land on different
  # partitions and every node gets its turn before any node gets a second. A
  # random draw with replacement put three of four disturbances on BVN3 in
  # the first hour of 20260917T223150Z and none on BVN1; Paul: "that isn't
  # testing very much". Every container runs a DN node as well as its BVN
  # node, so the
  # Directory is disturbed by every event. The kind alternates restart, pause,
  # and flips on each full cycle so a node sees both over a run. A no-op slot
  # is CHAOS_SKIP_ONE_IN=N (every Nth slot; 0, the default, never): it used to
  # be a hidden one-in-five draw, which cost 20260917T212457Z three of its
  # five slots.
  # NOT COVERED BY A TEST (reviewer L3, accepted for this CHAOS=off run and
  # carried to #4364): this intersection and the heals split below are bash,
  # and nothing exercises them. Harmless here — chaos is off — and
  # load-bearing the moment #4364 turns it on, which is where they get a
  # test.
  #
  # VALIDATORS only. `--filter name=acc-bvn` also matches `acc-bvn3-fol1`,
  # and a follower restarted by chaos is the node the run is measuring being
  # disturbed by the run (#4365). The roster is intersected with the
  # topology's validator list rather than filtered by a substring, so a
  # future naming change cannot quietly put a follower back in it.
  mapfile -t vals < <(python3 -c '
import sys
sys.path.insert(0, sys.argv[1])
import topology
print("\n".join(topology.validator_containers()))' "$here/.." 2>/dev/null)
  mapfile -t nodes < <(docker ps --filter name=acc-bvn --format '{{.Names}}' \
    | grep -Fxf <(printf '%s\n' "${vals[@]}") \
    | awk -F- '{print $3, $2, $0}' | sort -k1,1 -k2,2 | awk '{print $3}')
  val_on=1; [ "${CHAOS_VALIDATORS:-on}" = off ] && val_on=0
  if [ "$val_on" -eq 1 ] && [ "${#nodes[@]}" -eq 0 ]; then
    echo "$(date -u +%FT%TZ) NO validator containers matched the topology — chaos is doing nothing to validators" >> "$chaos"
    val_on=0
  fi
  [ "$val_on" -eq 1 ] && echo "$(date -u +%FT%TZ) order: ${nodes[*]}" >> "$chaos"
  [ "$val_on" -eq 0 ] && echo "$(date -u +%FT%TZ) validators: not disturbed (CHAOS_VALIDATORS=${CHAOS_VALIDATORS:-on})" >> "$chaos"

  # The follower kinds (#4364), when CHAOS_FOLLOWERS=on: add-follower starts
  # the follower whose compose service is in the `late-follower` profile — a
  # node `compose up` did not start, with its own key in no committee, its
  # peers from the bootstrap and its storage from docker-network.yml — and
  # remove-follower stops and removes it. They alternate, one follower up at
  # a time, CHAOS_FOLLOWER_CYCLES pairs (0: for the whole run). With the
  # validator walk on too, the slots alternate validator, follower, so each
  # kind keeps its turn.
  #
  # Each add clears the follower's two databases first (bvnN-M/{dnn,bvnn}/
  # data/accumulate.db, in the shared config volume that outlives the
  # container), so every add is a node that has never run, not a restart.
  #
  # While it is up, every wait below polls it: the chaos log gets a line when
  # accumulate_node_state reads ACTIVE on every partition it runs, and one at
  # its first block whose root equals a validator's, each with the seconds
  # since the add. The removal is bracketed by readings FOLLOWER_WINDOW_SECS
  # before and after it, and the log states whether every partition's block
  # cadence and every stream's delivered count were unaffected. The readings
  # are followerchaos.py's; it is tested there.
  fol_on=0; fol_svc=""; fol=""; fol_dir=""
  if [ "${CHAOS_FOLLOWERS:-off}" = on ]; then
    read -r fol_svc fol fol_dir < <(python3 "$here/followerchaos.py" late 2>/dev/null | head -1)
    if [ -n "$fol" ]; then
      fol_on=1
      echo "$(date -u +%FT%TZ) followers: $fol (service $fol_svc, data $fol_dir) is added and removed in turn, ${CHAOS_FOLLOWER_CYCLES:-0} pair(s) (0: unbounded)" >> "$chaos"
    else
      echo "$(date -u +%FT%TZ) followers: CHAOS_FOLLOWERS=on but no follower is in the compose's late-follower profile — none will be added" >> "$chaos"
    fi
  fi
  added=""; added_at=""; added_s=0; seen_active=0; seen_match=0; pairs=0; fol_done=0

  # Poll the added follower once: ACTIVE, then its first root match.
  follower_watch() {
    [ -n "$added" ] || return 0
    local out
    if [ "$seen_active" -eq 0 ]; then
      if out=$(python3 "$here/followerchaos.py" state "$added" 2>/dev/null); then
        seen_active=1
        echo "$(date -u +%FT%TZ) follower $added ACTIVE ($out), $(( $(date +%s) - added_s ))s after it was added" >> "$chaos"
      fi
    fi
    if [ "$seen_match" -eq 0 ]; then
      if out=$(python3 "$here/followerchaos.py" rootmatch "$added" "$added_at" 2>/dev/null); then
        seen_match=1
        echo "$(date -u +%FT%TZ) follower $added first root match ($out), $(( $(date +%s) - added_s ))s after it was added" >> "$chaos"
      fi
    fi
  }
  # Wait $1 seconds, polling the follower every 10.
  chaos_wait() {
    local left=$1 step
    while [ "$left" -gt 0 ]; do
      step=$(( left < 10 ? left : 10 ))
      sleep "$step"; left=$(( left - step ))
      follower_watch
    done
  }
  add_follower() {
    $compose run --rm --no-deps --entrypoint sh "$fol_svc" -c \
      "rm -rf /root/.accumulate/$fol_dir/dnn/data/accumulate.db /root/.accumulate/$fol_dir/bvnn/data/accumulate.db" >/dev/null 2>&1
    $compose --profile late-follower create --no-recreate "$fol_svc" >/dev/null 2>&1
    added_at=$(date -u +%FT%TZ); added_s=$(date +%s)
    echo "$added_at add-follower $fol (key in no committee; databases cleared)" >> "$chaos"
    docker start "$fol" >/dev/null 2>&1
    added=$fol; seen_active=0; seen_match=0
  }
  remove_follower() {
    local n=$((pairs + 1)) w=${FOLLOWER_WINDOW_SECS:-30}
    local pre="$rd/follower-removal-$n"
    python3 "$here/followerchaos.py" snapshot "$pre-before.json" 2>/dev/null
    chaos_wait "$w"
    python3 "$here/followerchaos.py" snapshot "$pre-at.json" 2>/dev/null
    [ "$seen_active" -eq 0 ] && echo "$(date -u +%FT%TZ) follower $added NEVER ACTIVE: removed $(( $(date +%s) - added_s ))s after it was added" >> "$chaos"
    [ "$seen_match" -eq 0 ] && echo "$(date -u +%FT%TZ) follower $added never matched a validator's root before its removal" >> "$chaos"
    echo "$(date -u +%FT%TZ) remove-follower $added" >> "$chaos"
    docker stop "$added" >/dev/null 2>&1
    # Its log goes with the container, and the run's live log capture only
    # follows the containers `up` started: keep it, in followerlog's shape.
    docker logs "$added" 2>&1 | sed "s/^/$added | /" > "$rd/node-logs-$added-$n.txt"
    docker rm -v "$added" >/dev/null 2>&1
    added=""; pairs=$n
    sleep "$w"
    python3 "$here/followerchaos.py" snapshot "$pre-after.json" 2>/dev/null
    echo "$(date -u +%FT%TZ) follower removal $n, ${w}s either side: $(python3 "$here/followerchaos.py" unaffected "$pre-before.json" "$pre-at.json" "$pre-after.json" 2>&1)" >> "$chaos"
  }

  i=0; slot=0
  while [ "$(date +%s)" -lt "$end" ]; do
    w=$(( CHAOS_MIN + RANDOM % CHAOS_JITTER ))
    echo "$(date -u +%FT%TZ) sleeping ${w}s until the next disturbance" >> "$chaos"
    chaos_wait "$w"
    slot=$((slot+1))
    if [ "${CHAOS_SKIP_ONE_IN:-0}" -gt 0 ] && [ $((slot % CHAOS_SKIP_ONE_IN)) -eq 0 ]; then
      echo "$(date -u +%FT%TZ) skip (slot $slot, CHAOS_SKIP_ONE_IN=$CHAOS_SKIP_ONE_IN)" >> "$chaos"; continue
    fi
    # Whose slot: with both walks on, even slots are the follower's.
    fslot=0
    if [ "$fol_on" -eq 1 ] && { [ "$val_on" -eq 0 ] || [ $((slot % 2)) -eq 0 ]; }; then fslot=1; fi
    if [ "$fslot" -eq 1 ] && [ -z "$added" ] && [ "${CHAOS_FOLLOWER_CYCLES:-0}" -gt 0 ] \
       && [ "$pairs" -ge "${CHAOS_FOLLOWER_CYCLES:-0}" ]; then
      [ "$fol_done" -eq 0 ] && echo "$(date -u +%FT%TZ) followers: done, $pairs pair(s) (CHAOS_FOLLOWER_CYCLES=$CHAOS_FOLLOWER_CYCLES)" >> "$chaos"
      fol_done=1; fslot=0
    fi
    if [ "$fslot" -eq 1 ]; then
      if [ -z "$added" ]; then add_follower; else remove_follower; fi
      continue
    fi
    [ "$val_on" -eq 1 ] || continue
    n=${nodes[$((i % ${#nodes[@]}))]}; cyc=$((i / ${#nodes[@]})); kind=$(( (i + cyc) % 2 )); i=$((i+1))
    if [ "$kind" -eq 0 ]; then
      echo "$(date -u +%FT%TZ) restart $n" >> "$chaos"; docker restart "$n" >/dev/null 2>&1
    else
      p=$((60 + RANDOM % 120))
      echo "$(date -u +%FT%TZ) pause $n ${p}s" >> "$chaos"
      docker pause "$n" >/dev/null 2>&1; chaos_wait "$p"; docker unpause "$n" >/dev/null 2>&1
    fi
  done ) &
CHAOS=$!
fi

# Monitor: heights + total heals every 5 min
# followerHeals is its own column, not part of the heals sum: see the loop.
#
# Heights are every node's own accumulate_node_executed_block, read from the
# same scrape as the heals (heights.py, #4404). `dnHeightMax` is the highest
# block any Directory validator that answered executed, `dnValidatorsAnswered`
# how many answered; then one `exec.<container>.<partition>` column
# per node the run has. The column used to be `dnHeight`, the Directory
# ledger index as ONE node answered it — host port 26680, acc-bvn1-val1 —
# and on run 20260924T052134Z that was the restarted node: it read 207 for a
# minute while the other eleven went 215 -> 323.
mon_tmp=$(mktemp -d)
echo "time,$(python3 "$here/heights.py" header "$FOL_LIST")" > "$mon"
( while kill -0 $DRIVER 2>/dev/null; do
    rm -f "$mon_tmp"/*.prom
    # Heals = entries that came back in answer to a span request AND filled a
    # gap (#4283). This used to read syntheticHeals/anchorHeals off
    # consensus-status, which no node has reported since healing became
    # receiver-pull, so every run's record said "heals 0 -> 0" -- including
    # 20260917T212457Z, where 1,291 requests were answered and 2,231 entries
    # applied. A run record that says zero over that is not a record.
    # Over the VALIDATORS. A follower heals like any node, but folding its
    # count into this sum would change what the column means between a run
    # with a follower and a run without, and this column is compared across
    # runs (#4365). The follower's own heals are in soakmon's per-node table,
    # labelled.
    # followerHeals is EMPTY, not 0, on a run with no follower: there is no
    # instrument behind it, and an absent instrument must not render as a
    # measurement (REPORTING-SPEC 1, L1). It is the only column a
    # no-follower run's monitor.csv gains.
    heals=0; fol_heals=""; [ "$n_fol" -gt 0 ] && fol_heals=0
    for c in $(docker ps --filter name=acc-bvn --format '{{.Names}}'); do
      docker exec "$c" sh -c 'wget -q -O - http://127.0.0.1:26670/metrics 2>/dev/null' > "$mon_tmp/$c.prom"
      x=$(grep -E '^accumulate_conductor_heal_entries_total\{[^}]*outcome="applied"' "$mon_tmp/$c.prom" | awk '{s+=$NF} END {printf "%d", s}')
      case ",$FOL_LIST," in
        *",$c,"*) fol_heals=$(( ${fol_heals:-0} + ${x:-0} )) ;;
        *)        heals=$((heals + ${x:-0})) ;;
      esac
    done
    # Per-container stats alongside the fleet sum: the fleet CPU column dated
    # the 20260819 collapse, but WHICH nodes were burning had to be inferred.
    stats=$(docker stats --no-stream --format '{{.Name}},{{.CPUPerc}},{{.MemUsage}}' 2>/dev/null)
    ts=$(date -u +%FT%T)
    echo "$stats" | sed "s/^/$ts,/" >> "$rd/stats.csv"
    cpu=$(echo "$stats" | cut -d, -f2 | tr -d '%' | awk '{s+=$1} END {printf "%.0f", s}')
    echo "$ts,$(python3 "$here/heights.py" row "$FOL_LIST" "$mon_tmp" "$heals" "${cpu:-?}" "$fol_heals")" >> "$mon"
    # 30 s, not 5 min: run 20260903T121819Z climbed from 45 MiB to the
    # GOMEMLIMIT in ten minutes and stats.csv had two points for it (PLAN S0).
    sleep ${MON_INTERVAL:-$([ "$duration_seconds" -le 1800 ] && echo 20 || echo 30)}
  done ) &
MONLOOP=$!    # recorded so teardown can name it (#4364)

# Storage-backend counters over time (PLAN S0). BlockchainDB rewrites
# stats.json every 50 commits, so only the last snapshot survives a run — and
# stagedCommits, the D5 instrument, had no history. One row per (node,
# database) a minute, the few counters that move.
#
# THE leftover Paul killed by hand after both runs on 2026-09-19. This loop
# captured no `$!`, so teardown's kill list could not name it; with
# STORAGE_STATS_INTERVAL at 300 in soak.conf it outlived the run by up to
# five minutes, `docker exec`-ing into containers that were already gone.
# It records its PID now and teardown passes it to `stop_bg`, which kills
# the `sleep` before the subshell so the sleep is not orphaned either.
echo "time,node,database,commits,stagedCommits,shallowMisses,maintenanceErrors,permPutTotal,dynaPutTotal,dynaLiveHit,deepHits,deepMisses" > "$rd/storage-stats.csv"
( while kill -0 $DRIVER 2>/dev/null; do
    ts=$(date -u +%FT%TZ)
    # Every container mounts the whole network's config volume, so any one
    # of them sees every node's stats.json (run 20260903T173742Z had each
    # row eight times). Ask one container, and take the node from the path.
    c=$(docker ps --format '{{.Names}}' | grep -E '^acc-(dn|bvn)' | head -1)
    [ -n "$c" ] && for once in 1; do
      docker exec "$c" sh -c 'for f in $(find /root/.accumulate -name stats.json 2>/dev/null); do echo "== $f"; cat "$f"; done' 2>/dev/null \
        | python3 -c '
import sys, json
ts = sys.argv[1]
blob = sys.stdin.read()
for part in blob.split("== ")[1:]:
    path, _, body = part.partition("\n")
    try:
        d = json.loads(body)
    except Exception:
        continue
    parts = path.split("/")
    db = parts[-4] if len(parts) >= 4 else path      # dnn / bvnn
    node = parts[-5] if len(parts) >= 5 else "?"      # e.g. bvn2-4
    perm, dyna = d.get("perm") or {}, d.get("dyna") or {}
    print(",".join(str(x) for x in [ts, node, db, d.get("commits", ""), d.get("stagedCommits", ""),
          sum((d.get("shallowMisses") or {}).values()), d.get("maintenanceErrors", ""),
          perm.get("PutTotal", ""), dyna.get("PutTotal", ""), dyna.get("LiveHit", ""),
          sum(v.get("hits", 0) for v in (d.get("historyReads") or {}).values()),
          sum(v.get("misses", 0) for v in (d.get("historyReads") or {}).values())]))
' "$ts" >> "$rd/storage-stats.csv" 2>/dev/null
    done
    sleep ${STORAGE_STATS_INTERVAL:-60}
  done ) &
STORELOOP=$!   # THE leftover (#4364): see above and stop_bg

# Profiles on the hour (PLAN S0): the steady-state criteria compare the heap
# profile at hour 12 with hour 1, and a capture taken only at the wedge shows
# the corpse, not the growth. Same capture as wedgewatch, prefixed hourly-.
if [ -x "$here/wedgewatch.sh" ]; then
  ( while kill -0 $DRIVER 2>/dev/null; do
      # Sleep in short steps so the loop dies with the driver instead of
      # outliving the run by up to an hour (runs 173742Z and 213153Z).
      waited=0
      while [ "$waited" -lt "${PROFILE_INTERVAL:-3600}" ] && kill -0 $DRIVER 2>/dev/null; do
        sleep 30; waited=$((waited + 30))
      done
      kill -0 $DRIVER 2>/dev/null || break
      env RUN_DIR="$rd" "$here/wedgewatch.sh" --now hourly >> "$rd/wedgewatch.log" 2>&1
    done ) &
  PROFLOOP=$!   # recorded so teardown can name it (#4364)
fi

wait $DRIVER; rc=$?
# When the load generator exited, so the manifest can say whether the row it
# reads was taken after the drain or before it (reviewer N2). `ended` is far
# later — after the idle tail and the whole teardown — so it answers a
# different question and cannot stand in for this one.
lg_exit=$(date -u +%FT%TZ)

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
# NOT stallkill: when it is the one ending the run it waits for this script to
# finish recording and then takes the network down, so killing it here would
# leave the containers up. It exits on its own once this script is gone.
# MON is the supervisor; kill its current soakmon child too, by PID, or the
# restart loop's last child outlives the run.
[ -n "${MON:-}" ] && pkill -P "$MON" 2>/dev/null
# The read probe writes its report on SIGTERM; give it a moment before the
# network goes away so the last round and the report land.
if [ -n "${READPROBE:-}" ]; then kill $READPROBE 2>/dev/null; wait $READPROBE 2>/dev/null; fi
# Every background job, by the PID it recorded, through stop_bg so the `sleep`
# each one is parked in dies with it (#4364). MONLOOP, STORELOOP and PROFLOOP
# were not on this list at all until then.
stop_bg "${CHAOS:-}" "${MON:-}" "${MONLOOP:-}" "${STORELOOP:-}" "${PROFLOOP:-}" \
        "${SEIZE:-}" "${LOGCAP:-}" "${WEDGE:-}"
rm -rf "${mon_tmp:-/nonexistent-mon-tmp}"
# And say so if one survived anyway. The symptom is invisible — an orphan
# subshell doing nothing anyone sees — so it has to become a line in the log
# rather than something the operator notices in `ps` a day later (#4364).
sleep 1
for p in "${CHAOS:-}" "${MON:-}" "${MONLOOP:-}" "${STORELOOP:-}" "${PROFLOOP:-}" \
         "${SEIZE:-}" "${LOGCAP:-}" "${WEDGE:-}"; do
  [ -n "$p" ] && kill -0 "$p" 2>/dev/null \
    && echo "WARNING: background job $p survived teardown: $(ps -o args= -p "$p" 2>/dev/null | head -c 100)" | tee -a "$log"
done
# The chaos walk is stopped; an added follower it had not yet removed is
# still up and is not a service `down` knows about.
rm_late_followers "still up at teardown"
ended=$(date -u +%FT%TZ)
echo "== soak finished $(date -u) driver-exit=$rc ==" | tee -a "$log"

# Capture evidence that only exists while the containers are alive. The
# interval reconcile (#4073) logs each pull, and those logs die with the
# containers — so a run that proved the fix would otherwise leave no trace.
$compose logs --no-color > "$rd/node-logs.txt" 2>/dev/null
grep "Reconcile: pulled messages" "$rd/node-logs.txt" > "$rd/reconcile-pulls.txt" 2>/dev/null
reconcile_pulls=$(wc -l < "$rd/reconcile-pulls.txt" 2>/dev/null || echo 0)
# Storage-backend counters (#4165). BlockchainDB writes stats.json beside each
# database — permanent-layer duplicates and conflicts, per record shape — and
# it dies with the volume. One file per (node, database).
mkdir -p "$rd/storage-stats"
# Every container mounts the whole network's volume, so one container sees
# every node's stats.json — and naming the copy by the CONTAINER wrote one
# node's file under every node's name (run 20260904T180918Z: eight identical
# bvnn files). Ask one container, name the copy by the node in the path.
c=$(docker ps --format '{{.Names}}' | grep -E '^acc-(dn|bvn)' | head -1)
[ -n "$c" ] && for f in $(docker exec "$c" sh -c 'find /root/.accumulate -name stats.json 2>/dev/null'); do
  node=$(printf '%s' "$f" | sed -E 's#^/root/.accumulate/([^/]+)/.*#\1#')
  # Every node runs TWO databases (dnn/ and bvnn/), both named accumulate.db —
  # name the copy by the path under the node's directory or the second
  # overwrites the first.
  rel=$(printf '%s' "$f" | sed -E 's#^/root/.accumulate/[^/]+/##; s#/stats.json$##; s#/#-#g')
  docker exec "$c" cat "$f" > "$rd/storage-stats/${node}-$rel.json" 2>/dev/null
done
rmdir "$rd/storage-stats" 2>/dev/null || true
# The follower's verdict (#4365), read out of the captured log rather than
# polled live: node-logs-live.txt streams from the first block, so nothing is
# lost, and comparing each node's anchor line per (source, destination,
# block) — the source read from the line, #4370 — is the same evidence a
# debugger uses for a divergence. A validator writes `Sending an anchor`; a
# node in no committee writes `Anchor not sent`, because since #4367 it
# states the root it computed instead of signing and dispatching one. Cheaper than an API poll per block at a one-second
# interval, and it re-runs on the saved run directory afterwards without the
# network. On an image built before #4370 the lines carry no source, the two
# nodes in a container cannot be told apart, and the root rows say
# `— not measured` rather than comparing.
if [ "$n_fol" -gt 0 ] && [ -x "$here/followerlog.py" ]; then
  "$here/followerlog.py" "$rd/node-logs-live.txt" \
    --follower "${FOL_LIST%%,*}" \
    --definition "$rd/network-definition.json" \
    --behind "$rd/follower.csv" > "$rd/follower-report.md" 2>>"$log"
  "$here/followerlog.py" "$rd/node-logs-live.txt" \
    --follower "${FOL_LIST%%,*}" \
    --definition "$rd/network-definition.json" \
    --behind "$rd/follower.csv" --rows > "$rd/follower-rows.md" 2>/dev/null
  echo "   follower report: $rd/follower-report.md" | tee -a "$log"
fi

# Final produced-vs-received across every channel, the check that sees a stall.
if [ -x "$here/streams.py" ]; then
  "$here/streams.py" > "$rd/streams-final.txt" 2>&1
  stalled_end=$(grep -oE 'stalled channels: [0-9]+' "$rd/streams-final.txt" | grep -oE '[0-9]+' | head -1)
fi
stalled_end="${stalled_end:-unknown}"

# Accepted, neither certified here nor accepted on relay (#4364). soakmon
# writes submissions.csv every 30s from three families:
# accumulate_dagbft_submissions_total, ..._certified_own_transactions_total
# and ..._relayed_total.
#   * certified, not proposed: a follower authors and broadcasts headers
#     carrying its own batches like any node, and what it never gets is the
#     2f+1 votes, so "never proposed" would read 0 on it;
#   * minus the relay, because Paul (2026-09-19) said followers can and
#     should relay — and relay is not gated on being synced: a read needs
#     local state, a relay needs none, so a node relays whether it is
#     following OR syncing (executor.md step 6). A node that hands on
#     everything it takes is WORKING, and subtracting only certified would
#     make it the largest red number on the board.
#   * minus the relays a validator REFUSED, because that is an answer the
#     caller was given and not a loss. Left in, whoever floods a follower
#     with garbage drives its figure while the same envelope sent straight
#     to a validator costs nothing (threat-reviewer F4 on #4366). The
#     refusals stay visible on the `relayed` row.
# What is left is the stranded count, and it is the one that must be 0 —
# at the LAST sample, which soakmon writes on its way out, after the drain.
# The row states the trend into it, because at any earlier sample a relay
# in flight and a stranded transaction are the same number.
# No build exports any of the three yet (#4366, #4369), so the file is a
# header with no rows and this says `— not measured` — never 0, which would
# assert that nothing stranded, the one claim run 20260919T191634Z could
# not make.
relay_row() {   # $1 = role: validator | follower
  python3 - "$rd/submissions.csv" "${1:-}" <<'PYEOF'
import csv, sys
path, role = sys.argv[1], sys.argv[2]
KEYS = ("relayedTaken", "relayedRefused", "relayedNotReady",
        "relayedUnreachable")
try:
    rows = [r for r in csv.DictReader(open(path))
            if not role or r.get("role") == role]
except OSError:
    print("— not measured (no `submissions.csv`; soakmon wrote none)"); raise SystemExit
if not rows:
    print("— not measured (no node exports `accumulate_dagbft_relayed_total`; #4366, #4369)")
    raise SystemExit
last = max(r["time"] for r in rows)
# DEDUPE BY (time, node, partition), last row wins. Timestamps are whole
# seconds and the final row is forced past the 30s interval, so about one
# run in thirty lands it in the same second as a periodic one — and summing
# both reported 1,600 taken for 800 (reviewer N1). These are counters: two
# readings of the same counter are one reading, never a sum.
at_last = {}
for r in rows:
    if r["time"] == last:
        at_last[(r["time"], r.get("node"), r.get("partition"))] = r
# ABSENT, ZERO AND NEVER-INCREMENTED ARE THREE DIFFERENT FACTS. A labelled
# counter has no child series until it is first incremented, so an empty
# `relayedRefused` beside a populated `relayedTaken` is a real 0 — the
# family is exported and nothing was refused — while an empty one with no
# relay column populated ANYWHERE in the run is not measured at all. Run
# 20260919T231856Z printed both as `0` and the reader had to open relay.go
# to learn which (REPORTING-SPEC 1). Family presence is judged over the
# WHOLE file, not the last sample: a counter first incremented mid-run is
# exported from then on.
family = any((r.get(k) or "").strip() for r in rows for k in KEYS)
tot = {k: 0 for k in KEYS}
seen = {k: False for k in KEYS}
for r in at_last.values():
    for k in KEYS:
        v = (r.get(k) or "").strip()
        if v:
            try:
                tot[k] += int(v); seen[k] = True
            except ValueError:
                pass
if not family:
    print("— not measured (no node exports `accumulate_dagbft_relayed_total`; "
          "#4366, #4369)")
    raise SystemExit


def cell(k):
    if seen[k]:
        return str(tot[k])
    return "0 (series present)" if family else "— not measured"


print("%s taken / %s refused / %s target not ready / %s unreachable (as of %s)"
      % (cell("relayedTaken"), cell("relayedRefused"), cell("relayedNotReady"),
         cell("relayedUnreachable"), last))
PYEOF
}

# The stranded figure across each disturbance (#4364). The acceptance
# criterion is "it does not climb between disturbances, and every step is
# attributable to one of them" — which was computable from submissions.csv
# and chaos.log and judgeable from neither, because sub_row's trend looks at
# the last five samples of a twelve-hour run. A criterion with no instrument
# is a criterion nobody applies.
#
# THE FIGURE IS NOT MONOTONE even though its inputs are: it rises when a
# submission is accepted and falls when the relay is answered, so between
# samples it jitters by whatever is in flight. Two rows either side of a
# disturbance therefore measure the jitter as often as the loss. The SETTLED
# level over a stretch is its MINIMUM — everything above the floor was in
# flight and came back.
#
# THE WINDOWS ARE LOCAL TO EACH DISTURBANCE, and that is the correction.
# Taking the minimum over a whole interval [e_i, e_i+1) puts it at the
# interval's START, so a rise in the middle of a quiet stretch first shows
# up as the NEXT interval's minimum and is billed to the next disturbance:
# a run that was flat through a restart, climbed +3 with nothing happening,
# and was flat through the next restart printed "01:20Z restart: 0 -> 3
# (+3)" — a violation rendered as compliance, on the one case this table
# exists for (reviewer on #4364). So:
#   step_i  = min over [e_i + S, e_i + W)  -  min over [e_i - W, e_i)
#   creep_i = min over [e_i+1 - W, e_i+1)  -  min over [e_i + S, e_i + W)
# S is the settle: the sample at the disturbance's own second still reads
# the pre-effect level, because a relay must time out before it gives up.
# The step is what the disturbance cost; the creep is the climb in the
# quiet stretch, and THAT is the criterion's number. Windows are clamped so
# two never meet, and a clamped one says so.
#
# A pause's step lands at the UN-pause, not at the log line: chaos logs
# `pause <node> <p>s` when it starts and never logs the end, so the
# disturbance's effective moment is the timestamp plus p.
steps_rows() {   # $1 = role: validator | follower
  python3 - "$rd/submissions.csv" "$rd/chaos.log" "${1:-}" \
           "${STEP_WINDOW_SECS:-120}" "${STEP_SETTLE_SECS:-60}" "$here" <<'PYEOF'
import re, sys
sys.path.insert(0, sys.argv[6])
import runseries

subs, chaos, role = sys.argv[1], sys.argv[2], sys.argv[3]
W = int(sys.argv[4]) if len(sys.argv) > 4 and sys.argv[4] else 120
S = int(sys.argv[5]) if len(sys.argv) > 5 and sys.argv[5] else 60


def row(a, b):
    print("| %s | %s |" % (a, b))


def hhmm(t):
    return t[11:16] + "Z"


if S >= W:
    row("stranded across disturbances",
        "— not measured (STEP_SETTLE_SECS=%d is not less than "
        "STEP_WINDOW_SECS=%d, so there is no after-window and no step can "
        "be taken)" % (S, W))
    raise SystemExit

# --- the disturbances, at the moment they take effect -----------------------
try:
    lines = [l.rstrip("\n") for l in open(chaos) if l.strip()]
except OSError:
    row("stranded across disturbances", "— not measured (no `chaos.log`)")
    raise SystemExit
events = []
for l in lines:
    m = re.match(r"^(\S+Z) restart (\S+)", l)
    if m:
        events.append([runseries._epoch(m.group(1)), hhmm(m.group(1)),
                       "restart", m.group(2)])
        continue
    m = re.match(r"^(\S+Z) pause (\S+) (\d+)s", l)
    if m:
        # at the UN-pause: chaos logs the start and never the end
        events.append([runseries._epoch(m.group(1)) + int(m.group(3)),
                       hhmm(m.group(1)), "pause", m.group(2)])
if not events:
    if any(" DISABLED " in l for l in lines):
        row("stranded across disturbances",
            "chaos off — no disturbances to attribute steps to")
    else:
        row("stranded across disturbances",
            "— not measured (`chaos.log` records no restart or pause)")
    raise SystemExit
events.sort()

# --- the series, built once, in runseries.py --------------------------------
SER = runseries.load(subs, role, W)
if SER["error"]:
    row("stranded across disturbances", "— not measured (%s)" % SER["error"])
    raise SystemExit
pts = runseries.points(SER)
if not pts:
    row("stranded across disturbances",
        "— not measured (no sample has all %d (node, partition) pairs; "
        "%d incomplete)" % (len(SER["pairs"]), SER["dropped"]))
    raise SystemExit
t0, t1 = pts[0][0], pts[-1][0]

# THE WINDOWS ARE LOCAL TO EACH DISTURBANCE, and the after-window starts S
# seconds late: the sample stamped at the disturbance's own second still
# reads the pre-effect level, because a relay has to time out before it
# gives up. The two mistakes are mirrors and a table built with either is
# wrong in the opposite direction.
#
# The after-floor takes the FIRST TWO complete samples at or after t + S,
# bounded by the next disturbance — not everything up to it. A node
# unreachable through the settle window used to give `— not measured` and
# then vanish from both summary rows, with the creep after it never
# measured at all; now the floor is taken from the samples nearest the
# disturbance whenever they arrive, and the row says how late.
nbr = [None] + [e[0] for e in events] + [None]
win = []
for i, e in enumerate(events):
    lo, hi = e[0] - W, e[0] + W
    by_event = False
    if nbr[i] is not None and nbr[i] > lo:
        lo, by_event = nbr[i], True
    if nbr[i + 2] is not None and nbr[i + 2] < hi:
        hi, by_event = nbr[i + 2], True
    lo = max(lo, t0 - 1)
    # The after-window may run past W when the node was away: it is
    # bounded by the next disturbance, or by the end of the run.
    a_hi = nbr[i + 2] if nbr[i + 2] is not None else t1 + 1
    hi = min(hi, t1 + 1)
    if by_event:
        note = (" (window met a neighbouring disturbance — its step and the "
                "creep beside it are not separable)")
    elif (e[0] - lo) < W or (hi - e[0]) < W:
        note = " (window shortened by the run's start or end)"
    else:
        note = ""
    win.append((lo, a_hi, note))

biggest_step = biggest_step_at = None
biggest_creep = biggest_creep_at = None
prev_after = prev_label = None
unmeasured = 0

head, _ = runseries.floor_of(pts, t0, min(events[0][0], t0 + W))
if head is not None:
    row("baseline (the first %ds of the run)" % W, "%d" % head)

for i, (t, at, kind, node) in enumerate(events):
    lo, a_hi, note = win[i]
    before, _ = runseries.floor_of(pts, lo, t)
    # The after-window is [t + S, t + W), bounded by the next disturbance.
    # A minimum already ignores a later loss — a loss RAISES the figure —
    # so taking only the first few samples could only exclude later, LOWER
    # readings and overstate the step (reviewer M1). When the window is
    # EMPTY the node was away: start a window of the same length at the
    # first complete sample from the settle, and say how late it was.
    after, a_t = runseries.floor_of(pts, t + S, min(t + W, a_hi))
    if after is None:
        back = [tt for tt, _ in pts if t + S <= tt < a_hi]
        if back:
            after, a_t = runseries.floor_of(pts, back[0],
                                            min(back[0] + W, a_hi))
    if a_t is not None and a_t > t + W:
        note += " (the after-floor is %ds late — no complete sample sooner)" % (
            int(a_t - (t + S)))
    # the quiet stretch that ENDS at this disturbance
    start_floor = prev_after if prev_after is not None else head
    start_label = prev_label if prev_label is not None else "the run's start"
    if start_floor is not None and before is not None:
        creep = before - start_floor
        row("between %s and %s" % (start_label, at), "crept %+d" % creep)
        if biggest_creep is None or creep > biggest_creep:
            biggest_creep, biggest_creep_at = creep, "%s to %s" % (start_label, at)
    if before is None or after is None:
        unmeasured += 1
        row("%s %s %s" % (at, kind, node),
            "— not measured (no complete sample in the %ds before, or "
            "between the settle and the next disturbance)%s" % (W, note))
        # The creep AFTER an unmeasured disturbance is still measurable:
        # start it at the first complete sample from here, or it and every
        # later loss in that stretch go unreported.
        nxt = [v for tt, v in pts if t <= tt < a_hi]
        prev_after = nxt[0] if nxt else None
        prev_label = at
        continue
    step = after - before
    row("%s %s %s" % (at, kind, node),
        "stranded %d -> %d (%+d)%s" % (before, after, step, note))
    if biggest_step is None or step > biggest_step:
        biggest_step, biggest_step_at = step, "%s %s %s" % (at, kind, node)
    prev_after, prev_label = after, at

tail, _ = runseries.floor_of(pts, max(t1 - W, events[-1][0] + S), t1 + 1)
if prev_after is not None and tail is not None:
    creep = tail - prev_after
    row("between %s and the end of the run" % prev_label, "crept %+d" % creep)
    if biggest_creep is None or creep > biggest_creep:
        biggest_creep = creep
        biggest_creep_at = "%s to the end of the run" % prev_label

miss = "" if not unmeasured else " (%d of %d disturbances not measured)" % (
    unmeasured, len(events))
if biggest_step is not None:
    row("largest step at a disturbance",
        "%+d, at %s%s%s" % (biggest_step, biggest_step_at,
                            "" if biggest_step else
                            " — no disturbance cost anything", miss))
elif unmeasured:
    row("largest step at a disturbance",
        "— not measured (%d of %d disturbances not measured)"
        % (unmeasured, len(events)))
if biggest_creep is not None:
    row("largest climb between disturbances",
        "%+d, %s%s%s" % (biggest_creep, biggest_creep_at,
                         "" if biggest_creep > 0 else
                         " — the figure did not climb", miss))
elif unmeasured:
    row("largest climb between disturbances",
        "— not measured (%d of %d disturbances not measured)"
        % (unmeasured, len(events)))
if SER["dropped"]:
    row("samples dropped as incomplete",
        "%d of %d — a node reported no counts at those, and a fleet total "
        "missing one node dips by that node's count"
        % (SER["dropped"], len(SER["samples"])))
reset = runseries.resets_row(SER)
if reset:
    row("counter resets seen", reset)
PYEOF
}

# One verdict per add-follower and per remove-follower (#4364), from the
# run's captured files only. An add is the container's life, from its
# `add-follower` line to the next `remove-follower`: every number on its row
# is read inside that life, so a second add of the same container never
# borrows the first one's. A quantity the life did not record reads `not
# measured`, never a pass. A removal's row is followerchaos.unaffected over
# the readings either side of it — the same judgement the chaos log states.
follower_verdict_rows() {
  python3 - "$rd" "$here" <<'PYEOF'
import csv, datetime, json, os, re, sys

rd, here = sys.argv[1], sys.argv[2]
sys.path.insert(0, here)
NM = "not measured"


def secs(t):
    return datetime.datetime.strptime(t, "%Y-%m-%dT%H:%M:%SZ").replace(
        tzinfo=datetime.timezone.utc).timestamp()


def hhmm(t):
    return t[11:16] + "Z"


def rows_of(name):
    try:
        return list(csv.DictReader(open(os.path.join(rd, name))))
    except OSError:
        return None


try:
    lines = [l.rstrip("\n") for l in open(os.path.join(rd, "chaos.log")) if l.strip()]
except OSError:
    print("| add-follower / remove-follower | — %s (no `chaos.log`) |" % NM)
    raise SystemExit

# --- the lives, from chaos.log ----------------------------------------------
lives, removals, cur = [], [], None
for l in lines:
    m = re.match(r"^(\S+Z) (.*)$", l)
    if not m:
        continue
    t, rest = m.group(1), m.group(2)
    a = re.match(r"add-follower (\S+)", rest)
    if a:
        cur = {"node": a.group(1), "at": t, "end": None, "active": None,
               "activeS": None, "never": None, "match": None}
        lives.append(cur)
        continue
    r = re.match(r"remove-follower (\S+)", rest)
    if r:
        removals.append({"node": r.group(1), "at": t, "n": len(removals) + 1})
        if cur is not None and cur["node"] == r.group(1):
            cur["end"] = t
            cur = None
        continue
    if cur is None:
        continue
    m = re.match(r"follower (\S+) ACTIVE .*, (\d+)s after it was added", rest)
    if m and m.group(1) == cur["node"]:
        cur["active"], cur["activeS"] = t, int(m.group(2))
    m = re.match(r"follower (\S+) NEVER ACTIVE: (.*)$", rest)
    if m and m.group(1) == cur["node"]:
        cur["never"] = m.group(2)
    m = re.match(r"follower (\S+) first root match \((.*)\), (\d+)s after", rest)
    if m and m.group(1) == cur["node"]:
        f = dict(kv.split("=", 1) for kv in m.group(2).split() if "=" in kv)
        cur["match"] = "block %s (source %s), %ss after the add" % (
            f.get("block", "?"), f.get("source", "?"), m.group(3))

if not lives and not removals:
    print("| add-follower / remove-follower | none in `chaos.log` |")
    raise SystemExit

nodestate = rows_of("nodestate.csv")
behind = rows_of("follower.csv")
subs = rows_of("submissions.csv")
probe = rows_of("readprobe-follower.csv")


def in_life(rows, life, col="node"):
    lo = secs(life["at"])
    hi = secs(life["end"]) if life["end"] else float("inf")
    return [r for r in rows or [] if r.get(col) == life["node"]
            and lo <= secs(r["time"]) <= hi]


out = []
for life in lives:
    parts = []
    # container start -> ACTIVE: nodestate.csv's reached rows for the start
    # inside this life (docker start follows the add line by a second or
    # two), the chaos log's own seconds only when soakmon wrote none.
    reached = [r for r in in_life(nodestate, life) if r.get("kind") == "reached"
               and r.get("startToActiveS")
               and secs(r["containerStarted"]) >= secs(life["at"]) - 5]
    handoff = None
    if reached:
        w = max(reached, key=lambda r: float(r["startToActiveS"]))
        handoff = max(secs(r["time"]) for r in reached)
        parts.append("container start → ACTIVE %ss (%s)" % (
            w["startToActiveS"], ", ".join(
                "%s %ss" % (r["partition"], r["startToActiveS"])
                for r in sorted(reached, key=lambda r: r["partition"]))))
    elif life["active"]:
        handoff = secs(life["active"])
        parts.append("ACTIVE %ds after the add (chaos.log; no `nodestate.csv` "
                     "row for this start)" % life["activeS"])
    elif life["never"]:
        parts.append("NEVER ACTIVE (%s)" % life["never"])
    else:
        parts.append("container start → ACTIVE %s" % NM)

    # blocks behind at hand-off: the follower.csv sample nearest the moment
    # the last partition went ACTIVE, per partition.
    if handoff is None:
        parts.append("behind at hand-off %s (no hand-off)" % NM)
    else:
        rs = in_life(behind, life, col="follower")
        if not rs:
            parts.append("behind at hand-off %s (no `follower.csv` sample)" % NM)
        else:
            near = min({r["time"] for r in rs},
                       key=lambda t: (abs(secs(t) - handoff), -secs(t)))
            parts.append("behind at hand-off %s blocks (sample %s)" % (
                ", ".join("%s %s" % (r["partition"], r.get("behindBlocks") or "?")
                          for r in sorted(rs, key=lambda r: r["partition"])
                          if r["time"] == near), near[11:]))

    parts.append("first root match %s" % life["match"] if life["match"]
                 else "first root match %s" % NM)

    # NotReady on a read before ACTIVE, and which service answered it.
    rs = in_life(probe, life, col="follower")
    before = [r for r in rs if handoff is None or secs(r["time"]) < handoff]
    if not before:
        parts.append("NotReady before ACTIVE %s (no read of it in `readprobe-follower.csv`)" % NM)
    else:
        # a row is one round's reads of one partition with one outcome
        n = lambda rs: sum(int(r["reads"]) if (r.get("reads") or "").isdigit()
                           else 1 for r in rs)
        nr = [r for r in before if r.get("outcome") == "not-ready"]
        if nr:
            parts.append("NotReady before ACTIVE on %d read(s): %s" % (
                n(nr), ", ".join(sorted({"%s %s" % (r["partition"], r["service"])
                                         for r in nr}))))
        else:
            parts.append("NO NotReady before ACTIVE (%d read(s), none refused)" % n(before))

    # stranded, and relayed-taken against accepted, on THIS follower over
    # this container's life: the last sample in it, deduped by partition.
    rs = in_life(subs, life)
    if not rs:
        parts.append("accepted / relayed-taken / stranded %s (no `submissions.csv` row in its life)" % NM)
    else:
        last = max(r["time"] for r in rs)
        tot = {"accepted": None, "relayedTaken": None,
               "acceptedNeitherCertifiedTakenNorRefused": None}
        for r in {r.get("partition"): r for r in rs if r["time"] == last}.values():
            for k in tot:
                v = (r.get(k) or "").strip()
                if v.isdigit():
                    tot[k] = (tot[k] or 0) + int(v)
        show = lambda v: NM if v is None else str(v)
        ratio = ""
        if tot["accepted"] and tot["relayedTaken"] is not None:
            ratio = " (%.1f%% of accepted)" % (100.0 * tot["relayedTaken"] / tot["accepted"])
        parts.append("accepted %s, relayed-taken %s%s, stranded %s (as of %s)" % (
            show(tot["accepted"]), show(tot["relayedTaken"]), ratio,
            show(tot["acceptedNeitherCertifiedTakenNorRefused"]), last[11:]))

    out.append((life["at"], "| %s add-follower %s | %s |" % (
        hhmm(life["at"]), life["node"], "; ".join(parts))))

import followerchaos
for rm in removals:
    pre = os.path.join(rd, "follower-removal-%d" % rm["n"])
    snaps = []
    for which in ("before", "at", "after"):
        try:
            with open("%s-%s.json" % (pre, which)) as f:
                snaps.append(json.load(f))
        except (OSError, ValueError):
            snaps.append(None)
    if any(s is None for s in snaps):
        text = "%s (no `follower-removal-%d-{before,at,after}.json`)" % (NM, rm["n"])
    else:
        text = followerchaos.unaffected(*snaps)[1]
    out.append((rm["at"], "| %s remove-follower %s (removal %d) | %s |" % (
        hhmm(rm["at"]), rm["node"], rm["n"], text)))
for _, row in sorted(out, key=lambda x: x[0]):
    print(row)
PYEOF
}

nodestate_row() {   # $1 = role: validator | follower
  # Rejoined, not ACTIVE (#4404): the gauge goes ACTIVE at the join's first
  # root match and is never demoted, so run 20260924T052134Z's row read three
  # failed starts as reaching ACTIVE. rejoin.py judges each start on the
  # gauge, its executed height against its partition's (nodestate.csv), and
  # its anchors against its peers' (node-logs-live.txt).
  python3 "$here/rejoin.py" "$rd" "${1:-}" --max-behind "${REJOIN_MAX_BEHIND:-10}"
}

sub_row() {   # $1 = role, $2 = when the loadgen exited, $3 = "stallkill" or ""
  python3 - "$rd/submissions.csv" "${1:-}" "${2:-}" "${3:-}" "$here" \
           "${STEP_WINDOW_SECS:-120}" <<'PYEOF'
import sys
sys.path.insert(0, sys.argv[5])
import runseries

path, role = sys.argv[1], sys.argv[2]
lg_exit = sys.argv[3] if len(sys.argv) > 3 else ""
stopped_early = sys.argv[4] if len(sys.argv) > 4 else ""
W = int(sys.argv[6]) if len(sys.argv) > 6 and sys.argv[6] else 120

# The series is built in runseries.py, once, for this row and the step
# table both: completeness, the counter-reset offsets and the "a level is
# a minimum" rule are one implementation or they drift (#4364).
S = runseries.load(path, role, W)
if S["error"]:
    print("— not measured (%s; soakmon wrote none)" % S["error"])
    raise SystemExit
rows_at = runseries.complete(S)
if not rows_at:
    print("— not measured (no sample has all %d (node, partition) pairs; "
          "%d incomplete)" % (len(S["pairs"]), S["dropped"]))
    raise SystemExit

TREND_N = 5
last = rows_at[-1]
per = last["byPair"]
total = sum(per.values())
(wv, wk) = max((v, k) for k, v in per.items())

vals = [s["total"] for s in rows_at[-TREND_N:]]
if len(vals) < 2:
    trend = "no trend (one sample)"
elif vals[-1] == 0:
    trend = "0 at the last sample"
elif vals[-1] < vals[0]:
    trend = "falling %d -> %d over the last %d samples (draining)" % (
        vals[0], vals[-1], len(vals))
else:
    trend = "%s %d -> %d over the last %d samples" % (
        "flat at" if vals[-1] == vals[0] else "rising", vals[0], vals[-1],
        len(vals))

# WAS THE FINAL ROW WRITTEN, AND IS IT A DRAINED SAMPLE (reviewer N2).
# "0 at the last sample after the drain" is unreadable if a reader cannot
# tell. Two separate facts, and neither is inferred from the other: the
# `sample` column says whether soakmon's exit write landed, and the
# timestamp says whether it postdates the load generator.
import csv as _csv
raw = [r for r in _csv.DictReader(open(path))
       if not role or r.get("role") == role]
kinds = {(r.get("sample") or "").strip() for r in raw
         if r["time"] == last["time"]}
if "final" in kinds:
    final = "final row written"
elif not any(kinds):
    final = "final row: unknown (this file has no `sample` column — pre-#4364 run)"
else:
    final = ("FINAL ROW MISSING — soakmon's exit write did not land; "
             "this reading is mid-drain and up to 30s stale")
if lg_exit:
    try:
        d = int(runseries._epoch(last["time"]) - runseries._epoch(lg_exit))
        final += (", %ds after the load generator exited" % d if d >= 0 else
                  ", but %ds BEFORE the load generator exited — mid-drain" % -d)
    except ValueError:
        pass
if S["dropped"]:
    final += ("; %d sample%s skipped as incomplete (a node reported no "
              "counts)" % (S["dropped"], "" if S["dropped"] == 1 else "s"))
    if S["samples"][-1]["time"] != last["time"]:
        final += " — INCLUDING THE LAST, so this is not the final row"
if S["resets"]:
    final += "; %d counter reset%s carried forward" % (
        len(S["resets"]), "" if len(S["resets"]) == 1 else "s")
if stopped_early:
    final += ("; the run was stopped by stallkill, so the load generator was "
              "killed mid-flight and this is NOT a drained sample")

print("%d, worst %s on %s (as of %s; %s; %s)"
      % (total, wv, "/".join(wk), last["time"], trend, final))
PYEOF
}

# stallkill ends a run by killing the load generator mid-flight, and it
# appends its own heading to the manifest before it does. The forced final
# row still lands — soakmon gets TERM, not KILL — but it is NOT a drained
# sample, and the row that quotes it has to say so.
stopped_early=""
grep -q '^## Stopped early by stallkill' "$manifest" 2>/dev/null && stopped_early=stallkill

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
  echo "| Directory height (block, the highest any of its validators that answered executed; first -> last sample) | ${first_h:-— not measured} -> ${last_h:-— not measured} |"
  echo "| heals | ${first_x:-?} -> ${last_x:-?} |"
  echo "| chaos events | $n_chaos |"
  echo "| monitor samples | $(( $(wc -l < "$mon") - 1 )) |"
  echo "| seizure | $(grep -q SEIZED "$rd/seizewatch.out" 2>/dev/null && grep SEIZED "$rd/seizewatch.out" | tail -1 || echo 'none detected') |"
  echo "| reconcile pulls (#4073) | $reconcile_pulls |"
  echo "| stalled channels at end | $stalled_end |"
  echo "| load generator reads a node would not answer (whole run) | $(python3 -c '
import json, sys
try:
    q = json.load(open(sys.argv[1])).get("queries")
except Exception:
    q = None
if q is None:
    print("— not measured (loadgen-stats.json has no `queries`: a load generator built before #4404)")
else:
    print("NotReady: %d answers retried at another endpoint, %d queries no endpoint would answer; transport error: %d answers retried at another endpoint, %d queries failed at every endpoint" % (
        q.get("notReadyRetriedElsewhere", 0), q.get("notReadyAtEveryEndpoint", 0),
        q.get("transportErrorRetriedElsewhere", 0), q.get("transportErrorAtEveryEndpoint", 0)))
' "$rd/loadgen-stats.json" 2>/dev/null) |"
  echo "| read-back probe | $(grep -m1 '^\*\*Whole run:\*\*' "$rd/readprobe-report.md" 2>/dev/null | sed 's/\*\*//g' || echo 'no report') |"
  if [ "$n_fol" -gt 0 ]; then
    echo "| follower read probe | $(grep -m1 -E '^\*\*acc-' "$rd/readprobe-report.md" 2>/dev/null | sed 's/\*\*//g' || echo '— not measured (no readprobe report)') |"
  fi
  # A run that wedged and dumped is the most valuable kind of run there is;
  # say so in the verdict rather than leaving the dirs to be stumbled upon.
  echo "| wedge captures (#4125) | $(ls -d "$rd"/wedge-* 2>/dev/null | wc -l) $(ls -d "$rd"/wedge-* 2>/dev/null | xargs -r -n1 basename | paste -sd', ' -) |"
  echo "| accepted, neither certified here, taken on relay, nor refused (#, whole run, the validators) | $(sub_row validator "$lg_exit" "$stopped_early") |"
  echo "| restarted node rejoined (per node and partition: gauge ACTIVE, executed block within ${REJOIN_MAX_BEHIND:-10} of the highest block any of the partition's answering validators executed, through its last reading, and every anchor it stated agreeing with its peers'; s = container start to the first sample ACTIVE and within that bound; the validators) | $(nodestate_row validator) |"
  if [ "$n_fol" -gt 0 ]; then
    echo
    echo "### Follower (#4365)"
    echo
    echo "| what | value |"
    echo "|---|---|"
    if [ -s "$rd/follower-rows.md" ]; then
      cat "$rd/follower-rows.md"
    else
      echo "| every follower measurement | — not measured (followerlog.py produced nothing; see \`soak.log\`) |"
    fi
    echo "| accepted, neither certified here, taken on relay, nor refused (#, whole run) | $(sub_row follower "$lg_exit" "$stopped_early") |"
    echo "| relayed (#, whole run) | $(relay_row follower) |"
    echo "| follower start rejoined (per partition, the add-follower and every restart; same reading as the validators' row) | $(nodestate_row follower) |"
    echo
    echo "**Stranded across disturbances (#4364).** The criterion is that the"
    echo "figure does not climb between disturbances and that every step is"
    echo "attributable to one of them — these counters never clear, so this is"
    echo "a cumulative loss, not a level. Two different numbers, so both are"
    echo "here: a **step** is what a disturbance cost, and a **creep** is the"
    echo "climb in the quiet stretch between two of them. The criterion's"
    echo "number is \`largest climb between disturbances\`."
    echo
    echo "Settled means the MINIMUM over a window, because the figure jitters"
    echo "by whatever is in flight between samples and a single reading is not"
    echo "a level. The windows are LOCAL — ${STEP_WINDOW_SECS:-120}s either"
    echo "side of the disturbance, four samples at the 30s cadence and well"
    echo "inside the chaos cadence — so a rise in the middle of a quiet"
    echo "stretch is a creep and not the next disturbance's step. The"
    echo "after-window starts ${STEP_SETTLE_SECS:-60}s late, because the"
    echo "sample at the disturbance's own second still reads the pre-effect"
    echo "level: a relay has to time out before it gives up, so the loss"
    echo "shows a sample later and a floor taken from that second would"
    echo "print the step as 0 and the loss as the creep after it. The sample"
    echo "AT the disturbance is in neither window. A window clamped by a"
    echo "neighbouring event says so, and there the step and the creep beside"
    echo "it are not separable. A pause is dated at its un-pause."
    echo
    echo "| disturbance | stranded |"
    echo "|---|---|"
    steps_rows follower
    echo
    echo "**Per add-follower and remove-follower (#4364).** Each add is read"
    echo "over that container's life only, from its \`add-follower\` line to"
    echo "its \`remove-follower\`; a quantity the life did not record reads"
    echo "\`not measured\`. A removal is judged on the readings"
    echo "${FOLLOWER_WINDOW_SECS:-30}s either side of it."
    echo
    echo "| event | verdict |"
    echo "|---|---|"
    follower_verdict_rows
    echo
    echo "Full detail in \`follower-report.md\`; the per-sample series in \`follower.csv\`."
  fi
  echo
  echo "Raw: \`soak.log\`, \`monitor.csv\`, \`mem.csv\` (every node, with its role), \`submissions.csv\`, \`chaos.log\`, \`nodestate.csv\`, \`loadgen-stats.json\`, \`readprobe.csv\` / \`readprobe-report.md\`$([ "$n_fol" -gt 0 ] && echo ', `follower.csv` / `follower-report.md`, `readprobe-follower.csv`, `follower-removal-N-{before,at,after}.json`, `network-definition.json`')."
} >> "$manifest"

# Accumulating index — one line per run, newest last, never rewritten.
idx="$runs/INDEX.md"
[ -f "$idx" ] || printf '# Soak runs\n\nEvery run appends one row. Details in `<runId>/manifest.md`.\n\n| run | commit | executor | healing | elapsed | exit | dn height | heals | note |\n|---|---|---|---|---|---|---|---|---|\n' > "$idx"
printf '| [%s](%s/manifest.md) | `%s` | %s | %s | %sh | %s | %s→%s | %s→%s | %s |\n' \
  "$run_id" "$run_id" "$git_desc" "$exec_ver" "$heal_flags" "$elapsed_h" "$rc" \
  "${first_h:-?}" "${last_h:-?}" "${first_x:-?}" "${last_x:-?}" "$NOTE" >> "$idx"

# A finished run tears its network down. It did not, on a clean finish: only
# the failure paths above called `down`, so every run that ended well left
# eight validators up with no monitor — the 20260829T003712Z network ran 22
# minutes past its verdict before anyone noticed. KEEP_UP=1 keeps it for
# probing, deliberately.
if [ "${KEEP_UP:-0}" = 1 ]; then
  echo "KEEP_UP=1: network left up for probing — tear it down yourself:" | tee -a "$log"
  echo "  COMPOSE_PROJECT_NAME=$COMPOSE_PROJECT_NAME $compose down -v --remove-orphans" | tee -a "$log"
else
  $compose down -v --remove-orphans >/dev/null 2>&1
  echo "network torn down" | tee -a "$log"
  # And the monitor, which otherwise keeps the port for the next run. The
  # supervisor first, or it respawns a child between the two kills (run
  # 20260905T032333Z left a soakmon.py serving :8099 into the next run).
  kill "$MON" 2>/dev/null; sleep 1; pkill -P "$MON" 2>/dev/null; pkill -f "soakmon.py" 2>/dev/null
fi

echo
echo "Run recorded: $rd/manifest.md"
echo "Index:        $idx"
tail -3 "$log"
exit $rc
