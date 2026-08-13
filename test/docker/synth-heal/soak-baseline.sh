#!/usr/bin/env bash
# Long-running NO-FAULT baseline soak. Sustained load, no injected drops, for
# a configurable duration (default 24h). Validates a release candidate the way
# a short run cannot: slow leaks, unbounded growth, gradual stream divergence
# and heal churn only show up over hours.
#
#   ./soak-baseline.sh                  # 24h at 2 tps
#   DURATION=6h TPS=5 ./soak-baseline.sh
#
# Designed to be launched detached (nohup/setsid) and to survive the shell that
# started it. All state lands in a run directory so results are never inferred
# from a stale log — see soak-run-provenance.
#
# PASS requires, at every sample: no wedged stream, every node alive, and no
# panic/consensus error in any container log. A soak that merely "finished" is
# not a pass.
set -uo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
repo="$(cd "$here/../../.." && pwd)"
cd "$here"

duration="${DURATION:-24h}"
tps="${TPS:-2}"
sample_every="${SAMPLE_EVERY:-300}"      # seconds between samples
api_port="${API_PORT:-27660}"
api="http://localhost:${api_port}"

# ATTACH=1 samples a network that is already up and already under load, rather
# than bringing one up. Use it to resume monitoring a run whose sampler died
# without discarding the hours the network has already accumulated — the state
# under test lives in the containers, not in this script.
#   ATTACH=1 RUN_DIR=soak/runs/<stamp> DURATION=23h ./soak-baseline.sh
attach="${ATTACH:-}"

if [ -n "$attach" ]; then
  run="$(cd "${RUN_DIR:?ATTACH=1 requires RUN_DIR}" && pwd)"
  stamp="$(basename "$run")"
else
  stamp="$(date -u +%Y%m%dT%H%M%SZ)"
  run="$here/soak/runs/$stamp"
fi
mkdir -p "$run"

# No fault injection. Note ${VAR-default} not ${VAR:-default}: the latter
# substitutes on empty and would silently inject a drop.
export DROP_SPEC=""

# Where the invoking user is not in the docker group, docker must be reached
# through `sg docker`. Getting this wrong is not a loud failure: `docker compose
# ps` just returns nothing, the sampler reads that as zero running nodes, and
# the soak reports a FAILURE that never happened.
if docker info >/dev/null 2>&1; then
  compose() { docker compose -f docker-compose.yml "$@"; }
  dock()    { docker "$@"; }
else
  compose() { sg docker -c "cd '$here' && docker compose -f docker-compose.yml $*"; }
  dock()    { sg docker -c "docker $*"; }
fi

log() { echo "[$(date -u +%H:%M:%S)] $*" | tee -a "$run/soak.log"; }

# grep -c prints "0" AND exits 1 when there is no match, so the obvious
# `$(grep -c ... || echo 0)` yields the two-line string "0\n0" — which corrupts
# the CSV row and makes every `[ "$n" -ne 0 ]` guard below die with "integer
# expression expected", silently disarming the failure checks.
count() {
  local n
  n=$(grep -ciE "$1" "$2" 2>/dev/null | head -1 | tr -dc '0-9')
  echo "${n:-0}"
}

cat >> "$run/manifest.txt" <<EOF
${attach:+attached_utc  $(date -u +%Y%m%dT%H%M%SZ) (resumed monitoring; network already running)}
started_utc   $stamp
duration      $duration
tps           $tps
sample_every  ${sample_every}s
image         acc-synthheal:test
git_describe  $(git -C "$repo" describe --tags 2>/dev/null)
git_commit    $(git -C "$repo" rev-parse HEAD 2>/dev/null)
drop_spec     (none — baseline)
api           $api
EOF
log "run dir: $run"
cat "$run/manifest.txt" | tee -a "$run/soak.log"

# Record the sampler's pid so it can be stopped by pid. Killing it by pattern
# (pkill -f soak-baseline) is a trap: the pattern also matches the very command
# line that launches or greps for it, so the caller kills its own shell.
echo $$ > "$run/sampler.pid"
trap 'rm -f "$run/sampler.pid"' EXIT

if [ -z "$attach" ]; then
  compose down -v --remove-orphans >/dev/null 2>&1
  compose up -d >>"$run/soak.log" 2>&1
fi

log "waiting for API"
for _ in $(seq 1 80); do
  curl -sf -X POST "$api/v3" -H 'content-type: application/json' \
    -d '{"jsonrpc":"2.0","id":1,"method":"network-status","params":{"partition":"Directory"}}' \
    >/dev/null 2>&1 && break
  sleep 3
done
sleep 15

lg_pid=""
if [ -z "$attach" ]; then
  log "starting loadgen: ${tps} tps for ${duration}"
  # -stats-file lets netmon.py show live transaction load beside block and
  # synthetic-stream state; without it the monitor can only parse loadgen.log.
  go run "$repo/tools/cmd/loadgen" -endpoint "$api" -tps "$tps" -duration "$duration" \
    -bootstrap 0 -faucet-seed FAUCET -grace 5m -logtostderr \
    -stats-file "$run/loadgen-stats.json" \
    >"$run/loadgen.log" 2>&1 &
  lg_pid=$!
else
  log "attached to running network; load is generated externally"
fi

[ -s "$run/samples.csv" ] || \
  echo "ts,wedged_streams,streams,nodes_up,rss_total_mb,disk_used_gb,drops,heals,reconcile,errors" > "$run/samples.csv"

secs=$(python3 -c "
import re
d='$duration'; m=re.fullmatch(r'(\d+)([hms])',d)
print(int(m.group(1))*{'h':3600,'m':60,'s':1}[m.group(2)] if m else 86400)" 2>/dev/null)
case "${secs:-}" in ''|*[!0-9]*) secs=86400 ;; esac
deadline=$(( $(date +%s) + secs ))
# Log the resolved window: a soak that ends early is otherwise indistinguishable
# from one that ran to completion, and its verdict looks just as authoritative.
log "sampling every ${sample_every}s until $(date -u -d "@$deadline" +%FT%TZ) (${secs}s)"
verdict=PASS
first_fail=""

while [ "$(date +%s)" -lt "$deadline" ]; do
  sleep "$sample_every"

  nodes_up=$(compose ps --status running -q 2>/dev/null | wc -l)
  rss=$(dock stats --no-stream --format "'{{.MemUsage}}'" 2>/dev/null \
        | tr -d "'" \
        | awk -F'/' '{print $1}' | sed 's/MiB//;s/GiB/*1024/' | awk '{s+=$1} END{printf "%.0f", s}')
  disk=$(df -BG --output=used / | tail -1 | tr -dc '0-9')

  compose logs --no-color >"$run/containers.log" 2>/dev/null
  drops=$(count "DEBUG dropping" "$run/containers.log")
  heals=$(count "Requested missing synthetic" "$run/containers.log")
  recon=$(count "Reconcile: pulled" "$run/containers.log")
  errs=$(count "panic:|consensus failure|apphash" "$run/containers.log")

  read -r wedged total < <(python3 - "$api" <<'PY'
import json,sys,urllib.request
API=sys.argv[1]
def q(s):
    b=json.dumps({"jsonrpc":"2.0","id":1,"method":"query","params":{"scope":s}}).encode()
    r=urllib.request.Request(f"{API}/v3",b,{"content-type":"application/json"})
    with urllib.request.urlopen(r,timeout=20) as f: return json.load(f)
w=t=0
for p in ("dn.acme","bvn-BVN1.acme","bvn-BVN2.acme","bvn-BVN3.acme"):
    try: a=q(f"acc://{p}/synthetic").get("result",{}).get("account",{})
    except Exception: continue
    for s in a.get("sequence",[]) or []:
        t+=1
        if s.get("delivered",0) < s.get("received",0): w+=1
print(w,t)
PY
)

  echo "$(date -u +%FT%TZ),$wedged,$total,$nodes_up,$rss,$disk,$drops,$heals,$recon,$errs" >> "$run/samples.csv"
  log "wedged=$wedged/$total nodes=$nodes_up rss=${rss}MB disk=${disk}G drops=$drops recov=$((heals+recon)) errs=$errs"

  if [ "${wedged:-0}" -ne 0 ] || [ "${errs:-0}" -ne 0 ] || [ "${nodes_up:-0}" -lt 6 ]; then
    verdict=FAIL
    [ -z "$first_fail" ] && first_fail="$(date -u +%FT%TZ) wedged=$wedged errs=$errs nodes=$nodes_up"
    log "FAILURE CONDITION: wedged=$wedged errs=$errs nodes_up=$nodes_up"
  fi
  if [ "${drops:-0}" -ne 0 ]; then
    verdict=INVALID
    log "INVALID: $drops drop(s) injected — this must be a no-fault run"
  fi
done

if [ -n "$lg_pid" ]; then
  wait "$lg_pid" 2>/dev/null
  log "loadgen finished"
fi

# A soak is only a pass if it actually soaked. Without this, a sampler that dies
# after one sample still writes "verdict PASS" and reads exactly like a run that
# went the distance — the failure mode is a false clean bill of health, which is
# worse than no run at all.
samples=$(( $(wc -l < "$run/samples.csv") - 1 ))
expected=$(( secs / sample_every ))
if [ "$samples" -lt $(( expected * 8 / 10 )) ]; then
  verdict=INVALID
  log "INVALID: only $samples of ~$expected expected samples — the run did not cover its window"
fi

{
  echo "verdict       $verdict"
  echo "first_failure ${first_fail:-none}"
  echo "samples       $samples of ~$expected expected"
  echo "ended_utc     $(date -u +%Y%m%dT%H%M%SZ)"
} | tee -a "$run/manifest.txt" | tee -a "$run/soak.log"

[ -n "${KEEP:-}" ] || compose down -v --remove-orphans >/dev/null 2>&1
log "done: $verdict"
