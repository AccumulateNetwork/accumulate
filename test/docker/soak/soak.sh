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
  fault_model="restart or pause one BVN container every ${CHAOS_MIN}s + 0-${CHAOS_JITTER}s"
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
read -r n_val n_fol FOL_LIST FOL_PORTS FOL_PARTS <<<"$(python3 -c '
import sys
sys.path.insert(0, sys.argv[1])
import topology
f = topology.followers()
print(len(topology.validator_records()), len(f),
      ",".join(x["container"] for x in f) or "-",
      ",".join(str(x["port"]) for x in f) or "-",
      ";".join("/".join(x["partitions"]) for x in f) or "-")' "$here/.." 2>/dev/null)"
n_val=${n_val:-$n_node}; n_fol=${n_fol:-0}
FOL_LIST=${FOL_LIST:--}; FOL_PORTS=${FOL_PORTS:--}; FOL_PARTS=${FOL_PARTS:--}
if [ "$n_fol" -gt 0 ]; then
  topo_desc="$n_bvn BVNs, $n_val validators + $n_fol follower ($FOL_LIST, partitions ${FOL_PARTS//\// }) + bootstrap"
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
  echo "| storage | $(sed -nE 's/^database: *([a-z]+).*/\1/p' "$here/../docker-network.yml" | head -1) (docker-network.yml) |"
  echo "| block interval | ${ACC_BLOCK_INTERVAL:-1s} |"
  echo "| memory budget | mem_limit ${ACC_MEM_LIMIT:-1536m}, GOMEMLIMIT ${GOMEMLIMIT:-1200MiB} |"
  echo
  echo "Config as run is frozen in \`config/\` (soak.conf${conf_override:+ + override.conf}, the compose and network files). Results appended below on exit."
} > "$manifest"

printf '{"runId":"%s","startedUtc":"%s","image":"%s","imageId":"%s","commit":"%s","describe":"%s","branch":"%s","uncommittedFiles":%s,"executorVersion":"%s","healing":"%s","faultModel":"%s","bvns":%s,"nodes":%s,"validators":%s,"followers":%s,"followerContainers":"%s","followerPorts":"%s","followerPartitions":"%s","followerKeyForm":"pending","partitions":"%s","chaos":"%s","duration":"%s","tps":"%s","note":"%s"}\n' \
  "$run_id" "$(date -u +%FT%TZ)" "$soak_image" "$image_id" "$git_head" "$git_desc" "$git_branch" "$git_dirty" \
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
$compose build >/dev/null 2>&1 || { echo "compose build failed" | tee -a "$log"; exit 1; }
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
    env RUN_DIR="$rd" "$here/soakmon.py" >> "$rd/soakmon.log" 2>&1
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
  if [ "${#nodes[@]}" -eq 0 ]; then
    echo "$(date -u +%FT%TZ) NO validator containers matched the topology — chaos is doing nothing" >> "$chaos"
  fi
  echo "$(date -u +%FT%TZ) order: ${nodes[*]}" >> "$chaos"
  i=0; slot=0
  while [ "$(date +%s)" -lt "$end" ]; do
    w=$(( CHAOS_MIN + RANDOM % CHAOS_JITTER ))
    echo "$(date -u +%FT%TZ) sleeping ${w}s until the next disturbance" >> "$chaos"
    sleep "$w"
    slot=$((slot+1))
    if [ "${CHAOS_SKIP_ONE_IN:-0}" -gt 0 ] && [ $((slot % CHAOS_SKIP_ONE_IN)) -eq 0 ]; then
      echo "$(date -u +%FT%TZ) skip (slot $slot, CHAOS_SKIP_ONE_IN=$CHAOS_SKIP_ONE_IN)" >> "$chaos"; continue
    fi
    n=${nodes[$((i % ${#nodes[@]}))]}; cyc=$((i / ${#nodes[@]})); kind=$(( (i + cyc) % 2 )); i=$((i+1))
    if [ "$kind" -eq 0 ]; then
      echo "$(date -u +%FT%TZ) restart $n" >> "$chaos"; docker restart "$n" >/dev/null 2>&1
    else
      p=$((60 + RANDOM % 120))
      echo "$(date -u +%FT%TZ) pause $n ${p}s" >> "$chaos"
      docker pause "$n" >/dev/null 2>&1; sleep "$p"; docker unpause "$n" >/dev/null 2>&1
    fi
  done ) &
CHAOS=$!
fi

# Monitor: heights + total heals every 5 min
# followerHeals is its own column, not part of the heals sum: see the loop.
echo "time,dnHeight,heals,cpuPct,followerHeals" > "$mon"
( while kill -0 $DRIVER 2>/dev/null; do
    h=$(curl -s -X POST http://localhost:26680/v3 -H 'content-type: application/json' \
      -d '{"jsonrpc":"2.0","id":1,"method":"query","params":{"scope":"acc://dn.acme/ledger"}}' \
      | grep -oE '"index":[0-9]+' | head -1 | cut -d: -f2)
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
      x=$(docker exec "$c" sh -c 'wget -q -O - http://127.0.0.1:26670/metrics 2>/dev/null' \
        | grep -E '^accumulate_conductor_heal_entries_total\{[^}]*outcome="applied"' | awk '{s+=$NF} END {printf "%d", s}')
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
    echo "$ts,${h:-?},$heals,${cpu:-?},$fol_heals" >> "$mon"
    # 30 s, not 5 min: run 20260903T121819Z climbed from 45 MiB to the
    # GOMEMLIMIT in ten minutes and stats.csv had two points for it (PLAN S0).
    sleep ${MON_INTERVAL:-$([ "$duration_seconds" -le 1800 ] && echo 20 || echo 30)}
  done ) &

# Storage-backend counters over time (PLAN S0). BlockchainDB rewrites
# stats.json every 50 commits, so only the last snapshot survives a run — and
# stagedCommits, the D5 instrument, had no history. One row per (node,
# database) a minute, the few counters that move.
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
fi

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
# NOT stallkill: when it is the one ending the run it waits for this script to
# finish recording and then takes the network down, so killing it here would
# leave the containers up. It exits on its own once this script is gone.
# MON is the supervisor; kill its current soakmon child too, by PID, or the
# restart loop's last child outlives the run.
[ -n "${MON:-}" ] && pkill -P "$MON" 2>/dev/null
# The read probe writes its report on SIGTERM; give it a moment before the
# network goes away so the last round and the report land.
if [ -n "${READPROBE:-}" ]; then kill $READPROBE 2>/dev/null; wait $READPROBE 2>/dev/null; fi
kill $CHAOS ${MON:-} ${SEIZE:-} ${LOGCAP:-} ${WEDGE:-} 2>/dev/null
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
# lost, and comparing `Sending an anchor` per (source, destination, block) —
# the source read from the line, #4370 — is the same evidence a debugger uses
# for a divergence. Cheaper than an API poll per block at a one-second
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
  echo "| read-back probe | $(grep -m1 '^\*\*Whole run:\*\*' "$rd/readprobe-report.md" 2>/dev/null | sed 's/\*\*//g' || echo 'no report') |"
  if [ "$n_fol" -gt 0 ]; then
    echo "| follower read probe | $(grep -m1 -E '^\*\*acc-' "$rd/readprobe-report.md" 2>/dev/null | sed 's/\*\*//g' || echo '— not measured (no readprobe report)') |"
  fi
  # A run that wedged and dumped is the most valuable kind of run there is;
  # say so in the verdict rather than leaving the dirs to be stumbled upon.
  echo "| wedge captures (#4125) | $(ls -d "$rd"/wedge-* 2>/dev/null | wc -l) $(ls -d "$rd"/wedge-* 2>/dev/null | xargs -r -n1 basename | paste -sd', ' -) |"
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
    echo
    echo "Full detail in \`follower-report.md\`; the per-sample series in \`follower.csv\`."
  fi
  echo
  echo "Raw: \`soak.log\`, \`monitor.csv\`, \`chaos.log\`, \`loadgen-stats.json\`, \`readprobe.csv\` / \`readprobe-report.md\`$([ "$n_fol" -gt 0 ] && echo ', `follower.csv` / `follower-report.md`, `network-definition.json`')."
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
