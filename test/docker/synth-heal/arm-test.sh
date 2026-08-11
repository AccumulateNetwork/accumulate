#!/usr/bin/env bash
# Two-arm experiment for #4070: is the reported "only → acc://ACME wedges,
# BVN→BVN heals fine" divergence real, or an artefact of ACME-destined
# synthetics simply being far more numerous?
#
# WHY THIS IS IN DOUBT
#
# The loadgen change that surfaced #4070 ("every identity burns ACME and buys
# its own credits") also made ACME-destined synthetics dominate the population.
# Drops land proportionally to volume, so "only ACME wedged" and "ACME was most
# of what could wedge" predict the same observation. This runs matched arms —
# same drop rule, same traffic, same settle time — varying ONLY which
# destination partition loses messages, and reports wedges PER DROP.
#
#   ./arm-test.sh Directory   # drops → the DN. For user traffic that is exactly
#                             # the → acc://ACME class (burns, AddCredits).
#   ./arm-test.sh BVN2        # drops → a BVN. The class reported healthy.
#
# WHAT THIS CANNOT ANSWER
#
# It cannot separate "the token issuer is special" from "the Directory is
# special". User accounts do not route to the Directory — a network's routing
# table targets BVNs only (mainnet's has a single route, to Cyclops) — so the
# ONLY DN-destined user synthetics are → acc://ACME. The two hypotheses are
# structurally confounded and no choice of recipient separates them. Telling
# them apart needs executor-side instrumentation, not traffic shaping.
#
#   KEEP=1 ./arm-test.sh ...  # leave the network up for inspection
set -euo pipefail

dest="${1:-Directory}"
drop_every="${DROP_EVERY:-4}"
txns="${TXNS:-60}"
settle="${SETTLE:-180}"
# Host port for this harness's DN API. NOT 26660 — that belongs to a separate
# long-running ASP/staking devnet on this machine; this project must not collide
# with it. Also isolates us from the host-networked `asp` container.
api_port="${API_PORT:-27660}"
api="http://localhost:${api_port}"

here="$(cd "$(dirname "$0")" && pwd)"
repo="$(cd "$here/../../.." && pwd)"
cd "$here"

# Permanent modulo drop: every Nth sequence to $dest is lost EVERY time, so the
# original never arrives and only a heal can recover it. The hook wraps the
# executor's dispatcher only — conductor heal/reconcile re-submissions bypass it
# and can still land, which makes this a test of healing rather than of retry.
export DROP_SPEC="${dest}:%${drop_every}!"
compose="docker compose -f docker-compose.yml"

cleanup() { [ -n "${KEEP:-}" ] || $compose down -v --remove-orphans >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "== ARM: drop destination=$dest  spec=$DROP_SPEC  txns=$txns =="

# Rebuild unless REUSE_IMAGE=1. A stale image silently invalidates a run: an
# image built before the modulo/permanent drop syntax landed (ea15ef2f7) parses
# "<dest>:%K!" as malformed, logs "Ignoring malformed drop spec", and injects
# NOTHING — which previously read as "every dropped message was healed".
if [ -z "${REUSE_IMAGE:-}" ] || ! docker image inspect acc-synthheal:test >/dev/null 2>&1; then
  echo "== Building image from the current tree =="
  docker build -q -t acc-synthheal:test -f "$repo/Dockerfile" "$repo" >/dev/null
fi

$compose down -v --remove-orphans >/dev/null 2>&1 || true
$compose up -d

echo "== Waiting for the API =="
for _ in $(seq 1 80); do
  curl -sf -X POST "$api/v3" -H 'content-type: application/json' \
    -d '{"jsonrpc":"2.0","id":1,"method":"network-status","params":{"partition":"Directory"}}' \
    >/dev/null 2>&1 && break
  sleep 3
done
sleep 15

# loadgen produces BOTH classes in one run: sends between lite accounts
# (BVN→BVN) and burns / credit purchases (→ acc://ACME, i.e. BVN→DN). Only the
# drop target differs between arms, so both classes are present in both arms and
# the comparison is like-for-like.
echo "== Driving $txns transactions (mixed: sends, burns, credit purchases) =="
lg_log="$(mktemp)"
go run "$repo/tools/cmd/loadgen" \
  -endpoint "$api" -count "$txns" -bootstrap 0 -faucet-seed FAUCET \
  -grace 120s -logtostderr >"$lg_log" 2>&1 || true
tail -25 "$lg_log"

# A run that generated nothing must not be reported as success. Without this
# guard "no wedged streams" is vacuously true and a broken harness looks
# identical to healthy healing — the #4073 lesson, applied to this script.
if ! grep -qE "sent=[1-9]|generating [0-9]+ transactions" "$lg_log"; then
  echo
  echo "RESULT: INVALID — loadgen produced no traffic; nothing was exercised."
  grep -iE "error|failed|fatal" "$lg_log" | head -5
  exit 3
fi

echo "== Settling ${settle}s (healing is jittered; give it room) =="
sleep "$settle"

echo
echo "== Synthetic ledger state =="
python3 - "$api" <<'PY'
import json, sys, urllib.request

API = sys.argv[1]

def q(scope):
    body = json.dumps({"jsonrpc":"2.0","id":1,"method":"query",
                       "params":{"scope":scope}}).encode()
    r = urllib.request.Request(f"{API}/v3", body,
                               {"content-type":"application/json"})
    with urllib.request.urlopen(r, timeout=20) as f:
        return json.load(f)

wedged = []
for part in ("dn.acme", "bvn-BVN1.acme", "bvn-BVN2.acme", "bvn-BVN3.acme"):
    try:
        acct = q(f"acc://{part}/synthetic").get("result", {}).get("account", {})
    except Exception as e:
        print(f"  {part}: query failed ({e})")
        continue
    for s in acct.get("sequence", []) or []:
        src = s.get("url", "?")
        prod, recv, deliv = s.get("produced",0), s.get("received",0), s.get("delivered",0)
        flag = ""
        if deliv < recv:
            flag = "  <-- WEDGED"
            wedged.append((part, src, recv, deliv))
        print(f"  {part:16} <- {src:22} produced={prod:<5} received={recv:<5} delivered={deliv:<5}{flag}")

print()
if wedged:
    print(f"RESULT: {len(wedged)} WEDGED stream(s) — a dropped message was not healed")
    for p, s, r, d in wedged:
        print(f"  {p} <- {s}: received={r} delivered={d} (gap {r-d})")
    sys.exit(2)
print("RESULT: no wedged streams — every dropped message was healed")
PY
rc=$?

echo
logf="$(mktemp)"
$compose logs >"$logf" 2>/dev/null || true
# "sequenced envelope" = modulo/permanent mode, "synthetic envelope" = count mode
drops=$(grep -ci "DEBUG dropping .* envelope" "$logf" || true)
heals=$(grep -ci "Requested missing synthetic\|request missing synthetic" "$logf" || true)
recon=$(grep -ci "Reconcile: pulled messages" "$logf" || true)
echo "  (full container log retained at $logf)"
echo "drops injected:    ${drops:-0}"
echo "gap-scan heals:    ${heals:-0}"
echo "reconcile pulls:   ${recon:-0}"
echo "total recoveries:  $(( ${heals:-0} + ${recon:-0} ))"

if [ "${drops:-0}" -eq 0 ]; then
  echo
  echo "RESULT: INVALID — no drop was injected, so nothing was exercised."
  echo "A 'no wedged streams' verdict here means the injector did not fire,"
  echo "NOT that healing worked. Check DROP_SPEC=$DROP_SPEC matches real traffic."
  exit 3
fi

echo
echo "Wedges per drop is the comparable figure across arms, not raw counts."
exit $rc
